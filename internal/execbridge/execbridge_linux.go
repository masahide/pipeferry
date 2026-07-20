//go:build linux

package execbridge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/masahide/pipeferry/internal/streamcopy"
)

type Config struct {
	Executable      string
	Args            []string
	ShutdownTimeout time.Duration
	Logger          *slog.Logger
}

type Result struct {
	ChildPID       int
	BytesToChild   int64
	BytesFromChild int64
	Err            error
}

type ProcessFactory interface {
	CommandContext(context.Context, string, ...string) *exec.Cmd
}

type SystemProcessFactory struct{}

func (SystemProcessFactory) CommandContext(ctx context.Context, executable string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, executable, args...)
}

func Serve(ctx context.Context, conn net.Conn, config Config, factory ProcessFactory) Result {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	command := factory.CommandContext(childCtx, config.Executable, config.Args...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return Result{Err: fmt.Errorf("open child stdin: %w", err)}
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Result{Err: fmt.Errorf("open child stdout: %w", err)}
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return Result{Err: fmt.Errorf("open child stderr: %w", err)}
	}
	if err := command.Start(); err != nil {
		return Result{Err: fmt.Errorf("start child: %w", err)}
	}
	pid := command.Process.Pid
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		scanner := bufio.NewScanner(io.LimitReader(stderr, 64*1024))
		for scanner.Scan() {
			if config.Logger != nil {
				config.Logger.Warn("child stderr", "pid", pid, "message", scanner.Text())
			}
		}
	}()

	childStream := childReadWriter{Reader: stdout, Writer: stdin}
	// Duplex must observe only caller cancellation. The close callback also
	// cancels childCtx to reap the child, so passing childCtx to Duplex would
	// misclassify every normal EOF as context.Canceled.
	copyResult := streamcopy.Duplex(ctx, conn, childStream, func() {
		_ = conn.Close()
		_ = stdin.Close()
		cancel()
	})

	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-time.After(config.ShutdownTimeout):
		_ = command.Process.Kill()
		waitErr = <-waitDone
	}
	stderrWG.Wait()
	if copyResult.Err != nil {
		err = copyResult.Err
	} else if waitErr != nil && !expectedExit(waitErr, ctx) {
		err = fmt.Errorf("child exited: %w", waitErr)
	}
	return Result{
		ChildPID:       pid,
		BytesToChild:   copyResult.BytesLeftToRight,
		BytesFromChild: copyResult.BytesRightToLeft,
		Err:            err,
	}
}

func expectedExit(err error, parent context.Context) bool {
	if err == nil || parent.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == -1
}

type childReadWriter struct {
	io.Reader
	io.Writer
}

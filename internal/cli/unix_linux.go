//go:build linux

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/masahide/pipeferry/internal/execbridge"
	"github.com/masahide/pipeferry/internal/logging"
	"github.com/masahide/pipeferry/internal/unixsocket"
)

type fileModeValue struct {
	mode fs.FileMode
}

func (v *fileModeValue) String() string {
	return fmt.Sprintf("%04o", v.mode.Perm())
}

func (v *fileModeValue) Set(value string) error {
	parsed, err := strconv.ParseUint(value, 8, 32)
	if err != nil || parsed == 0 || parsed > 0o777 {
		return fmt.Errorf("invalid file mode %q", value)
	}
	v.mode = fs.FileMode(parsed)
	return nil
}

func runUnixListen(ctx context.Context, args []string, stderr io.Writer) error {
	flags := newFlagSet("unix-listen", stderr)
	socketPath := flags.String("socket", "", "unix socket path")
	socketMode := &fileModeValue{mode: 0o600}
	flags.Var(socketMode, "socket-mode", "unix socket mode in octal")
	shutdownTimeout := flags.Duration("shutdown-timeout", 5*time.Second, "graceful shutdown timeout")
	maxConnections := flags.Int("max-connections", 32, "maximum concurrent connections")
	logLevel := flags.String("log-level", "info", "debug, info, warn, or error")
	logFormat := flags.String("log-format", "text", "text or json")
	logFile := flags.String("log-file", "", "append logs to this file")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return exitError(ExitUsage, "%v", err)
	}
	child := flags.Args()
	if len(child) == 0 {
		return exitError(ExitUsage, "child command is required after --")
	}
	if *shutdownTimeout <= 0 || *maxConnections < 1 {
		return exitError(ExitUsage, "shutdown timeout must be positive and max connections must be at least 1")
	}
	level, err := logging.ParseLevel(*logLevel)
	if err != nil {
		return exitError(ExitUsage, "%v", err)
	}
	logOutput, closeLog, err := logging.Open(*logFile, stderr)
	if err != nil {
		return exitError(ExitUsage, "%v", err)
	}
	defer closeLog()
	logger, err := logging.New(logOutput, level, *logFormat)
	if err != nil {
		return exitError(ExitUsage, "%v", err)
	}
	path, err := unixsocket.ResolvePath(*socketPath)
	if err != nil {
		return &Error{Code: ExitUnixSocket, Err: err}
	}
	listener, err := unixsocket.Listen(path, socketMode.mode)
	if err != nil {
		if errors.Is(err, unixsocket.ErrAlreadyRunning) {
			return &Error{Code: ExitAlreadyRunning, Err: err}
		}
		return &Error{Code: ExitUnixSocket, Err: err}
	}
	defer listener.Cleanup()

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("listener started", "socket", path, "max_connections", *maxConnections)
	go func() {
		<-signalCtx.Done()
		_ = listener.Close()
	}()

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, *maxConnections)
	for {
		select {
		case semaphore <- struct{}{}:
		case <-signalCtx.Done():
			return finishConnections(&wg, *shutdownTimeout, logger)
		}
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			<-semaphore
			if signalCtx.Err() != nil || errors.Is(acceptErr, net.ErrClosed) {
				return finishConnections(&wg, *shutdownTimeout, logger)
			}
			return &Error{Code: ExitUnixSocket, Err: fmt.Errorf("accept connection: %w", acceptErr)}
		}
		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer func() { <-semaphore }()
			id := connectionID()
			result := execbridge.Serve(signalCtx, conn, execbridge.Config{
				Executable: child[0], Args: append([]string(nil), child[1:]...),
				ShutdownTimeout: *shutdownTimeout, Logger: logger.With("connection_id", id),
			}, execbridge.SystemProcessFactory{})
			if result.Err != nil {
				logger.Error("connection ended", "connection_id", id, "pid", result.ChildPID, "error", result.Err)
			} else {
				logger.Debug("connection ended", "connection_id", id, "pid", result.ChildPID,
					"bytes_to_child", result.BytesToChild, "bytes_from_child", result.BytesFromChild)
			}
		}(conn)
	}
}

func finishConnections(wg *sync.WaitGroup, timeout time.Duration, logger *slog.Logger) error {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.Info("listener stopped")
		return nil
	case <-time.After(timeout):
		return &Error{Code: ExitTimeout, Err: errors.New("shutdown timeout exceeded")}
	}
}

func connectionID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func runStatus(args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("status", stderr)
	socketPath := flags.String("socket", "", "unix socket path")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return exitError(ExitUsage, "%v", err)
	}
	if flags.NArg() != 0 {
		return exitError(ExitUsage, "unexpected arguments: %v", flags.Args())
	}
	path, err := unixsocket.ResolvePath(*socketPath)
	if err != nil {
		return &Error{Code: ExitUnixSocket, Err: err}
	}
	result := unixsocket.Inspect(path)
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(result)
	}
	_, err = fmt.Fprintf(stdout, "socket: %s\nexists: %t\ntype: %s\nrunning: %t\nstale: %t\nlocked: %t\n",
		result.SocketPath, result.Exists, socketType(result), result.Running, result.Stale, result.Locked)
	return err
}

func socketType(status unixsocket.Status) string {
	if status.IsSocket {
		return "unix socket"
	}
	if status.Exists {
		return "other"
	}
	return "missing"
}

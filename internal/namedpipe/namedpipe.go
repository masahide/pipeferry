package namedpipe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/masahide/pipeferry/internal/streamcopy"
)

var (
	ErrConnect  = errors.New("named pipe connection failed")
	ErrTimeout  = errors.New("named pipe connection timeout")
	ErrTransfer = errors.New("named pipe transfer failed")
)

type Dialer interface {
	Dial(context.Context, string) (io.ReadWriteCloser, error)
}

type Config struct {
	PipePath       string
	ConnectTimeout time.Duration
	CheckOnly      bool
}

func NormalizePath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("pipe name is required")
	}
	if strings.HasPrefix(strings.ToLower(name), `\\.\pipe\`) {
		if len(name) == len(`\\.\pipe\`) {
			return "", errors.New("pipe name is required")
		}
		return name, nil
	}
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid pipe name %q", name)
	}
	return `\\.\pipe\` + name, nil
}

func Run(ctx context.Context, config Config, dialer Dialer, stdin io.Reader, stdout io.Writer) error {
	if config.ConnectTimeout <= 0 {
		return errors.New("connect timeout must be positive")
	}
	path, err := NormalizePath(config.PipePath)
	if err != nil {
		return err
	}
	dialCtx, cancel := context.WithTimeout(ctx, config.ConnectTimeout)
	defer cancel()
	pipe, err := dialer.Dial(dialCtx, path)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(dialCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return fmt.Errorf("%w: %v", ErrConnect, err)
	}
	if config.CheckOnly {
		return pipe.Close()
	}
	stdio := readWriter{Reader: stdin, Writer: stdout}
	result := streamcopy.Duplex(ctx, stdio, pipe, func() {
		_ = pipe.Close()
		_ = stdio.Close()
	})
	if result.Err != nil {
		return fmt.Errorf("%w: %v", ErrTransfer, result.Err)
	}
	return nil
}

type readWriter struct {
	io.Reader
	io.Writer
}

func (rw readWriter) Close() error {
	if closer, ok := rw.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

package namedpipe

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestNormalizePath(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
		ok    bool
	}{
		{"openssh-ssh-agent", `\\.\pipe\openssh-ssh-agent`, true},
		{`\\.\pipe\example-service`, `\\.\pipe\example-service`, true},
		{"", "", false},
		{`bad\name`, "", false},
	} {
		got, err := NormalizePath(tc.input)
		if (err == nil) != tc.ok || got != tc.want {
			t.Fatalf("NormalizePath(%q) = %q, %v", tc.input, got, err)
		}
	}
}

type dialFunc func(context.Context, string) (io.ReadWriteCloser, error)

func (fn dialFunc) Dial(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	return fn(ctx, path)
}

func TestRunTransfersWithoutStdoutLogs(t *testing.T) {
	bridge, server := net.Pipe()
	defer server.Close()
	dialer := dialFunc(func(context.Context, string) (io.ReadWriteCloser, error) {
		return bridge, nil
	})
	stdinReader, stdinWriter := io.Pipe()
	var stdout bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{PipePath: "test", ConnectTimeout: time.Second}, dialer, stdinReader, &stdout)
	}()
	go func() {
		request := make([]byte, 4)
		_, _ = io.ReadFull(server, request)
		_, _ = server.Write([]byte{0, 1, 2, 3})
		_ = server.Close()
	}()
	_, _ = stdinWriter.Write([]byte("ping"))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = stdinWriter.Close()
	if !bytes.Equal(stdout.Bytes(), []byte{0, 1, 2, 3}) {
		t.Fatalf("stdout changed: %v", stdout.Bytes())
	}
}

func TestRunCheckOnly(t *testing.T) {
	bridge, server := net.Pipe()
	defer server.Close()
	err := Run(context.Background(), Config{PipePath: "test", ConnectTimeout: time.Second, CheckOnly: true},
		dialFunc(func(context.Context, string) (io.ReadWriteCloser, error) { return bridge, nil }),
		bytes.NewReader([]byte("must not transfer")), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunClassifiesDialFailure(t *testing.T) {
	err := Run(context.Background(), Config{PipePath: "test", ConnectTimeout: time.Second},
		dialFunc(func(context.Context, string) (io.ReadWriteCloser, error) { return nil, errors.New("no service") }),
		bytes.NewReader(nil), io.Discard)
	if !errors.Is(err, ErrConnect) {
		t.Fatalf("expected ErrConnect, got %v", err)
	}
}

func TestRunClassifiesDialTimeout(t *testing.T) {
	err := Run(context.Background(), Config{PipePath: "test", ConnectTimeout: 10 * time.Millisecond},
		dialFunc(func(ctx context.Context, _ string) (io.ReadWriteCloser, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}),
		bytes.NewReader(nil), io.Discard)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

//go:build windows

package namedpipe

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func TestSystemDialerIntegration(t *testing.T) {
	path := `\\.\pipe\pipeferry-test-` + time.Now().Format("150405.000000000")
	listener, err := winio.ListenPipe(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		request := make([]byte, 7)
		if _, err := io.ReadFull(conn, request); err != nil {
			serverDone <- err
			return
		}
		if !bytes.Equal(request, []byte{0, 1, 0xff, 2, 0, 3, 4}) {
			serverDone <- &net.AddrError{Err: "payload changed", Addr: path}
			return
		}
		_, err = conn.Write([]byte{9, 0, 8, 0xff})
		serverDone <- err
	}()

	stdin, stdinWriter := io.Pipe()
	var stdout bytes.Buffer
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(context.Background(), Config{PipePath: path, ConnectTimeout: 2 * time.Second},
			SystemDialer{}, stdin, &stdout)
	}()
	if _, err := stdinWriter.Write([]byte{0, 1, 0xff, 2, 0, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if err := <-runDone; err != nil {
		t.Fatal(err)
	}
	_ = stdinWriter.Close()
	if !bytes.Equal(stdout.Bytes(), []byte{9, 0, 8, 0xff}) {
		t.Fatalf("stdout payload changed: %v", stdout.Bytes())
	}
}

func TestSystemDialerCheckAndMissingPipe(t *testing.T) {
	path := `\\.\pipe\pipeferry-check-` + time.Now().Format("150405.000000000")
	listener, err := winio.ListenPipe(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
		close(accepted)
	}()
	err = Run(context.Background(), Config{PipePath: path, ConnectTimeout: time.Second, CheckOnly: true},
		SystemDialer{}, bytes.NewReader([]byte("ignored")), io.Discard)
	_ = listener.Close()
	<-accepted
	if err != nil {
		t.Fatal(err)
	}

	missing := `\\.\pipe\pipeferry-missing-` + time.Now().Format("150405.000000000")
	err = Run(context.Background(), Config{PipePath: missing, ConnectTimeout: 30 * time.Millisecond},
		SystemDialer{}, os.Stdin, io.Discard)
	if err == nil {
		t.Fatal("missing pipe unexpectedly connected")
	}
}

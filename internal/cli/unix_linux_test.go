//go:build linux

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/masahide/pipeferry/internal/unixsocket"
)

func TestUnixListenEndToEndAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listener.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var stderr bytes.Buffer
	go func() {
		done <- runUnixListen(ctx, []string{
			"--socket", path,
			"--shutdown-timeout", "1s",
			"--",
			os.Args[0], "-test.run=TestCLIHelperProcess",
		}, &stderr)
	}()

	var conn net.Conn
	var err error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = net.Dial("unix", path)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("listener did not start: %v, stderr=%s", err, stderr.String())
	}
	payload := []byte{0, 1, 0xff, 3}
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(response, payload) {
		t.Fatalf("payload changed: %v", response)
	}
	_ = conn.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("listener returned %v, stderr=%s", err, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not stop")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("socket remains: %v", err)
	}
	if _, err := os.Lstat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock remains: %v", err)
	}
}

func TestCLIHelperProcess(t *testing.T) {
	if len(os.Args) < 2 || os.Args[1] != "-test.run=TestCLIHelperProcess" {
		return
	}
	payload := make([]byte, 4)
	if _, err := io.ReadFull(os.Stdin, payload); err != nil {
		os.Exit(2)
	}
	_, _ = os.Stdout.Write(payload)
	os.Exit(0)
}

func TestUnixListenRequiresChildAndValidOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--socket", filepath.Join(t.TempDir(), "x.sock")},
		{"--socket-mode", "999", "--", "echo"},
		{"--log-format", "xml", "--", "echo"},
		{"--max-connections", "0", "--", "echo"},
	} {
		if err := runUnixListen(context.Background(), args, io.Discard); exitCode(err) != ExitUsage {
			t.Fatalf("args=%v err=%v code=%d", args, err, exitCode(err))
		}
	}
}

func TestStatusJSONContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sock")
	var stdout bytes.Buffer
	if err := runStatus([]string{"--socket", path, "--json"}, &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	var result unixsocket.Status
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SocketPath != path || result.Exists {
		t.Fatalf("result=%+v", result)
	}
}

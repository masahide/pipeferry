//go:build linux

package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/masahide/pipeferry/internal/unixsocket"
)

var e2eSocketPath = flag.String("pipeferry-e2e-socket", "", "Unix socket for the manual cross-OS E2E client")
var e2ePayloadSize = flag.Int("pipeferry-e2e-size", 0, "number of bytes for the manual cross-OS E2E client")
var e2eWindowsExecutable = flag.String("pipeferry-e2e-windows-exe", "", "Windows executable for E2E startup timing")
var e2eBenchmarkSamples = flag.Int("pipeferry-e2e-samples", 0, "number of manual E2E benchmark samples")

func TestUnixSocketE2EClient(t *testing.T) {
	if *e2eSocketPath == "" || *e2ePayloadSize <= 0 {
		t.Skip("E2E helper is not requested")
	}
	conn, err := net.DialTimeout("unix", *e2eSocketPath, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	payload := make([]byte, *e2ePayloadSize)
	for index := range payload {
		payload[index] = byte((index*37 + 0xff) % 256)
	}
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(response, payload) {
		t.Fatal("cross-OS E2E payload changed")
	}
}

func TestWindowsBridgeE2EBenchmark(t *testing.T) {
	if *e2eSocketPath == "" || *e2eWindowsExecutable == "" || *e2eBenchmarkSamples <= 0 {
		t.Skip("E2E benchmark is not requested")
	}
	startup := make([]time.Duration, 0, *e2eBenchmarkSamples)
	roundTrip := make([]time.Duration, 0, *e2eBenchmarkSamples)
	for range *e2eBenchmarkSamples {
		started := time.Now()
		if output, err := exec.Command(*e2eWindowsExecutable, "--version").CombinedOutput(); err != nil {
			t.Fatalf("start Windows process: %v, output=%s", err, output)
		}
		startup = append(startup, time.Since(started))

		started = time.Now()
		conn, err := net.DialTimeout("unix", *e2eSocketPath, 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Write([]byte{0, 0, 0, 1, 11}); err != nil {
			_ = conn.Close()
			t.Fatal(err)
		}
		var length [4]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			_ = conn.Close()
			t.Fatal(err)
		}
		body := make([]byte, binary.BigEndian.Uint32(length[:]))
		if _, err := io.ReadFull(conn, body); err != nil {
			_ = conn.Close()
			t.Fatal(err)
		}
		_ = conn.Close()
		roundTrip = append(roundTrip, time.Since(started))
	}
	t.Logf("Windows process startup: p50=%v p95=%v p99=%v",
		percentile(startup, 50), percentile(startup, 95), percentile(startup, 99))
	t.Logf("SSH Agent connection and round trip: p50=%v p95=%v p99=%v",
		percentile(roundTrip, 50), percentile(roundTrip, 95), percentile(roundTrip, 99))
}

func percentile(samples []time.Duration, percent int) time.Duration {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (len(sorted)*percent + 99) / 100
	if index < 1 {
		index = 1
	}
	return sorted[index-1]
}

func TestUnixListenEndToEndAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "listener.sock")
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

func TestUnixListenSignalsCleanup(t *testing.T) {
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private", "listener.sock")
			command := exec.Command(os.Args[0], "-test.run=^TestUnixListenSignalHelper$")
			command.Env = append(os.Environ(),
				"GO_WANT_UNIX_LISTEN_SIGNAL_HELPER=1",
				"PIPEFERRY_TEST_SOCKET="+path,
			)
			var output bytes.Buffer
			command.Stdout = &output
			command.Stderr = &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}

			waitForPath(t, path, 2*time.Second)
			conn, err := net.Dial("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			time.Sleep(50 * time.Millisecond)
			if err := command.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- command.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("listener exited after %s: %v, output=%s", signal, err, output.String())
				}
			case <-time.After(2 * time.Second):
				_ = command.Process.Kill()
				t.Fatalf("listener did not stop after %s", signal)
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatalf("socket remains after %s: %v", signal, err)
			}
			if _, err := os.Lstat(path + ".lock"); !os.IsNotExist(err) {
				t.Fatalf("lock remains after %s: %v", signal, err)
			}
		})
	}
}

func TestUnixListenSignalHelper(t *testing.T) {
	if os.Getenv("GO_WANT_UNIX_LISTEN_SIGNAL_HELPER") != "1" {
		return
	}
	err := runUnixListen(context.Background(), []string{
		"--socket", os.Getenv("PIPEFERRY_TEST_SOCKET"),
		"--shutdown-timeout", "500ms",
		"--",
		os.Args[0], "-test.run=^TestCLIBlockingHelper$",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCLIBlockingHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[1] != "-test.run=^TestCLIBlockingHelper$" {
		return
	}
	time.Sleep(10 * time.Second)
}

func TestFinishConnectionsTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	started := time.Now()
	err := finishConnections(&wg, 20*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	wg.Done()
	if exitCode(err) != ExitTimeout {
		t.Fatalf("finishConnections error=%v code=%d", err, exitCode(err))
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond || elapsed > time.Second {
		t.Fatalf("timeout elapsed=%v", elapsed)
	}
}

func waitForPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("path did not appear: %s", path)
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

func TestResolveConfiguredWindowsExecutable(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("PATH", "")
	configDir := filepath.Join(configHome, "pipeferry")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	windowsExecutable := filepath.Join(t.TempDir(), "pipeferry.exe")
	if err := os.WriteFile(windowsExecutable, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "windows-executable"),
		[]byte(windowsExecutable+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveChildExecutable("pipeferry.exe")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != windowsExecutable {
		t.Fatalf("resolved=%q want=%q", resolved, windowsExecutable)
	}
}

func TestResolveConfiguredWindowsExecutableRejectsInvalidPath(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("PATH", "")
	configDir := filepath.Join(configHome, "pipeferry")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "windows-executable"),
		[]byte("relative/pipeferry.exe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveChildExecutable("pipeferry.exe"); err == nil {
		t.Fatal("relative configured path was accepted")
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

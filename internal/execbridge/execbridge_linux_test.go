//go:build linux

package execbridge

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func TestServeEchoChild(t *testing.T) {
	client, bridge := net.Pipe()
	done := make(chan Result, 1)
	go func() {
		done <- Serve(context.Background(), bridge, Config{
			Executable:      os.Args[0],
			Args:            []string{"-test.run=TestExecbridgeHelperProcess"},
			ShutdownTimeout: time.Second,
			Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		}, SystemProcessFactory{})
	}()
	payload := []byte{0, 1, 0xff, 2}
	if _, err := client.Write(payload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(client, response); err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	result := <-done
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.ChildPID == 0 || result.BytesToChild != int64(len(payload)) || result.BytesFromChild != int64(len(payload)) {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestServeMissingExecutable(t *testing.T) {
	client, bridge := net.Pipe()
	defer client.Close()
	result := Serve(context.Background(), bridge, Config{
		Executable:      "/definitely/not/pipeferry",
		ShutdownTimeout: 100 * time.Millisecond,
	}, SystemProcessFactory{})
	if result.Err == nil {
		t.Fatal("missing executable succeeded")
	}
}

func TestServeAbnormalExitAndClientDisconnect(t *testing.T) {
	client, bridge := net.Pipe()
	abnormal := Serve(context.Background(), bridge, Config{
		Executable:      os.Args[0],
		Args:            []string{"-test.run=TestExecbridgeAbnormalHelper"},
		ShutdownTimeout: time.Second,
	}, SystemProcessFactory{})
	_ = client.Close()
	if abnormal.Err == nil {
		t.Fatal("abnormal child exit was accepted")
	}

	client, bridge = net.Pipe()
	done := make(chan Result, 1)
	go func() {
		done <- Serve(context.Background(), bridge, Config{
			Executable:      os.Args[0],
			Args:            []string{"-test.run=TestExecbridgeBlockingHelper"},
			ShutdownTimeout: time.Second,
		}, SystemProcessFactory{})
	}()
	_ = client.Close()
	select {
	case result := <-done:
		if result.ChildPID == 0 {
			t.Fatalf("child was not started: %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client disconnect did not reap child")
	}
}

func TestServeCancellationAndFailureIsolation(t *testing.T) {
	client, bridge := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() {
		done <- Serve(ctx, bridge, Config{
			Executable:      os.Args[0],
			Args:            []string{"-test.run=TestExecbridgeBlockingHelper"},
			ShutdownTimeout: time.Second,
		}, SystemProcessFactory{})
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not reap child")
	}
	_ = client.Close()

	failedClient, failedBridge := net.Pipe()
	failed := make(chan Result, 1)
	go func() {
		failed <- Serve(context.Background(), failedBridge, Config{
			Executable: "/definitely/not/pipeferry", ShutdownTimeout: time.Second,
		}, SystemProcessFactory{})
	}()
	goodClient, goodBridge := net.Pipe()
	good := make(chan Result, 1)
	go func() {
		good <- Serve(context.Background(), goodBridge, Config{
			Executable: os.Args[0], Args: []string{"-test.run=TestExecbridgeHelperProcess"},
			ShutdownTimeout: time.Second,
		}, SystemProcessFactory{})
	}()
	payload := []byte{0, 1, 0xff, 2}
	_, _ = goodClient.Write(payload)
	response := make([]byte, 4)
	if _, err := io.ReadFull(goodClient, response); err != nil {
		t.Fatal(err)
	}
	_ = goodClient.Close()
	_ = failedClient.Close()
	if result := <-failed; result.Err == nil {
		t.Fatal("failed connection unexpectedly succeeded")
	}
	if result := <-good; result.Err != nil {
		t.Fatalf("failed connection affected good connection: %v", result.Err)
	}
}

func TestServeThirtyTwoConcurrentAndOneHundredSequential(t *testing.T) {
	run := func() error {
		client, bridge := net.Pipe()
		done := make(chan Result, 1)
		go func() {
			done <- Serve(context.Background(), bridge, Config{
				Executable:      os.Args[0],
				Args:            []string{"-test.run=TestExecbridgeHelperProcess"},
				ShutdownTimeout: time.Second,
			}, SystemProcessFactory{})
		}()
		payload := []byte{0, 1, 0xff, 2}
		if _, err := client.Write(payload); err != nil {
			return err
		}
		response := make([]byte, len(payload))
		if _, err := io.ReadFull(client, response); err != nil {
			return err
		}
		_ = client.Close()
		select {
		case result := <-done:
			return result.Err
		case <-time.After(3 * time.Second):
			return fmt.Errorf("child did not exit")
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- run()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100; i++ {
		if err := run(); err != nil {
			t.Fatalf("sequential connection %d: %v", i, err)
		}
	}
}

func TestExecbridgeHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_EXECBRIDGE_HELPER") != "1" {
		// The parent process sets no environment override through Config, so use
		// the test name to distinguish the helper invocation.
		if len(os.Args) < 2 || os.Args[1] != "-test.run=TestExecbridgeHelperProcess" {
			return
		}
	}
	payload := make([]byte, 4)
	if _, err := io.ReadFull(os.Stdin, payload); err != nil {
		os.Exit(2)
	}
	if _, err := os.Stdout.Write(payload); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestExecbridgeAbnormalHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[1] != "-test.run=TestExecbridgeAbnormalHelper" {
		return
	}
	os.Exit(42)
}

func TestExecbridgeBlockingHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[1] != "-test.run=TestExecbridgeBlockingHelper" {
		return
	}
	time.Sleep(10 * time.Second)
	os.Exit(0)
}

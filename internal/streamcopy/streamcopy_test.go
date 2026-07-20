package streamcopy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestDuplexByteExact(t *testing.T) {
	leftClient, leftBridge := net.Pipe()
	rightBridge, rightClient := net.Pipe()
	var closes atomic.Int32
	done := make(chan Result, 1)
	go func() {
		done <- Duplex(context.Background(), leftBridge, rightBridge, func() {
			closes.Add(1)
			_ = leftBridge.Close()
			_ = rightBridge.Close()
		})
	}()

	payload := append([]byte{0, 1, 2}, bytes.Repeat([]byte("pipeferry"), 128*1024)...)
	go func() {
		_, _ = leftClient.Write(payload)
	}()
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(rightClient, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("left-to-right payload changed")
	}

	response := []byte{0xff, 0x00, 0x7f}
	go func() {
		_, _ = rightClient.Write(response)
	}()
	back := make([]byte, len(response))
	if _, err := io.ReadFull(leftClient, back); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, response) {
		t.Fatal("right-to-left payload changed")
	}
	_ = leftClient.Close()
	_ = rightClient.Close()
	select {
	case result := <-done:
		if result.Err != nil {
			t.Fatalf("Duplex returned %v", result.Err)
		}
		if closes.Load() != 1 {
			t.Fatalf("close called %d times", closes.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Duplex leaked after EOF")
	}
}

func TestDuplexCancellation(t *testing.T) {
	leftClient, leftBridge := net.Pipe()
	rightBridge, rightClient := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() {
		done <- Duplex(ctx, leftBridge, rightBridge, func() {
			_ = leftBridge.Close()
			_ = rightBridge.Close()
		})
	}()
	cancel()
	result := <-done
	_ = leftClient.Close()
	_ = rightClient.Close()
	if result.Reason != EndCanceled || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("got reason=%s err=%v", result.Reason, result.Err)
	}
}

func TestNormalErrors(t *testing.T) {
	for _, err := range []error{nil, io.EOF, io.ErrClosedPipe, net.ErrClosed, errors.New("write: broken pipe")} {
		if !IsNormalError(err) {
			t.Fatalf("%v should be normal", err)
		}
	}
	if IsNormalError(errors.New("disk exploded")) {
		t.Fatal("unexpected normal classification")
	}
}

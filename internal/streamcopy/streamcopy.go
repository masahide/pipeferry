package streamcopy

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type EndReason string

const (
	EndEOF      EndReason = "eof"
	EndCanceled EndReason = "canceled"
	EndError    EndReason = "error"
)

type Result struct {
	BytesLeftToRight int64
	BytesRightToLeft int64
	Reason           EndReason
	Err              error
}

type directionResult struct {
	leftToRight bool
	bytes       int64
	err         error
}

// Duplex copies opaque bytes in both directions. closeAll must interrupt any
// blocked reads and is called exactly once when either direction ends.
func Duplex(ctx context.Context, left io.ReadWriter, right io.ReadWriter, closeAll func()) Result {
	var once sync.Once
	closeOnce := func() {
		if closeAll != nil {
			once.Do(closeAll)
		}
	}

	results := make(chan directionResult, 2)
	go copyDirection(results, true, right, left)
	go copyDirection(results, false, left, right)

	var first directionResult
	select {
	case first = <-results:
		closeOnce()
	case <-ctx.Done():
		closeOnce()
		first = directionResult{err: ctx.Err()}
	}

	var second directionResult
	select {
	case second = <-results:
	case <-ctx.Done():
		second.err = ctx.Err()
	case <-time.After(5 * time.Second):
		second.err = errors.New("copy shutdown timeout")
	}
	closeOnce()

	result := Result{Reason: EndEOF}
	for _, item := range []directionResult{first, second} {
		if item.leftToRight {
			result.BytesLeftToRight = item.bytes
		} else {
			result.BytesRightToLeft = item.bytes
		}
		if !IsNormalError(item.err) && result.Err == nil {
			result.Err = item.err
		}
	}
	if errors.Is(result.Err, context.Canceled) || errors.Is(result.Err, context.DeadlineExceeded) {
		result.Reason = EndCanceled
	} else if result.Err != nil {
		result.Reason = EndError
	}
	return result
}

func copyDirection(results chan<- directionResult, leftToRight bool, dst io.Writer, src io.Reader) {
	n, err := io.Copy(dst, src)
	results <- directionResult{leftToRight: leftToRight, bytes: n, err: err}
}

func IsNormalError(err error) bool {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "closed network connection") ||
		strings.Contains(message, "file already closed")
}

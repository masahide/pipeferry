//go:build windows

package namedpipe

import (
	"context"
	"io"

	"github.com/Microsoft/go-winio"
)

type SystemDialer struct{}

func (SystemDialer) Dial(ctx context.Context, path string) (io.ReadWriteCloser, error) {
	return winio.DialPipeContext(ctx, path)
}

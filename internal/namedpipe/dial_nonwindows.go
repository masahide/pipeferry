//go:build !windows

package namedpipe

import (
	"context"
	"errors"
	"io"
)

type SystemDialer struct{}

func (SystemDialer) Dial(context.Context, string) (io.ReadWriteCloser, error) {
	return nil, errors.New("named pipes are supported only on windows")
}

//go:build !linux

package cli

import (
	"context"
	"io"
)

func runUnixListen(context.Context, []string, io.Writer) error {
	return exitError(ExitUsage, "unix-listen is supported only on linux")
}

func runStatus([]string, io.Writer, io.Writer) error {
	return exitError(ExitUsage, "status is supported only on linux")
}

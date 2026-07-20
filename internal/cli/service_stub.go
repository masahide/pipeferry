//go:build !linux

package cli

import (
	"context"
	"io"
)

func runService(context.Context, []string, io.Writer, io.Writer) error {
	return exitError(ExitUsage, "service is supported only on linux")
}

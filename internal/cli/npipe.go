package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/masahide/pipeferry/internal/logging"
	"github.com/masahide/pipeferry/internal/namedpipe"
)

func runNamedPipe(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := newFlagSet("npipe-connect", stderr)
	pipeName := flags.String("pipe", "", "named pipe name or full path")
	timeout := flags.Duration("connect-timeout", 5*time.Second, "named pipe connection timeout")
	checkOnly := flags.Bool("check", false, "check connectivity without transferring data")
	logLevel := flags.String("log-level", "info", "debug, info, warn, or error")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return exitError(ExitUsage, "%v", err)
	}
	if flags.NArg() != 0 {
		return exitError(ExitUsage, "unexpected arguments: %v", flags.Args())
	}
	if _, err := logging.ParseLevel(*logLevel); err != nil {
		return exitError(ExitUsage, "%v", err)
	}
	config := namedpipe.Config{PipePath: *pipeName, ConnectTimeout: *timeout, CheckOnly: *checkOnly}
	err := namedpipe.Run(ctx, config, namedpipe.SystemDialer{}, stdin, stdout)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, namedpipe.ErrTimeout):
		return &Error{Code: ExitTimeout, Err: err}
	case errors.Is(err, namedpipe.ErrConnect):
		return &Error{Code: ExitNamedPipe, Err: err}
	case errors.Is(err, namedpipe.ErrTransfer):
		return &Error{Code: ExitTransfer, Err: err}
	default:
		return exitError(ExitUsage, "%v", err)
	}
}

package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"

	"github.com/masahide/pipeferry/internal/buildinfo"
)

const usage = `Usage:
  pipeferry unix-listen [options] -- executable [arguments...]
  pipeferry npipe-connect --pipe NAME [options]
  pipeferry status [options]
  pipeferry doctor [options] -- executable [arguments...]
  pipeferry version
`

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	err := run(context.Background(), args, stdin, stdout, stderr)
	if err == nil {
		return ExitOK
	}
	if !errors.Is(err, flag.ErrHelp) {
		_, _ = fmt.Fprintf(stderr, "pipeferry: %v\n", err)
	}
	return exitCode(err)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return exitError(ExitUsage, "command is required")
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = io.WriteString(stdout, usage)
		return nil
	case "-v", "--version", "version":
		if len(args) != 1 {
			return exitError(ExitUsage, "version does not accept arguments")
		}
		_, _ = fmt.Fprintln(stdout, buildinfo.String())
		return nil
	case "npipe-connect":
		if runtime.GOOS != "windows" {
			return exitError(ExitUsage, "npipe-connect is supported only on windows")
		}
		return runNamedPipe(ctx, args[1:], stdin, stdout, stderr)
	case "unix-listen":
		if runtime.GOOS != "linux" {
			return exitError(ExitUsage, "unix-listen is supported only on linux")
		}
		return runUnixListen(ctx, args[1:], stderr)
	case "status":
		if runtime.GOOS != "linux" {
			return exitError(ExitUsage, "status is supported only on linux")
		}
		return runStatus(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(ctx, args[1:], stdout, stderr)
	default:
		return exitError(ExitUsage, "unknown command %q", args[0])
	}
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

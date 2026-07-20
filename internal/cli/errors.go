package cli

import (
	"errors"
	"fmt"
)

const (
	ExitOK             = 0
	ExitInternal       = 1
	ExitUsage          = 2
	ExitNotFound       = 3
	ExitUnixSocket     = 4
	ExitNamedPipe      = 5
	ExitTransfer       = 6
	ExitAlreadyRunning = 7
	ExitTimeout        = 8
	ExitDiagnostic     = 9
)

type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string {
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	return e.Err
}

func exitError(code int, format string, args ...any) error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

func exitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return cliErr.Code
	}
	return ExitInternal
}

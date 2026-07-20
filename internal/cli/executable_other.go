//go:build !linux

package cli

func resolveChildExecutable(executable string) (string, error) {
	return executable, nil
}

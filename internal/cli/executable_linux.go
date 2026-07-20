//go:build linux

package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func resolveChildExecutable(executable string) (string, error) {
	if executable != "pipeferry.exe" {
		return executable, nil
	}
	if resolved, err := exec.LookPath(executable); err == nil {
		return resolved, nil
	}
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve config directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}
	configPath := filepath.Join(configDir, "pipeferry", "windows-executable")
	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return executable, nil
	}
	if err != nil {
		return "", fmt.Errorf("read Windows executable config: %w", err)
	}
	resolved := strings.TrimSpace(string(data))
	if resolved == "" || !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("invalid Windows executable path in %s", configPath)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("configured Windows executable %q: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("configured Windows executable is not a regular file: %s", resolved)
	}
	return resolved, nil
}

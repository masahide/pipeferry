//go:build linux

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/masahide/pipeferry/internal/unixsocket"
)

const (
	serviceStartTimeout = 10 * time.Second
	serviceDocURL       = "https://github.com/masahide/pipeferry"
)

var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type serviceConfig struct {
	Name            string
	SocketName      string
	ShutdownTimeout time.Duration
	MaxConnections  int
	Force           bool
	Child           []string
}

type serviceStatus struct {
	Name        string `json:"name"`
	Unit        string `json:"unit"`
	UnitFile    string `json:"unitFile"`
	Enabled     bool   `json:"enabled"`
	Active      bool   `json:"active"`
	Socket      string `json:"socket"`
	SocketState string `json:"socketState"`
}

func runService(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return exitError(ExitUsage, "service subcommand is required: install, status, or uninstall")
	}
	switch args[0] {
	case "install":
		return runServiceInstall(ctx, args[1:], stdout, stderr)
	case "status":
		return runServiceStatus(ctx, args[1:], stdout, stderr)
	case "uninstall":
		return runServiceUninstall(ctx, args[1:], stdout, stderr)
	default:
		return exitError(ExitUsage, "unknown service subcommand %q", args[0])
	}
}

func runServiceInstall(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("service install", stderr)
	config := serviceConfig{ShutdownTimeout: 5 * time.Second, MaxConnections: 32}
	flags.StringVar(&config.Name, "name", "", "logical service name")
	flags.StringVar(&config.SocketName, "socket-name", "", "socket file name below %t/pipeferry")
	flags.DurationVar(&config.ShutdownTimeout, "shutdown-timeout", config.ShutdownTimeout, "graceful shutdown timeout")
	flags.IntVar(&config.MaxConnections, "max-connections", config.MaxConnections, "maximum concurrent connections")
	flags.BoolVar(&config.Force, "force", false, "replace a different existing unit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return exitError(ExitUsage, "%v", err)
	}
	config.Child = flags.Args()
	if err := validateServiceConfig(config); err != nil {
		return err
	}
	runtimeDir, err := requireSystemdUser(ctx)
	if err != nil {
		return err
	}
	self, err := absoluteExecutable(os.Args[0])
	if err != nil {
		return &Error{Code: ExitNotFound, Err: fmt.Errorf("resolve Pipeferry executable: %w", err)}
	}
	child, err := resolveServiceChild(config.Child)
	if err != nil {
		return &Error{Code: ExitNotFound, Err: err}
	}
	config.Child = child
	unitPath, err := serviceUnitPath(config.Name)
	if err != nil {
		return &Error{Code: ExitUnixSocket, Err: err}
	}
	content, err := renderServiceUnit(config, self)
	if err != nil {
		return err
	}
	existing, readErr := os.ReadFile(unitPath)
	switch {
	case readErr == nil && !bytes.Equal(existing, content) && !config.Force:
		return exitError(ExitAlreadyRunning, "%s already exists with different settings; use --force to replace it", unitPath)
	case readErr != nil && !errors.Is(readErr, os.ErrNotExist):
		return &Error{Code: ExitUnixSocket, Err: fmt.Errorf("read existing unit: %w", readErr)}
	}
	if err := writeFileAtomic(unitPath, content, 0o644); err != nil {
		return &Error{Code: ExitUnixSocket, Err: fmt.Errorf("install unit file: %w", err)}
	}
	unit := serviceUnitName(config.Name)
	for _, command := range [][]string{
		{"daemon-reload"},
		{"enable", unit},
		{"restart", unit},
	} {
		if err := runSystemctl(ctx, command...); err != nil {
			printServiceRecovery(stderr, unit)
			return err
		}
	}
	socketPath := filepath.Join(runtimeDir, "pipeferry", config.SocketName)
	if err := waitForService(ctx, unit, socketPath, serviceStartTimeout); err != nil {
		printServiceRecovery(stderr, unit)
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Installed and started %s\nSocket: %s\nLogs: journalctl --user --unit %s --follow\n",
		unit, socketPath, unit)
	return nil
}

func runServiceStatus(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("service status", stderr)
	name := flags.String("name", "", "logical service name")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return exitError(ExitUsage, "%v", err)
	}
	if flags.NArg() != 0 {
		return exitError(ExitUsage, "unexpected arguments: %v", flags.Args())
	}
	if err := validateServiceName(*name); err != nil {
		return err
	}
	runtimeDir, err := requireSystemdUser(ctx)
	if err != nil {
		return err
	}
	unit := serviceUnitName(*name)
	unitPath, err := serviceUnitPath(*name)
	if err != nil {
		return &Error{Code: ExitUnixSocket, Err: err}
	}
	socketName := readUnitSocketName(unitPath)
	socketPath := ""
	socketState := "missing"
	if socketName != "" {
		socketPath = filepath.Join(runtimeDir, "pipeferry", socketName)
		socketState = inspectSocketState(socketPath)
	}
	result := serviceStatus{
		Name: *name, Unit: unit, UnitFile: unitPath,
		Enabled: systemctlState(ctx, "is-enabled", unit),
		Active:  systemctlState(ctx, "is-active", unit),
		Socket:  socketPath, SocketState: socketState,
	}
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(result)
	}
	_, err = fmt.Fprintf(stdout, "Service: %s\nUnitFile: %s\nEnabled: %s\nActive: %s\nSocket: %s\nSocketState: %s\n",
		result.Unit, result.UnitFile, yesNo(result.Enabled), yesNo(result.Active), result.Socket, result.SocketState)
	return err
}

func runServiceUninstall(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("service uninstall", stderr)
	name := flags.String("name", "", "logical service name")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return exitError(ExitUsage, "%v", err)
	}
	if flags.NArg() != 0 {
		return exitError(ExitUsage, "unexpected arguments: %v", flags.Args())
	}
	if err := validateServiceName(*name); err != nil {
		return err
	}
	runtimeDir, err := requireSystemdUser(ctx)
	if err != nil {
		return err
	}
	unit := serviceUnitName(*name)
	unitPath, err := serviceUnitPath(*name)
	if err != nil {
		return &Error{Code: ExitUnixSocket, Err: err}
	}
	socketName := readUnitSocketName(unitPath)
	if err := runSystemctlAllowMissing(ctx, "disable", "--now", unit); err != nil {
		printServiceRecovery(stderr, unit)
		return err
	}
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return &Error{Code: ExitUnixSocket, Err: fmt.Errorf("remove unit file: %w", err)}
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctlAllowMissing(ctx, "reset-failed", unit); err != nil {
		return err
	}
	if socketName != "" {
		socketPath := filepath.Join(runtimeDir, "pipeferry", socketName)
		if err := removeOwnedServiceFiles(socketPath); err != nil {
			return &Error{Code: ExitUnixSocket, Err: err}
		}
	}
	_, _ = fmt.Fprintf(stdout, "Uninstalled %s\n", unit)
	return nil
}

func validateServiceConfig(config serviceConfig) error {
	if err := validateServiceName(config.Name); err != nil {
		return err
	}
	if err := validateSocketName(config.SocketName); err != nil {
		return err
	}
	if config.ShutdownTimeout <= 0 {
		return exitError(ExitUsage, "shutdown timeout must be positive")
	}
	if config.MaxConnections < 1 {
		return exitError(ExitUsage, "max connections must be at least 1")
	}
	if len(config.Child) == 0 {
		return exitError(ExitUsage, "child command is required after --")
	}
	for _, value := range config.Child {
		if err := validateUnitValue(value); err != nil {
			return exitError(ExitUsage, "invalid child argument: %v", err)
		}
	}
	return nil
}

func validateServiceName(name string) error {
	if !serviceNamePattern.MatchString(name) {
		return exitError(ExitUsage, "name is required and may contain only letters, digits, '-' and '_'")
	}
	return nil
}

func validateSocketName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.IsAbs(name) ||
		strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return exitError(ExitUsage, "socket-name must be a file name without path separators or '..'")
	}
	if err := validateUnitValue(name); err != nil {
		return exitError(ExitUsage, "invalid socket-name: %v", err)
	}
	return nil
}

func validateUnitValue(value string) error {
	if value == "" {
		return errors.New("empty values are not allowed")
	}
	for _, r := range value {
		if r == 0 || r == '\n' || r == '\r' || unicode.IsControl(r) {
			return fmt.Errorf("control characters are not allowed")
		}
	}
	return nil
}

func resolveServiceChild(child []string) ([]string, error) {
	result := append([]string(nil), child...)
	resolved, err := resolveChildExecutable(result[0])
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(resolved) {
		resolved, err = exec.LookPath(resolved)
		if err != nil {
			return nil, fmt.Errorf("find child executable %q: %w", result[0], err)
		}
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, fmt.Errorf("make child executable absolute: %w", err)
	}
	result[0] = resolved
	return result, nil
}

func absoluteExecutable(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if evaluated, evalErr := filepath.EvalSymlinks(path); evalErr == nil {
		path = evaluated
	}
	return path, nil
}

func renderServiceUnit(config serviceConfig, self string) ([]byte, error) {
	for _, value := range append([]string{self, config.Name, config.SocketName}, config.Child...) {
		if err := validateUnitValue(value); err != nil {
			return nil, exitError(ExitUsage, "invalid unit value: %v", err)
		}
	}
	arguments := []string{
		self, "unix-listen",
		"--socket", "%t/pipeferry/" + config.SocketName,
		"--shutdown-timeout", config.ShutdownTimeout.String(),
		"--max-connections", strconv.Itoa(config.MaxConnections),
		"--",
	}
	arguments = append(arguments, config.Child...)
	escaped := make([]string, len(arguments))
	for index, argument := range arguments {
		escaped[index] = quoteSystemdArgument(argument, strings.HasPrefix(argument, "%t/pipeferry/"))
	}
	content := fmt.Sprintf(`[Unit]
Description=Pipeferry service %s
Documentation=%s

[Service]
# Pipeferry-SocketName=%s
Type=simple
ExecStart=:%s
Restart=on-failure
RestartSec=2s

[Install]
WantedBy=default.target
`, config.Name, serviceDocURL, config.SocketName, strings.Join(escaped, " "))
	return []byte(content), nil
}

func quoteSystemdArgument(value string, preserveRuntimeSpecifier bool) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	if preserveRuntimeSpecifier {
		value = "%t" + strings.ReplaceAll(strings.TrimPrefix(value, "%t"), `%`, `%%`)
	} else {
		value = strings.ReplaceAll(value, `%`, `%%`)
	}
	return `"` + value + `"`
}

func requireSystemdUser(ctx context.Context) (string, error) {
	comm, err := os.ReadFile("/proc/1/comm")
	if err != nil || strings.TrimSpace(string(comm)) != "systemd" {
		return "", systemdUnavailableError()
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "", systemdUnavailableError()
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" || !filepath.IsAbs(runtimeDir) {
		return "", systemdUnavailableError()
	}
	info, err := os.Stat(runtimeDir)
	if err != nil || !info.IsDir() {
		return "", systemdUnavailableError()
	}
	command := exec.CommandContext(ctx, "systemctl", "--user", "show-environment")
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w\n\nsystemctl --user: %s", systemdUnavailableError(), strings.TrimSpace(string(output)))
	}
	return runtimeDir, nil
}

func systemdUnavailableError() error {
	return errors.New(`systemd user services are not available

Enable systemd in /etc/wsl.conf:

[boot]
systemd=true

Then run the following command from Windows:

wsl --shutdown`)
}

func serviceUnitName(name string) string {
	return "pipeferry-" + name + ".service"
}

func serviceUnitPath(name string) (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "systemd", "user", serviceUnitName(name)), nil
}

func writeFileAtomic(path string, content []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".pipeferry-unit-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func runSystemctl(ctx context.Context, args ...string) error {
	commandArgs := append([]string{"--user"}, args...)
	command := exec.CommandContext(ctx, "systemctl", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(commandArgs, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runSystemctlAllowMissing(ctx context.Context, args ...string) error {
	commandArgs := append([]string{"--user"}, args...)
	command := exec.CommandContext(ctx, "systemctl", commandArgs...)
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.ToLower(string(output))
	if strings.Contains(text, "not loaded") || strings.Contains(text, "not exist") ||
		strings.Contains(text, "not found") || strings.Contains(text, "no files found") {
		return nil
	}
	return fmt.Errorf("systemctl %s: %w: %s", strings.Join(commandArgs, " "), err, strings.TrimSpace(string(output)))
}

func systemctlState(ctx context.Context, verb, unit string) bool {
	command := exec.CommandContext(ctx, "systemctl", "--user", "--quiet", verb, unit)
	return command.Run() == nil
}

func waitForService(ctx context.Context, unit, socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return &Error{Code: ExitTimeout, Err: ctx.Err()}
		}
		active := systemctlState(ctx, "is-active", unit)
		if active && inspectSocketState(socketPath) == "live" {
			return nil
		}
		if time.Now().After(deadline) {
			if !active {
				return &Error{Code: ExitTimeout, Err: fmt.Errorf("service %s did not become active within %s", unit, timeout)}
			}
			return &Error{Code: ExitDiagnostic, Err: fmt.Errorf("service is active but socket is not live: %s", socketPath)}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func inspectSocketState(path string) string {
	status := unixsocket.Inspect(path)
	switch {
	case status.Running:
		return "live"
	case status.Stale:
		return "stale"
	case status.Exists:
		return "invalid"
	default:
		return "missing"
	}
}

func readUnitSocketName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	const prefix = "# Pipeferry-SocketName="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			name := strings.TrimPrefix(line, prefix)
			if validateSocketName(name) == nil {
				return name
			}
		}
	}
	return ""
}

func removeOwnedServiceFiles(socketPath string) error {
	for _, path := range []string{socketPath, socketPath + ".lock"} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect residual file %s: %w", path, err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) {
			continue
		}
		if path == socketPath && info.Mode()&os.ModeSocket == 0 {
			continue
		}
		if path != socketPath && !info.Mode().IsRegular() {
			continue
		}
		if path == socketPath {
			conn, dialErr := net.DialTimeout("unix", path, 100*time.Millisecond)
			if dialErr == nil {
				_ = conn.Close()
				return fmt.Errorf("refusing to remove live socket %s", path)
			}
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove residual file %s: %w", path, err)
		}
	}
	return nil
}

func printServiceRecovery(stderr io.Writer, unit string) {
	_, _ = fmt.Fprintf(stderr, "\nInspect the service with:\n  systemctl --user status %s\n  journalctl --user --unit %s --no-pager\n", unit, unit)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

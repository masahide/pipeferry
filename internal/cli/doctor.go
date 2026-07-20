package cli

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type doctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

type doctorResult struct {
	OK     bool          `json:"ok"`
	Checks []doctorCheck `json:"checks"`
}

type doctorChecker interface {
	Check(context.Context) doctorCheck
}

type doctorCheckerFunc func(context.Context) doctorCheck

func (fn doctorCheckerFunc) Check(ctx context.Context) doctorCheck {
	return fn(ctx)
}

func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("doctor", stderr)
	jsonOutput := flags.Bool("json", false, "write JSON output")
	sshAgent := flags.Bool("ssh-agent", false, "include SSH agent diagnostics")
	timeout := flags.Duration("connect-timeout", 5*time.Second, "child check timeout")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return exitError(ExitUsage, "%v", err)
	}
	if *timeout <= 0 {
		return exitError(ExitUsage, "connect timeout must be positive")
	}

	checkers := []doctorChecker{
		doctorCheckerFunc(func(context.Context) doctorCheck { return checkWSL() }),
		doctorCheckerFunc(func(context.Context) doctorCheck { return checkInterop() }),
	}
	child := flags.Args()
	if len(child) > 0 {
		child = append([]string(nil), child...)
		if resolved, err := resolveChildExecutable(child[0]); err == nil {
			child[0] = resolved
		}
		checkers = append(checkers,
			doctorCheckerFunc(func(context.Context) doctorCheck { return checkExecutable(child[0]) }),
			doctorCheckerFunc(func(ctx context.Context) doctorCheck { return checkChild(ctx, *timeout, child) }),
		)
	} else {
		checkers = append(checkers, doctorCheckerFunc(func(context.Context) doctorCheck {
			return doctorCheck{Name: "child-command", OK: false, Error: "no child command provided after --"}
		}))
	}
	if *sshAgent {
		checkers = append(checkers, doctorCheckerFunc(func(ctx context.Context) doctorCheck {
			return checkSSHAgent(ctx, *timeout, child)
		}))
	}
	checks := make([]doctorCheck, 0, len(checkers))
	for _, checker := range checkers {
		checks = append(checks, checker.Check(ctx))
	}
	result := doctorResult{OK: true, Checks: checks}
	for _, check := range checks {
		result.OK = result.OK && check.OK
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			return err
		}
	} else {
		for _, check := range result.Checks {
			state := "ok"
			detail := check.Detail
			if !check.OK {
				state = "failed"
				detail = check.Error
			}
			_, _ = fmt.Fprintf(stdout, "%-16s %-6s %s\n", check.Name, state, detail)
		}
	}
	if !result.OK {
		return &Error{Code: ExitDiagnostic, Err: errors.New("one or more diagnostic checks failed")}
	}
	return nil
}

func checkWSL() doctorCheck {
	check := doctorCheck{Name: "wsl-environment"}
	if runtime.GOOS != "linux" {
		check.Error = "doctor is intended for WSL Linux"
		return check
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		check.Error = err.Error()
		return check
	}
	if !strings.Contains(strings.ToLower(string(data)), "microsoft") {
		check.Error = "kernel does not identify as WSL"
		return check
	}
	check.OK = true
	check.Detail = strings.TrimSpace(string(data))
	return check
}

func checkInterop() doctorCheck {
	check := doctorCheck{Name: "wsl-interop"}
	if runtime.GOOS != "linux" {
		check.Error = "not running on Linux"
		return check
	}
	candidates := []string{"cmd.exe", "/mnt/c/Windows/System32/cmd.exe"}
	var failures []string
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if err := exec.Command(path, "/d", "/c", "exit", "0").Run(); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		check.OK = true
		check.Detail = "WSL interoperability executed " + path
		return check
	}
	check.Error = strings.Join(failures, "; ")
	return check
}

func checkExecutable(name string) doctorCheck {
	path, err := exec.LookPath(name)
	if err != nil {
		return doctorCheck{Name: "child-executable", Error: err.Error()}
	}
	return doctorCheck{Name: "child-executable", OK: true, Detail: path}
}

func checkChild(ctx context.Context, timeout time.Duration, argv []string) doctorCheck {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append([]string(nil), argv[1:]...)
	if !contains(args, "--check") {
		args = append(args, "--check")
	}
	command := exec.CommandContext(checkCtx, argv[0], args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return doctorCheck{Name: "child-check", Error: strings.TrimSpace(string(output)) + ": " + err.Error()}
	}
	return doctorCheck{Name: "child-check", OK: true, Detail: "child connectivity check succeeded"}
}

func checkSSHAgent(ctx context.Context, timeout time.Duration, argv []string) doctorCheck {
	if len(argv) == 0 {
		return doctorCheck{Name: "ssh-agent", Error: "no child command provided after --"}
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(checkCtx, argv[0], argv[1:]...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return doctorCheck{Name: "ssh-agent", Error: err.Error()}
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return doctorCheck{Name: "ssh-agent", Error: err.Error()}
	}
	var childStderr strings.Builder
	command.Stderr = io.MultiWriter(io.Discard, &limitedWriter{Writer: &childStderr, remaining: 4096})
	if err := command.Start(); err != nil {
		return doctorCheck{Name: "ssh-agent", Error: err.Error()}
	}
	// SSH2_AGENTC_REQUEST_IDENTITIES: uint32 payload length followed by type 11.
	if _, err := stdin.Write([]byte{0, 0, 0, 1, 11}); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return doctorCheck{Name: "ssh-agent", Error: err.Error()}
	}
	var lengthBytes [4]byte
	if _, err := io.ReadFull(stdout, lengthBytes[:]); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return doctorCheck{Name: "ssh-agent", Error: strings.TrimSpace(childStderr.String() + " " + err.Error())}
	}
	length := binary.BigEndian.Uint32(lengthBytes[:])
	if length == 0 || length > 1024*1024 {
		_ = command.Process.Kill()
		_ = command.Wait()
		return doctorCheck{Name: "ssh-agent", Error: fmt.Sprintf("invalid SSH agent response length %d", length)}
	}
	response := make([]byte, length)
	if _, err := io.ReadFull(stdout, response); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return doctorCheck{Name: "ssh-agent", Error: err.Error()}
	}
	_ = stdin.Close()
	_ = command.Wait()
	if response[0] != 12 { // SSH2_AGENT_IDENTITIES_ANSWER
		return doctorCheck{Name: "ssh-agent", Error: fmt.Sprintf("unexpected SSH agent response type %d", response[0])}
	}
	return doctorCheck{Name: "ssh-agent", OK: true, Detail: "SSH agent identities request succeeded"}
}

type limitedWriter struct {
	io.Writer
	remaining int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return len(p), nil
	}
	toWrite := p
	if len(toWrite) > w.remaining {
		toWrite = toWrite[:w.remaining]
	}
	_, err := w.Writer.Write(toWrite)
	w.remaining -= len(toWrite)
	return len(p), err
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

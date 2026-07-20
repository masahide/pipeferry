package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExitCodeContracts(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"missing", nil, ExitUsage},
		{"unknown", []string{"wat"}, ExitUsage},
		{"help", []string{"--help"}, ExitOK},
		{"version", []string{"--version"}, ExitOK},
		{"version args", []string{"version", "extra"}, ExitUsage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := Run(tc.args, bytes.NewReader(nil), &stdout, &stderr); got != tc.code {
				t.Fatalf("Run(%v) = %d, stderr=%q", tc.args, got, stderr.String())
			}
		})
	}
}

func TestDoctorJSONContractAndIndependentChecks(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor", "--json", "--", "definitely-missing-pipeferry-child"},
		bytes.NewReader(nil), &stdout, &stderr)
	if code != ExitDiagnostic {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var result doctorResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || len(result.Checks) < 4 {
		t.Fatalf("result=%+v", result)
	}
	names := map[string]bool{}
	for _, check := range result.Checks {
		names[check.Name] = true
	}
	for _, name := range []string{"wsl-environment", "wsl-interop", "child-executable", "child-check"} {
		if !names[name] {
			t.Fatalf("missing independent check %q: %+v", name, result.Checks)
		}
	}
}

func TestSSHAgentCheck(t *testing.T) {
	result := checkSSHAgent(t.Context(), time.Second, []string{os.Args[0], "-test.run=TestSSHAgentHelper"})
	if !result.OK {
		t.Fatalf("result=%+v", result)
	}
}

func TestSSHAgentHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[1] != "-test.run=TestSSHAgentHelper" {
		return
	}
	request := make([]byte, 5)
	if _, err := io.ReadFull(os.Stdin, request); err != nil || !bytes.Equal(request, []byte{0, 0, 0, 1, 11}) {
		os.Exit(2)
	}
	_, _ = os.Stdout.Write([]byte{0, 0, 0, 5, 12, 0, 0, 0, 0})
	os.Exit(0)
}

func TestUnsupportedCommand(t *testing.T) {
	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"unix-listen", "--", "echo"}
	} else {
		args = []string{"npipe-connect", "--pipe", "test"}
	}
	var stderr bytes.Buffer
	if got := Run(args, bytes.NewReader(nil), ioDiscard{}, &stderr); got != ExitUsage {
		t.Fatalf("got %d: %s", got, stderr.String())
	}
	if !strings.Contains(stderr.String(), "supported only") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func TestNamedPipeValidationOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	for _, args := range [][]string{
		{"npipe-connect"},
		{"npipe-connect", "--pipe", "x", "--connect-timeout", "0s"},
		{"npipe-connect", "--pipe", "x", "--log-level", "trace"},
		{"npipe-connect", "--unknown"},
	} {
		var stderr bytes.Buffer
		if got := Run(args, bytes.NewReader(nil), ioDiscard{}, &stderr); got != ExitUsage {
			t.Fatalf("Run(%v)=%d, stderr=%q", args, got, stderr.String())
		}
	}
}

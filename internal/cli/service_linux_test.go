//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateServiceConfig(t *testing.T) {
	valid := serviceConfig{
		Name: "ssh-Agent_1", SocketName: "ssh-agent.sock",
		ShutdownTimeout: 5 * time.Second, MaxConnections: 32,
		Child: []string{"pipeferry.exe", "npipe-connect"},
	}
	if err := validateServiceConfig(valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	tests := []serviceConfig{
		{Name: "../bad", SocketName: "x.sock", ShutdownTimeout: time.Second, MaxConnections: 1, Child: []string{"x"}},
		{Name: "ok", SocketName: "../x.sock", ShutdownTimeout: time.Second, MaxConnections: 1, Child: []string{"x"}},
		{Name: "ok", SocketName: "/x.sock", ShutdownTimeout: time.Second, MaxConnections: 1, Child: []string{"x"}},
		{Name: "ok", SocketName: "x/y.sock", ShutdownTimeout: time.Second, MaxConnections: 1, Child: []string{"x"}},
		{Name: "ok", SocketName: "x.sock", ShutdownTimeout: 0, MaxConnections: 1, Child: []string{"x"}},
		{Name: "ok", SocketName: "x.sock", ShutdownTimeout: time.Second, MaxConnections: 0, Child: []string{"x"}},
		{Name: "ok", SocketName: "x.sock", ShutdownTimeout: time.Second, MaxConnections: 1},
		{Name: "ok", SocketName: "x.sock", ShutdownTimeout: time.Second, MaxConnections: 1, Child: []string{"x", "bad\narg"}},
	}
	for index, config := range tests {
		if err := validateServiceConfig(config); exitCode(err) != ExitUsage {
			t.Errorf("case %d: error=%v code=%d", index, err, exitCode(err))
		}
	}
}

func TestRenderServiceUnitEscapesArgumentsAndRecordsSocket(t *testing.T) {
	config := serviceConfig{
		Name: "ssh-agent", SocketName: "ssh%agent.sock",
		ShutdownTimeout: 7 * time.Second, MaxConnections: 9,
		Child: []string{"/mnt/c/Program Files/pipe%ferry.exe", `arg"with\special`, "${HOME}"},
	}
	content, err := renderServiceUnit(config, "/home/me/bin/pipeferry")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, expected := range []string{
		"Type=simple",
		"Restart=on-failure",
		"RestartSec=2s",
		"WantedBy=default.target",
		"# Pipeferry-SocketName=ssh%agent.sock",
		`ExecStart=:"/home/me/bin/pipeferry" "unix-listen" "--socket" "%t/pipeferry/ssh%%agent.sock"`,
		`"/mnt/c/Program Files/pipe%%ferry.exe"`,
		`"arg\"with\\special"`,
		`"${HOME}"`,
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("unit missing %q:\n%s", expected, text)
		}
	}
}

func TestUnitMetadataAndAtomicWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "systemd", "user", "pipeferry-test.service")
	content := []byte("[Service]\n# Pipeferry-SocketName=test.sock\n")
	if err := writeFileAtomic(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readUnitSocketName(path); got != "test.sock" {
		t.Fatalf("socket name=%q", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("unit mode=%v err=%v", info.Mode(), err)
	}
}

func TestResolveServiceChildUsesAbsolutePath(t *testing.T) {
	child, err := resolveServiceChild([]string{"sh", "-c", "exit 0"})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(child[0]) {
		t.Fatalf("child executable is not absolute: %q", child[0])
	}
}

func TestRemoveOwnedServiceFilesPreservesRegularSocketPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.sock")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeOwnedServiceFiles(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("regular file removed: %v", err)
	}
}

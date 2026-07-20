//go:build linux

package unixsocket

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestResolvePath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/xdg")
	got, err := ResolvePath("")
	if err != nil || got != "/tmp/xdg/pipeferry/pipeferry.sock" {
		t.Fatalf("ResolvePath = %q, %v", got, err)
	}
	explicit := filepath.Join(t.TempDir(), "custom.sock")
	got, err = ResolvePath(explicit)
	if err != nil || got != explicit {
		t.Fatalf("explicit ResolvePath = %q, %v", got, err)
	}
	home := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", home)
	got, err = ResolvePath("")
	want := filepath.Join(home, ".local", "run", "pipeferry", "pipeferry.sock")
	if err != nil || got != want {
		t.Fatalf("home ResolvePath = %q, %v; want %q", got, err, want)
	}
}

func TestListenModesLockAndCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "service.sock")
	listener, err := Listen(path, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	parentInfo, _ := os.Stat(filepath.Dir(path))
	socketInfo, _ := os.Stat(path)
	if parentInfo.Mode().Perm() != 0o700 || socketInfo.Mode().Perm() != 0o600 {
		t.Fatalf("modes: parent=%o socket=%o", parentInfo.Mode().Perm(), socketInfo.Mode().Perm())
	}
	if _, err := Listen(path, 0o600); !os.IsExist(err) && err != ErrAlreadyRunning {
		t.Fatalf("second listener = %v", err)
	}
	status := Inspect(path)
	if !status.Exists || !status.IsSocket || !status.Locked {
		t.Fatalf("status = %+v", status)
	}
	if err := listener.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("socket remains: %v", err)
	}
	if _, err := os.Lstat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock remains: %v", err)
	}
}

func TestStaleSocketRecoveryAndRegularFileSafety(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.sock")
	old, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	listener, err := Listen(path, 0o600)
	if err != nil {
		t.Fatalf("stale recovery: %v", err)
	}
	_ = listener.Cleanup()

	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(path, 0o600); err == nil {
		t.Fatal("regular file accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("regular file was modified: %q, %v", data, err)
	}

	directoryPath := filepath.Join(t.TempDir(), "existing-directory")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(directoryPath, 0o600); err == nil {
		t.Fatal("directory accepted as a socket")
	}
	info, err := os.Stat(directoryPath)
	if err != nil || !info.IsDir() {
		t.Fatalf("directory was modified: %v, %v", info, err)
	}
}

func TestOwnershipSafetyCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if !ownedByUID(info, stat.Uid) {
		t.Fatal("current owner rejected")
	}
	if ownedByUID(info, stat.Uid+1) {
		t.Fatal("different owner accepted")
	}
}

func TestInspectMissingStaleAndRegularFile(t *testing.T) {
	dir := t.TempDir()
	missing := Inspect(filepath.Join(dir, "missing.sock"))
	if missing.Exists || missing.Running || missing.Stale {
		t.Fatalf("missing = %+v", missing)
	}

	stalePath := filepath.Join(dir, "stale-inspect.sock")
	listener, err := net.Listen("unix", stalePath)
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()
	stale := Inspect(stalePath)
	if !stale.Exists || !stale.IsSocket || !stale.Stale || stale.Running {
		t.Fatalf("stale = %+v", stale)
	}

	filePath := filepath.Join(dir, "regular")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	regular := Inspect(filePath)
	if !regular.Exists || regular.IsSocket || regular.Error == "" {
		t.Fatalf("regular = %+v", regular)
	}
}

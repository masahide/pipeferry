//go:build linux

package unixsocket

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

var ErrAlreadyRunning = errors.New("pipeferry is already running")

type Status struct {
	SocketPath  string `json:"socketPath"`
	Exists      bool   `json:"exists"`
	IsSocket    bool   `json:"isSocket"`
	Connectable bool   `json:"connectable"`
	Locked      bool   `json:"locked"`
	Running     bool   `json:"running"`
	Stale       bool   `json:"stale"`
	Mode        string `json:"mode,omitempty"`
	Error       string `json:"error,omitempty"`
}

type Listener struct {
	path     string
	lockPath string
	lockFile *os.File
	listener net.Listener
	once     sync.Once
}

func ResolvePath(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Clean(explicit), nil
	}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "pipeferry", "pipeferry.sock"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "run", "pipeferry", "pipeferry.sock"), nil
}

func Listen(path string, mode fs.FileMode) (*Listener, error) {
	if mode.Perm() == 0 || mode.Perm()&0o077 != 0 {
		return nil, fmt.Errorf("socket mode must grant only owner permissions")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return nil, fmt.Errorf("secure socket directory: %w", err)
	}

	result := &Listener{path: path, lockPath: path + ".lock"}
	if err := result.acquire(); err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = result.Cleanup()
		}
	}()
	if err := prepareSocketPath(path); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on unix socket: %w", err)
	}
	result.listener = listener
	if err := os.Chmod(path, mode.Perm()); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("set socket mode: %w", err)
	}
	ok = true
	return result, nil
}

func (l *Listener) acquire() error {
	file, err := os.OpenFile(l.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open instance lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrAlreadyRunning
		}
		return fmt.Errorf("acquire instance lock: %w", err)
	}
	l.lockFile = file
	return nil
}

func prepareSocketPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect socket path: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("socket path exists and is not a unix socket")
	}
	if !ownedByUID(info, uint32(os.Geteuid())) {
		return fmt.Errorf("refusing to remove unix socket not owned by current user")
	}
	conn, dialErr := net.DialTimeout("unix", path, 150*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		return ErrAlreadyRunning
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale unix socket: %w", err)
	}
	return nil
}

func (l *Listener) Accept() (net.Conn, error) {
	return l.listener.Accept()
}

func (l *Listener) Close() error {
	if l.listener == nil {
		return nil
	}
	return l.listener.Close()
}

func (l *Listener) Cleanup() error {
	var cleanupErr error
	l.once.Do(func() {
		if l.listener != nil {
			_ = l.listener.Close()
		}
		if err := removeOwnedSocket(l.path); err != nil {
			cleanupErr = err
		}
		if l.lockFile != nil {
			_ = syscall.Flock(int(l.lockFile.Fd()), syscall.LOCK_UN)
			_ = l.lockFile.Close()
		}
		if err := os.Remove(l.lockPath); err != nil && !errors.Is(err, os.ErrNotExist) && cleanupErr == nil {
			cleanupErr = err
		}
	})
	return cleanupErr
}

func removeOwnedSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil
	}
	if !ownedByUID(info, uint32(os.Geteuid())) {
		return nil
	}
	return os.Remove(path)
}

func ownedByUID(info fs.FileInfo, uid uint32) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uid
}

func Inspect(path string) Status {
	result := Status{SocketPath: path}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return result
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Exists = true
	result.IsSocket = info.Mode()&os.ModeSocket != 0
	result.Mode = "0" + strconv.FormatUint(uint64(info.Mode().Perm()), 8)
	if !result.IsSocket {
		result.Error = "path is not a unix socket"
		return result
	}
	conn, err := net.DialTimeout("unix", path, 150*time.Millisecond)
	if err == nil {
		result.Connectable = true
		result.Running = true
		_ = conn.Close()
	} else {
		result.Stale = true
		result.Error = err.Error()
	}
	lockFile, lockErr := os.OpenFile(path+".lock", os.O_RDWR, 0o600)
	if lockErr == nil {
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			result.Locked = true
		} else {
			_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		}
		_ = lockFile.Close()
	}
	return result
}

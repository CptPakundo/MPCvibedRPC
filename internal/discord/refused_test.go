package discord

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// A connection that is there but may not be opened (Discord running as administrator) is told apart from a Discord
// that is not running. Checked with a Unix socket nobody may open; a Windows pipe that refuses needs a security
// descriptor, so the Windows run only checks the plain "not running" case.
func TestLoginRefused(t *testing.T) {
	missing := New([]string{filepath.Join(t.TempDir(), "discord-ipc-0")})
	if err := missing.Login("1", time.Second); err == nil || errors.Is(err, ErrRefused) {
		t.Fatalf("no socket: %v", err)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return // root opens any socket
	}
	dir, err := os.MkdirTemp("/tmp", "dr") // short: macOS allows 104 bytes for a socket path
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "discord-ipc-0")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if err := New([]string{path}).Login("1", time.Second); !errors.Is(err, ErrRefused) {
		t.Fatalf("a socket that may not be opened: %v", err)
	}
}

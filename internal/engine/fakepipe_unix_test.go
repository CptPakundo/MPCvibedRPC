//go:build !windows

package engine

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakePath is where the fake Discord listens: a Unix socket in the test's folder.
func fakePath(dir string) string { return filepath.Join(dir, "discord-ipc-0") }

type unixListener struct{ ln net.Listener }

func (u unixListener) Accept() (io.ReadWriteCloser, error) { return u.ln.Accept() }
func (u unixListener) Close() error                        { return u.ln.Close() }

// listenFake starts the fake Discord's listener.
func listenFake(path string) (fakeListener, error) {
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	return unixListener{ln}, nil
}

// sockDir is a folder for a test's sockets. macOS allows only 104 bytes for a socket path and its temporary folder is
// long, so there the folder is made under /tmp.
func sockDir(t *testing.T) string {
	if runtime.GOOS != "darwin" {
		return t.TempDir()
	}
	d, err := os.MkdirTemp("/tmp", "mr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

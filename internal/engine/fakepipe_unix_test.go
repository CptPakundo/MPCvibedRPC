//go:build !windows

package engine

import (
	"io"
	"net"
	"path/filepath"
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

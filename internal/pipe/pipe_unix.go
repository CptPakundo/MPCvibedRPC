//go:build !windows

package pipe

import (
	"io"
	"net"
	"os"
	"time"
)

// Dial connects to a Unix socket.
func Dial(path string) (io.ReadWriteCloser, error) {
	return net.DialTimeout("unix", path, 2*time.Second)
}

// Exists reports whether a socket is there, without connecting to it.
func Exists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}

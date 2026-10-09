//go:build !windows

package pipe

import (
	"io"
	"net"
	"time"
)

// Dial connects to a Unix socket.
func Dial(path string) (io.ReadWriteCloser, error) {
	return net.DialTimeout("unix", path, 2*time.Second)
}

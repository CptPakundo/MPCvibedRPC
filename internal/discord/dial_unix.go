//go:build !windows

package discord

import (
	"io"
	"net"
	"time"
)

func dial(path string) (io.ReadWriteCloser, error) {
	return net.DialTimeout("unix", path, 2*time.Second)
}

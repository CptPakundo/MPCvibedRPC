//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package winsys

import (
	"os"
	"path/filepath"
	"syscall"
)

var lockFile *os.File // kept open (and locked) for the life of the process

// AcquireSingleInstance locks a file in the data folder dir. It returns false when another copy holds the lock. The
// system releases the lock when the process ends, however it ends.
func AcquireSingleInstance(dir string) bool {
	if lockFile != nil {
		return true
	}
	f, err := os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return true // cannot tell: do not lock ourselves out
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return err != syscall.EWOULDBLOCK
	}
	lockFile = f
	return true
}

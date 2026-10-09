//go:build !windows && !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package winsys

// AcquireSingleInstance cannot lock on this system: the ping of a running copy is all there is.
func AcquireSingleInstance(dir string) bool { return true }

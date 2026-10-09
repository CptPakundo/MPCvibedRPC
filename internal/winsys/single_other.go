//go:build !windows

package winsys

// AcquireSingleInstance is only meaningful on Windows (other systems rely on the ping of the running copy).
func AcquireSingleInstance() bool { return true }

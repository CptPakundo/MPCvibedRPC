//go:build windows

package winsys

import (
	"syscall"
	"unsafe"
)

// instanceMutex is the mutex this copy holds (0: none).
var instanceMutex syscall.Handle

// AcquireSingleInstance takes a process-wide named mutex. It returns false when another copy already holds it. (The
// data folder dir is used by the other systems, which lock a file in it.) The handle stays open: Windows releases the
// mutex when the process ends, and that is what keeps a second copy out until then (or until ReleaseSingleInstance).
func AcquireSingleInstance(dir string) bool {
	// The default install keeps the name it always had; a copy with a data folder of its own (MPCRPC_HOME) gets its
	// own, like its autostart value, so it can run next to an installed copy.
	name, _ := syscall.UTF16PtrFromString(`Local\` + runName() + `-single-instance`)
	h, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true // cannot tell: do not lock ourselves out
	}
	if errno, ok := err.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
		syscall.CloseHandle(syscall.Handle(h))
		return false
	}
	instanceMutex = syscall.Handle(h)
	return true
}

// ReleaseSingleInstance lets another copy start while this one still runs: after an update the old copy stays for a
// moment to see that the new one comes up, and the new one waits for the mutex.
func ReleaseSingleInstance() {
	if instanceMutex != 0 {
		syscall.CloseHandle(instanceMutex)
		instanceMutex = 0
	}
}

//go:build windows

package winsys

import (
	"syscall"
	"unsafe"
)

var held uintptr // kept for the life of the process

// AcquireSingleInstance takes a process-wide named mutex. It returns false when another copy already holds it. (The
// data folder dir is used by the other systems, which lock a file in it.)
func AcquireSingleInstance(dir string) bool {
	name, _ := syscall.UTF16PtrFromString(`Local\MPCvibedRPC-single-instance`)
	h, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true // cannot tell: do not lock ourselves out
	}
	if errno, ok := err.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
		syscall.CloseHandle(syscall.Handle(h))
		return false
	}
	held = h
	return true
}

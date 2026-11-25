//go:build windows

package winsys

import (
	"syscall"
	"unsafe"
)

// instanceMutexes are the mutexes this copy holds (none: nil).
var instanceMutexes []syscall.Handle

// instanceNames are the names of the mutexes that keep a second copy out. The default install uses the program's name;
// a copy with a data folder of its own (MPRP_HOME) gets its own, like its autostart value, so it can run
// next to an installed copy. The name under the program's former name is held too: a copy of the former version that
// updates itself to this one releases that mutex as it hands over, and this copy waits for it as it always did.
func instanceNames() []string {
	return []string{`Local\` + runName() + `-single-instance`, `Local\` + formerRunName() + `-single-instance`}
}

// AcquireSingleInstance takes the process-wide named mutexes. It returns false when another copy already holds one of
// them. (The data folder dir is used by the other systems, which lock a file in it.) The handles stay open: Windows
// releases the mutexes when the process ends, and that is what keeps a second copy out until then (or until
// ReleaseSingleInstance).
func AcquireSingleInstance(dir string) bool {
	if instanceMutexes != nil {
		return true
	}
	var held []syscall.Handle
	for _, n := range instanceNames() {
		name, _ := syscall.UTF16PtrFromString(n)
		h, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
		if h == 0 {
			continue // cannot tell: do not lock ourselves out
		}
		if errno, ok := err.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
			syscall.CloseHandle(syscall.Handle(h))
			for _, o := range held {
				syscall.CloseHandle(o)
			}
			return false
		}
		held = append(held, syscall.Handle(h))
	}
	instanceMutexes = held
	if instanceMutexes == nil {
		instanceMutexes = []syscall.Handle{}
	}
	return true
}

// ReleaseSingleInstance lets another copy start while this one still runs: after an update the old copy stays for a
// moment to see that the new one comes up, and the new one waits for the mutexes.
func ReleaseSingleInstance() {
	for _, h := range instanceMutexes {
		syscall.CloseHandle(h)
	}
	instanceMutexes = nil
}

package main

import (
	"syscall"
	"unsafe"
)

var pMoveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

// renameNoReplace renames from to to, failing when to exists (os.Rename would replace it). Windows lets a running
// program's file be renamed.
func renameNoReplace(from, to string) error {
	f, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	if err := pMoveFileEx.Find(); err != nil {
		return err
	}
	if r, _, e := pMoveFileEx.Call(uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(t)), 0); r == 0 {
		return e
	}
	return nil
}

package pipe

import (
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func listenPipe(t *testing.T, path string) func() {
	t.Helper()
	path = `\\.\pipe\` + strings.TrimPrefix(path, `\\?\pipe\`) // CreateNamedPipe only takes the \\.\ form
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	create := kernel32.NewProc("CreateNamedPipeW")
	const pipeAccessDuplex, unlimited = 0x3, 255
	h, _, e := create.Call(uintptr(unsafe.Pointer(p)), pipeAccessDuplex, 0, unlimited, 4096, 4096, 0, 0)
	if syscall.Handle(h) == syscall.InvalidHandle {
		t.Fatal(e)
	}
	return func() { syscall.CloseHandle(syscall.Handle(h)) }
}

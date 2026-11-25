package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// openNoDelete opens a file the way a reader without FILE_SHARE_DELETE does (PowerShell's Get-Content, for one):
// while it is open, Windows refuses to delete the file.
func openNoDelete(t *testing.T, path string) syscall.Handle {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// A reader holding ipc.json for a moment does not leave it behind: removeIPC tries again until the reader lets go.
func TestRemoveIPCWhileRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ipc.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	h := openNoDelete(t, path)
	if err := os.Remove(path); err == nil {
		t.Fatal("Windows deleted a file held open without delete sharing; the test proves nothing")
	}
	time.AfterFunc(200*time.Millisecond, func() { syscall.CloseHandle(h) })
	if err := removeIPC(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
}

// A reader that never lets go makes removeIPC give up after ipcRemoveWait with the error.
func TestRemoveIPCGivesUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ipc.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	h := openNoDelete(t, path)
	defer syscall.CloseHandle(h)
	old := ipcRemoveWait
	ipcRemoveWait = 150 * time.Millisecond
	defer func() { ipcRemoveWait = old }()
	start := time.Now()
	if err := removeIPC(path); err == nil {
		t.Fatal("removed a file that is held open")
	}
	if d := time.Since(start); d < 150*time.Millisecond || d > 2*time.Second {
		t.Fatalf("gave up after %v", d)
	}
}

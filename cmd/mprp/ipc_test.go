package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// removeIPC deletes the file, and a file that is already gone is no error and costs no wait.
func TestRemoveIPC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ipc.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeIPC(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	start := time.Now()
	if err := removeIPC(path); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if d := time.Since(start); d >= ipcRemoveWait/2 {
		t.Fatalf("missing file waited %v", d)
	}
}

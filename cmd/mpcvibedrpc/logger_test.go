package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A log that passes its limit while the program runs is moved to .old and started again.
func TestLogRotatesWhileRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	l := newLogger(path)
	defer l.close()
	line := strings.Repeat("x", 1000)
	for i := 0; i < 1100; i++ {
		l.log("INFO", line)
	}
	old, err := os.Stat(path + ".old")
	if err != nil || old.Size() <= logLimit {
		t.Fatalf("no full .old file: %v", err)
	}
	cur, err := os.Stat(path)
	if err != nil || cur.Size() >= logLimit/2 {
		t.Fatalf("the log did not start again: %v", err)
	}
}

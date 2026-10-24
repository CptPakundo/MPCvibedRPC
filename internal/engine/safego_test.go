package engine

import (
	"strings"
	"sync"
	"testing"
)

// A panic in a background goroutine is logged and ends only that goroutine.
func TestGoLogsAPanicInsteadOfCrashing(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var lines []string
	done := make(chan struct{})
	log := func(level, msg string) {
		mu.Lock()
		lines = append(lines, level+" "+msg)
		mu.Unlock()
		close(done)
	}
	Go(log, "the sample watch", func() { panic("boom") })
	<-done
	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 1 || !strings.Contains(lines[0], "ERROR the sample watch stopped after an error: boom") {
		t.Fatalf("got %q", lines)
	}
	ran := make(chan struct{})
	Go(nil, "quiet", func() { close(ran) }) // a nil log is fine
	<-ran
}

package capture

import (
	"context"
	"testing"
	"time"
)

// The record is Windows' own and is only read. Whatever it holds on this computer (nothing, on a system without
// it), reading must work and must agree with itself; what it says is logged, not judged.
func TestReadsTheRealRecord(t *testing.T) {
	entries := read()
	if !Supported() && len(entries) != 0 {
		t.Errorf("%d records were read from a store that is said to be missing", len(entries))
	}
	live := 0
	for _, e := range entries {
		if !isDiscord(e.path) {
			t.Errorf("a record of another program was read")
		}
		if !e.start.IsZero() && e.stop.IsZero() {
			live++
		}
		if !e.stop.IsZero() && e.stop.Before(e.start) {
			t.Errorf("a record stops before it starts: %v, %v", e.start, e.stop)
		}
	}
	t.Logf("supported=%v, Discord records=%d, without a stop time=%d, sharing now=%v", Supported(), len(entries), live, DiscordSharing())
}

// Watch ends when its context does.
func TestWatchEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Watch(ctx, func() {}); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not end with its context")
	}
}

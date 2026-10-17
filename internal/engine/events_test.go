package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// With mpv's events, a pause shows at once although the next regular check is a minute away.
func TestMpvPauseShowsAtOnce(t *testing.T) {
	m := newFakeMpv(t, map[string]any{
		"path": "/v/Sample Movie (2001).mkv", "filename": "Sample Movie (2001).mkv",
		"pause": false, "time-pos": 60.0, "duration": 1500.0, "speed": 1.0, "idle-active": false,
	})
	e, d, _ := newTestEngine(t, withPort(deadPort(t)), withPipes([]PlayerPipe{{"mpv", m.path}}), func(cfg *core.Config, _ *Options) {
		cfg.PollInterval = 60000
	})
	e.Start()
	// the first check finds mpv and connects to Discord; the next regular one would be a minute later
	eventually(t, "connected and watching", func() bool { return e.Status().Discord == "connected" && m.watchers() == 1 })
	n := len(d.list())

	m.set("pause", true)
	start := time.Now()
	m.event(`{"event":"property-change","id":1,"name":"pause","data":true}`)
	eventually(t, "the pause sent", func() bool { return len(d.list()) > n && e.Status().Paused })
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("the pause took %v to show", d)
	}
	if s := lastDetails(d); !strings.Contains(s, "1:00 / 25:00") {
		t.Errorf("last status: %q", s)
	}
}

// A burst of events (seeking) makes one early check, and early checks are at least nudgeGap apart.
func TestNudgesArePaced(t *testing.T) {
	e := &Engine{wake: make(chan struct{}, 1)}
	wakes := 0
	count := func(d time.Duration) {
		deadline := time.After(d)
		for {
			select {
			case <-e.wake:
				wakes++
			case <-deadline:
				return
			}
		}
	}
	for i := 0; i < 20; i++ {
		e.nudge()
	}
	count(nudgeDelay + 500*time.Millisecond)
	if wakes != 1 {
		t.Fatalf("%d checks after a burst; want 1", wakes)
	}
	e.nudge() // right after: waits for the gap
	count(time.Second)
	if wakes != 1 {
		t.Fatalf("an early check came within the gap (%d)", wakes)
	}
	count(nudgeGap)
	if wakes != 2 {
		t.Fatalf("%d checks; the waiting one should come after the gap", wakes)
	}
}

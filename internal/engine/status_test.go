package engine

import (
	"strings"
	"testing"
	"time"
)

// Discord is only contacted once something plays: until then the window must not claim it is missing.
func TestDiscordOnStandbyUntilSomethingPlays(t *testing.T) {
	e, m, _, _ := setup(t)
	m.set(func(m *mpc) { m.state = -1 })
	e.Start()
	time.Sleep(600 * time.Millisecond)
	if s := e.Status(); s.Discord != "standby" {
		t.Fatalf("nothing played yet: %+v", s)
	}
	m.set(func(m *mpc) { m.state = 2 })
	eventually(t, "connected", func() bool { return e.Status().Discord == "connected" })
}

// Saving settings that did not change for the engine (app-only options) must not restart it.
func TestUnchangedSettingsDoNotRestart(t *testing.T) {
	e, _, _, l := setup(t)
	e.Start()
	eventually(t, "shown", func() bool { return e.Status().NowPlaying != nil })
	e.ApplySettings(e.Config())
	time.Sleep(300 * time.Millisecond)
	if n := strings.Count(strings.Join(l.list(), "\n"), "Presence stopped."); n != 0 || e.Status().NowPlaying == nil {
		t.Fatalf("restarted %d times for nothing", n)
	}
	cfg := e.Config()
	cfg.ProgressWidth++
	e.ApplySettings(cfg)
	eventually(t, "restart for a real change", func() bool { return strings.Contains(strings.Join(l.list(), "\n"), "Presence stopped.") })
}

func (l *logs) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.l...)
}

func TestPausedFlagClearsWhenPlaybackEnds(t *testing.T) {
	e, m, _, _ := setup(t)
	e.Start()
	m.set(func(m *mpc) { m.state = 1 })
	eventually(t, "paused", func() bool { return e.Status().Paused })
	m.set(func(m *mpc) { m.state = -1 })
	eventually(t, "nothing playing, not paused", func() bool { s := e.Status(); return s.NowPlaying == nil && !s.Paused })
	e.Stop()
	if e.Status().Paused {
		t.Fatal("a stopped engine reports paused")
	}
}

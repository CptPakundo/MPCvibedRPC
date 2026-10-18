package engine

import (
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
)

// clears counts the SET_ACTIVITY calls without an activity (a cleared status) from the n-th call on.
func clears(d *disc, from int) int {
	n := 0
	for i, a := range d.list() {
		if _, has := a["activity"]; i >= from && !has {
			n++
		}
	}
	return n
}

// A busy Discord that applies the status but answers after the timeout: the player then quits. The status must still
// be cleared (it was left on the profile when the program took the late answer for "not shown").
func TestStatusClearedAfterALateAnswer(t *testing.T) {
	defer func(d time.Duration) { discord.RequestTimeout = d }(discord.RequestTimeout)
	discord.RequestTimeout = 200 * time.Millisecond
	e, m, d, l := setup(t)
	d.mu.Lock()
	d.slow = 600 * time.Millisecond
	d.mu.Unlock()
	e.Start()
	eventually(t, "the status reached Discord", func() bool { return len(d.list()) >= 1 })
	m.set(func(m *mpc) { m.down = true }) // the player quits before a resend gets through
	d.mu.Lock()
	d.slow = 0
	d.mu.Unlock()
	eventually(t, "a late answer seen", func() bool { return l.has("did not answer in time") })
	eventually(t, "the status cleared", func() bool { return clears(d, 1) >= 1 })
}

// A clear Discord does not take is tried again until one works, and logged once.
func TestFailedClearIsRetried(t *testing.T) {
	e, m, d, l := setup(t)
	e.Start()
	eventually(t, "shown", func() bool { return e.Status().NowPlaying != nil })
	d.mu.Lock()
	d.failClears = true
	d.mu.Unlock()
	n := len(d.list())
	m.set(func(m *mpc) { m.down = true })
	eventually(t, "two clears tried", func() bool { return clears(d, n) >= 2 })
	if !l.has("Discord did not take the clearing of the status") {
		t.Fatal(l.l)
	}
	d.mu.Lock()
	d.failClears = false
	d.mu.Unlock()
	eventually(t, "cleared", func() bool { return l.has("Nothing playing - presence cleared.") })
	k := len(d.list())
	time.Sleep(600 * time.Millisecond) // more checks: no clear storm once one worked
	if extra := clears(d, k); extra > 0 {
		t.Fatalf("%d more clears right after one worked", extra)
	}
}

// While nothing plays and Discord is connected, the status is cleared again now and then, without a log line.
func TestNothingPlayingIsClearedAgainNowAndThen(t *testing.T) {
	defer func(d time.Duration) { reclearEvery = d }(reclearEvery)
	reclearEvery = 300 * time.Millisecond
	e, m, d, l := setup(t)
	e.Start()
	eventually(t, "shown", func() bool { return e.Status().NowPlaying != nil })
	n := len(d.list())
	m.set(func(m *mpc) { m.down = true })
	eventually(t, "cleared", func() bool { return clears(d, n) >= 1 })
	eventually(t, "cleared again", func() bool { return clears(d, n) >= 3 })
	cleared := 0
	l.mu.Lock()
	for _, s := range l.l {
		if s == "INFO Nothing playing - presence cleared." {
			cleared++
		}
	}
	l.mu.Unlock()
	if cleared != 1 {
		t.Fatalf("logged %d times; the repeats are quiet", cleared)
	}
}

// A player paused at the very end of its file (some stay on the last frame) has finished the video: the status is
// cleared at once instead of showing "Paused" until the pause limit.
func TestPausedAtTheEndClears(t *testing.T) {
	e, m, d, l := setup(t)
	e.Start()
	eventually(t, "shown", func() bool { return e.Status().NowPlaying != nil })
	n := len(d.list())
	m.set(func(m *mpc) { m.state, m.pos = 1, m.dur-1000 })
	eventually(t, "cleared", func() bool { return clears(d, n) >= 1 })
	// Discord gets the clear a moment before the log and the status say so
	eventually(t, "logged as ended", func() bool { return l.has("The video ended - presence cleared.") && e.Status().NowPlaying == nil })
	m.set(func(m *mpc) { m.state, m.pos = 2, 60000 }) // played again from earlier on: shown again
	eventually(t, "shown again", func() bool { return e.Status().NowPlaying != nil })
}

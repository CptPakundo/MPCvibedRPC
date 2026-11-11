package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// A Discord that refuses every status is not asked at every check: the engine waits longer after each failure, names
// the failure once in the log, and shows the status as soon as Discord takes it again.
func TestFailedSendsArePaced(t *testing.T) {
	t.Parallel()
	e, _, d, l := setup(t)
	d.set(func(d *disc) { d.rejectAll = true })
	e.Start()
	failures := func() (n int) {
		l.mu.Lock()
		defer l.mu.Unlock()
		for _, s := range l.l {
			if strings.Contains(s, "Failed to update presence") {
				n++
			}
		}
		return n
	}
	eventually(t, "a first refusal", func() bool { return failures() >= 1 })
	eventually(t, "the refusal in the window", func() bool { return e.Status().LastError != nil })
	// 40 checks pass: unpaced, each would send. The waits are 4, 8, 16 and 32 checks long (Options.RetryAfter).
	time.Sleep(40 * testPoll)
	if n := len(d.list()); n < 3 || n > 9 {
		t.Errorf("%d requests in 40 checks: a refused status is tried again, but after a growing wait", n)
	}
	if n := failures(); n != 1 {
		t.Errorf("the same failure was logged %d times", n)
	}
	d.set(func(d *disc) { d.rejectAll = false })
	eventually(t, "the status once Discord takes it", func() bool {
		s := e.Status()
		return s.NowPlaying != nil && s.LastError == nil
	})
}

// The progress bar is counted from the moment the player reported its position, not from the moment the title's
// lookup is done.
func TestProgressCountsFromWhenThePlayerAnswered(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000)
	info := &core.Info{File: "Sample Movie (2020).mkv", State: 2, Position: 60000, Duration: 6000000, Rate: 1}
	for _, hide := range []bool{false, true} {
		cfg := core.DefaultConfig()
		cfg.HideTitle = hide
		b := buildActivity(info, &cfg, nil, nil, at)
		if b.activity == nil || b.activity.Timestamps == nil {
			t.Fatalf("hidden=%v: %+v", hide, b.activity)
		}
		if ts := b.activity.Timestamps; ts.Start != at.UnixMilli()-60000 || ts.End != at.UnixMilli()+5940000 {
			t.Errorf("hidden=%v: start=%d end=%d", hide, ts.Start, ts.End)
		}
	}
}

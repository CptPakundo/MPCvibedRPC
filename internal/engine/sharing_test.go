package engine

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// No test reads this computer's record of Discord's screen capture: a test that wants a stream sets Options.Sharing.
func init() {
	defaultSharing = func() bool { return false }
	defaultWatchSharing = func(context.Context, func()) {}
}

// With the stream title on "auto" (the default) the status is the usual Watching one, turns into "Playing <title>"
// while Discord shares the screen, and turns back when the stream ends. The same file is sent again each time.
func TestStatusFollowsTheScreenShare(t *testing.T) {
	t.Parallel()
	var sharing atomic.Bool
	m := newMPC(t)
	e, d, l := newTestEngine(t, withPort(m.port), func(_ *core.Config, o *Options) { o.Sharing = sharing.Load })
	e.Start()
	eventually(t, "the Watching status", func() bool {
		s := d.showing()
		return len(s) == 1 && s[0]["type"] == float64(3)
	})
	sharing.Store(true)
	eventually(t, "the Playing status while the screen is shared", func() bool {
		s := d.showing()
		return len(s) == 1 && s[0]["type"] == nil && s[0]["name"] == "Show Name" && l.has("screen share began")
	})
	if p := e.Status().Preview; p == nil || p.Watching {
		t.Errorf("the window's preview should show the Playing form: %+v", p)
	}
	sharing.Store(false)
	eventually(t, "the Watching status again", func() bool {
		s := d.showing()
		return len(s) == 1 && s[0]["type"] == float64(3) && l.has("screen share ended")
	})
}

// "off" never asks whether the screen is shared, and "always" does not need to.
func TestStreamTitleOffAndAlwaysDoNotAsk(t *testing.T) {
	t.Parallel()
	for mode, wantType := range map[string]any{"off": float64(3), "always": nil} {
		var asked atomic.Int32
		m := newMPC(t)
		e, d, _ := newTestEngine(t, withPort(m.port), func(cfg *core.Config, o *Options) {
			cfg.StreamTitle = mode
			o.Sharing = func() bool { asked.Add(1); return true }
		})
		e.Start()
		eventually(t, mode+": a status", func() bool { return len(d.showing()) == 1 })
		if s := d.showing(); s[0]["type"] != wantType {
			t.Errorf("%s: type %v, want %v", mode, s[0]["type"], wantType)
		}
		if asked.Load() != 0 {
			t.Errorf("%s: the screen share was asked about %d times", mode, asked.Load())
		}
		e.Stop()
	}
}

// A hidden title stays hidden while the screen is shared.
func TestHiddenTitleIgnoresTheScreenShare(t *testing.T) {
	t.Parallel()
	m := newMPC(t)
	e, d, _ := newTestEngine(t, withPort(m.port), func(cfg *core.Config, o *Options) {
		cfg.HideTitle = true
		o.Sharing = func() bool { return true }
	})
	e.Start()
	eventually(t, "the hidden status", func() bool { return len(d.showing()) == 1 })
	if s := d.showing()[0]; s["type"] != float64(3) || s["name"] != nil || s["details"] != "Watching a video" {
		t.Errorf("while sharing, the hidden status is %v", s)
	}
}

package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// No test reads this computer's record of Discord's screen capture: a test that wants a stream sets Options.Sharing.
func init() {
	defaultSharing = func() bool { return false }
	defaultWatchSharing = func(context.Context, func()) {}
}

// With the stream title on "auto" (the default) the status is the usual Watching one. While Discord shares the
// screen it stays, and a second connection adds "Playing <title>" for the stream's name, without a button or links;
// when the stream ends that one goes, and the Watching status is still there.
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
	eventually(t, "Watching and Playing while the screen is shared", func() bool {
		w, p := byType(d.showing())
		return w != nil && p != nil && p["name"] == "Show Name" && l.has("screen share began")
	})
	// the Watching status goes after the stream status, so that it is the newer one
	eventually(t, "the Watching status sent after the stream status", func() bool {
		lastW, lastP := -1, -1
		for i, args := range d.list() {
			if act, _ := args["activity"].(map[string]any); act != nil {
				if act["type"] == float64(3) {
					lastW = i
				} else if act["type"] == nil {
					lastP = i
				}
			}
		}
		return lastP >= 0 && lastW > lastP
	})
	if _, p := byType(d.showing()); p["buttons"] != nil || p["details_url"] != nil || p["state_url"] != nil {
		t.Errorf("the stream status carries links: %v", p)
	}
	if p := e.Status().Preview; p == nil || !p.Watching {
		t.Errorf("the preview shows %+v, want the Watching status", p)
	}
	sharing.Store(false)
	eventually(t, "the Watching status alone again", func() bool {
		s := d.showing()
		return len(s) == 1 && s[0]["type"] == float64(3) && l.has("screen share ended")
	})
	sharing.Store(true)
	eventually(t, "the stream status again", func() bool { return len(d.showing()) == 2 })
	m.set(func(m *mpc) { m.state = 0 }) // nothing plays: both go
	eventually(t, "nothing shown once the video stops", func() bool { return len(d.showing()) == 0 })
	m.set(func(m *mpc) { m.state = 2 })
	eventually(t, "both back", func() bool { return len(d.showing()) == 2 })
	e.Stop()
	eventually(t, "nothing shown after Stop", func() bool { return len(d.showing()) == 0 })
}

// With the Playing style the status is a Playing one already: while the screen is shared it is sent in the stream
// form, on the one connection, as before.
func TestPlayingStyleStreamsOnOneStatus(t *testing.T) {
	t.Parallel()
	m := newMPC(t)
	e, d, _ := newTestEngine(t, withPort(m.port), func(cfg *core.Config, o *Options) {
		cfg.ActivityType = "playing"
		o.Sharing = func() bool { return true }
	})
	e.Start()
	eventually(t, "the Playing status named after the title", func() bool {
		s := d.showing()
		return len(s) == 1 && s[0]["type"] == nil && s[0]["name"] == "Show Name"
	})
	time.Sleep(200 * time.Millisecond)
	if n := len(d.showing()); n != 1 {
		t.Errorf("%d statuses with the Playing style, want 1", n)
	}
}

// byType picks the Watching and the Playing status out of what the fake Discord shows.
func byType(s []map[string]any) (watching, playing map[string]any) {
	for _, a := range s {
		switch a["type"] {
		case float64(3):
			watching = a
		case nil:
			playing = a
		}
	}
	return watching, playing
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

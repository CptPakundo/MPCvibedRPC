package engine

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// remoteRig is an engine that follows one remote (Plex, Stremio, Jellyfin or Emby) whose fake shows "Sample Show".
type remoteRig struct {
	name  string // the player the window shows
	e     *Engine
	play  func()     // makes the remote report the episode as playing
	stop  func()     // makes the remote report that it stopped (nil: the fake cannot)
	count func() int // the requests the fake got
}

// remoteRigs make a rig on the given port; mods change the engine before it is made.
var remoteRigs = []struct {
	name  string
	build func(t *testing.T, port int, mods ...testMod) *remoteRig
}{
	{"Plex", func(t *testing.T, port int, mods ...testMod) *remoteRig {
		p := newFakePlex(t)
		e, _, _ := newTestEngine(t, append([]testMod{withPort(port), shortPause, withPlex(p)}, mods...)...)
		return &remoteRig{"Plex", e, func() { p.play(60000, "playing") }, func() { p.play(60000, "stopped") }, p.count}
	}},
	{"Stremio", func(t *testing.T, port int, mods ...testMod) *remoteRig {
		s := newFakeStremio(t)
		s.play("Sample Show", "tt0000002:1:2", 600_000)
		e, _, _ := newTestEngine(t, append([]testMod{withPort(port), shortPause, withStremio(s)}, mods...)...)
		return &remoteRig{"Stremio", e, func() {}, nil, s.count}
	}},
	{"Jellyfin", func(t *testing.T, port int, mods ...testMod) *remoteRig {
		f := newFakeJellyfin(t)
		e, _, _ := newTestEngine(t, append([]testMod{withPort(port), shortPause, withJellyfin(f, "Jellyfin")}, mods...)...)
		return &remoteRig{"Jellyfin", e, func() {}, nil, f.count}
	}},
}

// A remote is asked after the players on this computer: one that plays wins, and the remote comes back when it stops.
func TestRemotesInThePlayerOrder(t *testing.T) {
	for _, tc := range remoteRigs {
		t.Run(tc.name, func(t *testing.T) {
			m := newMPC(t)
			rig := tc.build(t, m.port)
			rig.e.Start()
			eventually(t, "MPC-HC", func() bool { s := rig.e.Status(); return s.Player == "MPC-HC" && s.NowPlaying != nil })
			rig.play()
			eventually(t, "the remote asked", func() bool { return rig.count() >= 2 })
			time.Sleep(600 * time.Millisecond)
			if st := rig.e.Status(); st.Player != "MPC-HC" {
				t.Fatalf("%s won over a playing MPC-HC: %+v", rig.name, st)
			}
			m.set(func(m *mpc) { m.state = 0; m.file = ""; m.filepath = "" })
			eventually(t, rig.name, func() bool {
				s := rig.e.Status()
				return s.Player == rig.name && s.NowPlaying != nil && strings.Contains(*s.NowPlaying, "Sample Show")
			})
			if rig.stop != nil {
				// an idle MPC-HC that answers is reported before an idle remote
				rig.stop()
				eventually(t, "idle MPC-HC", func() bool { s := rig.e.Status(); return s.Player == "MPC-HC" && s.NowPlaying == nil })
			}
		})
	}
}

// Resting stops the watcher (no more requests) and starts it again when it ends.
func TestRemotesStopWhileResting(t *testing.T) {
	for _, tc := range remoteRigs {
		t.Run(tc.name, func(t *testing.T) {
			var mayRest atomic.Bool
			// the fake Discord of the test engine is not the one asked, so Discord looks closed
			rig := tc.build(t, deadPort(t), func(_ *core.Config, o *Options) {
				o.DiscordPaths = []string{fakePath(sockDir(t))}
				o.MayRest = mayRest.Load
			})
			rig.e.Start()
			eventually(t, "the remote asked", func() bool { return rig.count() >= 1 })
			mayRest.Store(true)
			eventually(t, "resting", func() bool { return rig.e.Status().Resting })
			time.Sleep(150 * time.Millisecond) // a request under way may still land
			n := rig.count()
			time.Sleep(500 * time.Millisecond)
			if rig.count() != n {
				t.Fatalf("%s was asked while resting", rig.name)
			}
			mayRest.Store(false)
			rig.e.Wake()
			eventually(t, "asked again", func() bool { return rig.count() > n })
		})
	}
}

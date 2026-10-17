package engine

import (
	"sync"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// fakePot stands in for the PotPlayer windows on this computer.
type fakePot struct {
	mu sync.Mutex
	in *core.Info // nil: no PotPlayer open
}

func (f *fakePot) set(in *core.Info) { f.mu.Lock(); f.in = in; f.mu.Unlock() }

func (f *fakePot) query() []*core.Info {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.in == nil {
		return nil
	}
	c := *f.in
	return []*core.Info{&c}
}

func withPot(f *fakePot) testMod { return func(_ *core.Config, o *Options) { o.PotPlayer = f.query } }

func potPlaying() *core.Info {
	return core.NewInfo("Sample Movie (2020).mkv", `D:\Videos\Sample Movie (2020).mkv`, "", 2, 60000, 1500000, 1)
}

// PotPlayer is shown under its own name; a video playing in a player asked earlier (mpv) comes first, and PotPlayer
// comes before the services (Plex).
func TestPotPlayerInThePlayerOrder(t *testing.T) {
	pot := &fakePot{}
	pot.set(potPlaying())
	m := newFakeMpv(t, map[string]any{"path": `D:\Videos\Sample Show - S01E02.mkv`, "pause": false, "time-pos": 5.0, "duration": 600.0, "speed": 1.0, "idle-active": false})
	p := newFakePlex(t)
	e, d, l := newTestEngine(t, withPort(deadPort(t)), withPipes([]PlayerPipe{{"mpv", m.path}}), withPot(pot), withPlex(p))
	e.Start()
	eventually(t, "mpv first", func() bool { s := e.Status(); return s.Player == "mpv" && s.NowPlaying != nil })

	m.set("idle-active", true) // mpv stops: PotPlayer is next, before Plex
	p.play(60000, "playing")
	eventually(t, "PotPlayer shown", func() bool { s := e.Status(); return s.Player == "PotPlayer" && s.NowPlaying != nil })
	if !l.has("PotPlayer detected.") || !l.has("PotPlayer") {
		t.Error("the log should name PotPlayer")
	}
	eventually(t, "sent with its title", func() bool {
		act := lastActivity(d)
		as, _ := act["assets"].(map[string]any)
		return act != nil && act["name"] == "Sample Movie" && as != nil && as["large_text"] == "PotPlayer" && as["large_image"] == core.PlayerIcon("PotPlayer") // no cover: the logo
	})

	pot.set(nil) // PotPlayer closed: Plex is shown
	eventually(t, "Plex shown", func() bool { s := e.Status(); return s.Player == "Plex" && s.NowPlaying != nil })
}

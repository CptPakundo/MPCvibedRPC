package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// The Never show list also matches where a server keeps the item: a Plex library's name, or a folder of the file's
// path on the Plex, Jellyfin or Emby server. Nothing is shown then, and the path never reaches the log.
func TestNeverShowMatchesServerLibraryAndPath(t *testing.T) {
	t.Parallel()
	hide := func(entries ...string) testMod {
		return func(cfg *core.Config, _ *Options) { cfg.HideFiles = entries }
	}
	for _, c := range []struct {
		name  string
		start func(t *testing.T) (*Engine, *disc, *logs, func())
	}{
		{"Plex library", func(t *testing.T) (*Engine, *disc, *logs, func()) {
			p := newFakePlex(t)
			e, d, l := newTestEngine(t, withPort(deadPort(t)), shortPause, withPlex(p), hide("TV"))
			return e, d, l, func() { p.play(60000, "playing") }
		}},
		{"Plex folder", func(t *testing.T) (*Engine, *disc, *logs, func()) {
			p := newFakePlex(t)
			e, d, l := newTestEngine(t, withPort(deadPort(t)), shortPause, withPlex(p), hide("/data/shows/*"))
			return e, d, l, func() { p.play(60000, "playing") }
		}},
		{"Jellyfin folder", func(t *testing.T) (*Engine, *disc, *logs, func()) {
			f := newFakeJellyfin(t)
			e, d, l := newTestEngine(t, withPort(deadPort(t)), shortPause, withJellyfin(f, "Jellyfin"), hide("Kids"))
			return e, d, l, func() {}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, d, l, play := c.start(t)
			e.Start()
			play()
			eventually(t, "hidden", func() bool { return e.Status().Hidden })
			for _, a := range d.list() {
				if act, _ := a["activity"].(map[string]any); act != nil && strings.Contains(fmt.Sprint(act["name"], act["details"]), "Sample") {
					t.Fatalf("a hidden item reached Discord: %v", act)
				}
			}
			for _, line := range l.l {
				if strings.Contains(line, "/data/") || strings.Contains(line, "/media/") {
					t.Fatalf("a server path reached the log: %s", line)
				}
			}
		})
	}

	// without a matching entry the same item shows as usual
	p := newFakePlex(t)
	e, d, _ := newTestEngine(t, withPort(deadPort(t)), shortPause, withPlex(p), hide("Movies"))
	e.Start()
	p.play(60000, "playing")
	eventually(t, "shown", func() bool { return len(d.list()) > 0 && !isClear(d) })
	if e.Status().Hidden {
		t.Fatal("hidden although no entry matches")
	}
}

package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

func (m *mpc) count() int { m.mu.Lock(); defer m.mu.Unlock(); return m.asked }

// While Discord is closed and the window is closed, nothing is asked; the window opening or Discord starting ends it.
func TestRestWhileDiscordIsClosed(t *testing.T) {
	t.Parallel()
	m := newMPC(t)
	path := fakePath(sockDir(t)) // the fake Discord starts later
	var mayRest atomic.Bool
	mayRest.Store(true)
	var rested atomic.Int32
	cfg := core.DefaultConfig()
	cfg.Port, cfg.PollInterval, cfg.ShowArtwork = m.port, 250, false
	l := &logs{}
	e := New(cfg, Options{Pipes: noPipes, MPRIS: noMPRIS, PotPlayer: noPotPlayer, Log: l.add, DiscordPaths: []string{path}, CacheFile: filepath.Join(t.TempDir(), "c.json"),
		MayRest: mayRest.Load, OnRest: func() { rested.Add(1) }})
	t.Cleanup(e.Stop)
	e.Start()
	eventually(t, "resting", func() bool { return e.Status().Resting })
	if !l.has("Discord is not running: resting") || rested.Load() != 1 {
		t.Fatalf("resting was not announced (OnRest %d times)", rested.Load())
	}
	if n := m.count(); n != 0 {
		t.Fatalf("the player was asked %d times while resting", n)
	}
	time.Sleep(time.Second) // four checks
	if n := m.count(); n != 0 || rested.Load() != 1 {
		t.Fatalf("while resting: player asked %d times, OnRest called %d times", n, rested.Load())
	}

	// the window opens: the player is asked at once (Wake), with Discord still closed
	mayRest.Store(false)
	e.Wake()
	eventually(t, "the player asked with the window open", func() bool { return m.count() > 0 && e.Status().MPC })
	if s := e.Status(); s.Resting || !l.has("Resting ended") {
		t.Fatalf("still resting with the window open: %+v", s)
	}
	// the window closes again
	mayRest.Store(true)
	eventually(t, "resting again", func() bool { return e.Status().Resting })
	if s := e.Status(); s.MPC || s.Player != "" {
		t.Fatalf("a resting status still names a player: %+v", s)
	}

	// Discord starts: presence follows the player again
	d := newDiscAt(t, path)
	eventually(t, "activity sent once Discord runs", func() bool { return len(d.list()) >= 1 })
	if s := e.Status(); s.Resting || s.Discord != "connected" {
		t.Fatalf("%+v", s)
	}
}

// Without MayRest (tests, and the behaviour before resting existed) the engine never rests.
func TestNoRestWithoutMayRest(t *testing.T) {
	t.Parallel()
	m := newMPC(t)
	cfg := core.DefaultConfig()
	cfg.Port, cfg.PollInterval, cfg.ShowArtwork = m.port, 250, false
	e := New(cfg, Options{Pipes: noPipes, MPRIS: noMPRIS, PotPlayer: noPotPlayer, DiscordPaths: []string{fakePath(sockDir(t))}, CacheFile: filepath.Join(t.TempDir(), "c.json")})
	t.Cleanup(e.Stop)
	e.Start()
	eventually(t, "the player asked", func() bool { return m.count() >= 2 })
	if e.Status().Resting {
		t.Fatal("rested without MayRest")
	}
}

// fakeStremioAPI is Stremio's account services with one library entry, updated by the test.
type fakeStremioAPI struct {
	*httptest.Server
	mu    sync.Mutex
	item  map[string]any
	mtime int64
	asked int
}

func newFakeStremio(t *testing.T) *fakeStremioAPI {
	f := &fakeStremioAPI{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.asked++
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		if body["authKey"] != "stremio-key" {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "Session not found", "code": 1}})
			return
		}
		switch r.URL.Path {
		case "/api/datastoreMeta":
			out := [][]any{}
			if f.item != nil {
				out = append(out, []any{f.item["_id"], f.mtime})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": out})
		case "/api/datastoreGet":
			_ = json.NewEncoder(w).Encode(map[string]any{"result": []any{f.item}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakeStremioAPI) play(name, video string, offset int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mtime = time.Now().UnixMilli()
	f.item = map[string]any{"_id": "tt0000002", "name": name, "type": "series", "removed": false, "temp": false,
		"_mtime": time.UnixMilli(f.mtime).UTC().Format(time.RFC3339),
		"state":  map[string]any{"timeOffset": offset, "duration": 1_500_000, "video_id": video}}
}

func (f *fakeStremioAPI) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.asked }

// withStremio signs the engine in to the fake Stremio.
func withStremio(s *fakeStremioAPI) testMod {
	return func(cfg *core.Config, o *Options) {
		o.StremioAPI = s.URL
		cfg.StremioKey = "stremio-key"
	}
}

func TestStremioPlayback(t *testing.T) {
	t.Parallel()
	s := newFakeStremio(t)
	s.play("Sample Show", "tt0000002:1:2", 600_000)
	e, d, l := newTestEngine(t, withPort(deadPort(t)), shortPause, withStremio(s))
	e.Start()
	eventually(t, "Stremio's video shown", func() bool { return len(d.list()) > 0 })
	act := d.list()[len(d.list())-1]["activity"].(map[string]any)
	assets, _ := act["assets"].(map[string]any)
	if act["name"] != "Sample Show" || !strings.Contains(fmt.Sprint(assets["large_text"]), "Season 1, Episode 2") {
		t.Fatalf("unexpected activity %v", act)
	}
	ts, _ := act["timestamps"].(map[string]any)
	start, _ := ts["start"].(float64)
	if d := time.Now().UnixMilli() - int64(start); d < 590_000 || d > 620_000 {
		t.Fatalf("the progress bar starts %d ms ago, not about 600 s", d)
	}
	eventually(t, "status shows Stremio", func() bool { return e.Status().NowPlaying != nil })
	if st := e.Status(); st.Player != "Stremio" {
		t.Fatalf("%+v", st)
	}
	if !l.has("your Stremio account") || !l.has("Stremio detected") {
		t.Fatal("the log does not mention Stremio")
	}
}

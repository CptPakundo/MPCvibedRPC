package engine

import (
	"fmt"
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

// fakeJellyfin is a Jellyfin (or Emby) server whose session list shows the user's episode, shaped like Jellyfin's
// SessionInfoDto (see internal/jellyfin).
type fakeJellyfin struct {
	*httptest.Server
	mu     sync.Mutex
	paused bool
	asked  int
}

const jellyfinUser = "6c8f0a0b1c2d4e5f8a9b0c1d2e3f4a5b"

func newFakeJellyfin(t *testing.T) *fakeJellyfin {
	f := &fakeJellyfin{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.asked++
		if r.URL.Path != "/Sessions" || !strings.Contains(r.Header.Get("Authorization"), `Token="jf-token"`) {
			w.WriteHeader(401)
			return
		}
		fmt.Fprintf(w, `[{"PlayState":{"PositionTicks":6000000000,"IsPaused":%t},"Id":"s1","UserId":%q,"Client":"Jellyfin Web",
			"LastPlaybackCheckIn":%q,"NowPlayingItem":{"Name":"The Sample","Id":"i1","Type":"Episode","MediaType":"Video",
			"SeriesName":"Sample Show","ParentIndexNumber":1,"IndexNumber":2,"RunTimeTicks":14400000000}}]`,
			f.paused, jellyfinUser, time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z"))
	}))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakeJellyfin) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.asked }

func jellyfinEngine(t *testing.T, port int, f *fakeJellyfin, product string) (*Engine, *disc, *logs) {
	e, d, l := pipeEngine(t, port, nil)
	e.opts.JellyfinEvery = 100 * time.Millisecond
	e.cfg.JellyfinAddress, e.cfg.JellyfinToken, e.cfg.JellyfinUser, e.cfg.JellyfinProduct = f.URL, "jf-token", jellyfinUser, product
	return e, d, l
}

func TestJellyfinPlayback(t *testing.T) {
	f := newFakeJellyfin(t)
	e, d, l := jellyfinEngine(t, deadPort(t), f, "Jellyfin")
	e.Start()
	eventually(t, "the episode shown", func() bool { return e.Status().NowPlaying != nil && len(d.list()) > 0 })
	st := e.Status()
	if st.Player != "Jellyfin" || !strings.Contains(*st.NowPlaying, "Sample Show") || st.Paused {
		t.Fatalf("%+v %q", st, *st.NowPlaying)
	}
	act := lastActivity(d)
	if act["type"] != float64(3) || act["name"] != "Sample Show" || act["details"] != "The Sample" {
		t.Fatalf("activity: %v", act)
	}
	ts, _ := act["timestamps"].(map[string]any)
	if start, _ := ts["start"].(float64); start == 0 || time.Since(time.UnixMilli(int64(start))) < 9*time.Minute {
		t.Fatalf("the progress bar starts 10 minutes back: %v", act)
	}
	if !l.has("your Jellyfin server") || !l.has("Jellyfin detected") || !l.has("Following your Jellyfin server") {
		t.Fatal(l.l)
	}
	n := len(d.list())
	f.mu.Lock()
	f.paused = true
	f.mu.Unlock()
	eventually(t, "paused", func() bool { return e.Status().Paused && len(d.list()) > n })
}

// An Emby server shows as Emby.
func TestEmbyIsCalledEmby(t *testing.T) {
	f := newFakeJellyfin(t)
	e, _, l := jellyfinEngine(t, deadPort(t), f, "Emby")
	e.Start()
	eventually(t, "Emby shown", func() bool { return e.Status().Player == "Emby" })
	if !l.has("your Emby server") {
		t.Fatal(l.l)
	}
}

// A player on this computer that plays beats what the server reports.
func TestJellyfinAfterLocalPlayers(t *testing.T) {
	f := newFakeJellyfin(t)
	m := newMPC(t)
	e, _, _ := jellyfinEngine(t, m.port, f, "Jellyfin")
	e.Start()
	eventually(t, "the server asked", func() bool { return f.count() >= 2 })
	eventually(t, "MPC-HC shown", func() bool { return e.Status().Player == "MPC-HC" })
	time.Sleep(600 * time.Millisecond)
	if st := e.Status(); st.Player != "MPC-HC" {
		t.Fatalf("Jellyfin won over a playing MPC-HC: %+v", st)
	}
	m.set(func(m *mpc) { m.state = 0 }) // MPC-HC stops: the server's video shows
	eventually(t, "Jellyfin shown", func() bool { return e.Status().Player == "Jellyfin" })
}

// A refused sign-in is shown in the window.
func TestJellyfinRefusedSignIn(t *testing.T) {
	f := newFakeJellyfin(t)
	e, _, _ := jellyfinEngine(t, deadPort(t), f, "Jellyfin")
	e.cfg.JellyfinToken = "revoked"
	e.Start()
	eventually(t, "the hint", func() bool { return strings.Contains(e.Status().Hint, "Jellyfin refused the sign-in") })
}

// Resting stops the watcher (no more requests) and starts it again when it ends.
func TestJellyfinStopsWhileResting(t *testing.T) {
	f := newFakeJellyfin(t)
	var mayRest atomic.Bool
	cfg := core.DefaultConfig()
	cfg.Port, cfg.PollInterval, cfg.ShowArtwork = deadPort(t), 250, false
	cfg.JellyfinAddress, cfg.JellyfinToken, cfg.JellyfinUser, cfg.JellyfinProduct = f.URL, "jf-token", jellyfinUser, "Jellyfin"
	e := New(cfg, Options{Pipes: noPipes, MPRIS: noMPRIS, DiscordPaths: []string{fakePath(sockDir(t))}, CacheFile: filepath.Join(t.TempDir(), "c.json"),
		JellyfinEvery: 100 * time.Millisecond, MayRest: mayRest.Load})
	t.Cleanup(e.Stop)
	e.Start()
	eventually(t, "the server asked", func() bool { return f.count() >= 1 })
	mayRest.Store(true)
	eventually(t, "resting", func() bool { return e.Status().Resting })
	time.Sleep(150 * time.Millisecond) // a request under way may still land
	n := f.count()
	time.Sleep(500 * time.Millisecond)
	if f.count() != n {
		t.Fatal("the server was asked while resting")
	}
	mayRest.Store(false)
	e.Wake()
	eventually(t, "asked again", func() bool { return f.count() > n })
}

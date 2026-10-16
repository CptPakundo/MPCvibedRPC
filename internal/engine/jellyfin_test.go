package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// withJellyfin signs the engine in to the fake server, which is asked every 100 ms.
func withJellyfin(f *fakeJellyfin, product string) testMod {
	return func(cfg *core.Config, o *Options) {
		o.JellyfinEvery = 100 * time.Millisecond
		cfg.JellyfinAddress, cfg.JellyfinToken, cfg.JellyfinUser, cfg.JellyfinProduct = f.URL, "jf-token", jellyfinUser, product
	}
}

func jellyfinEngine(t *testing.T, port int, f *fakeJellyfin, product string) (*Engine, *disc, *logs) {
	return newTestEngine(t, withPort(port), shortPause, withJellyfin(f, product))
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

// A refused sign-in is shown in the window.
func TestJellyfinRefusedSignIn(t *testing.T) {
	f := newFakeJellyfin(t)
	e, _, _ := jellyfinEngine(t, deadPort(t), f, "Jellyfin")
	e.cfg.JellyfinToken = "revoked"
	e.Start()
	eventually(t, "the hint", func() bool { return strings.Contains(e.Status().Hint, "Jellyfin refused the sign-in") })
}

package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakePlex is plex.tv (the account's one server) and that Plex Media Server, in one test server. The answers are
// shaped like the ones a real server gave on CI (see tools/ci/live-plex.sh).
type fakePlex struct {
	*httptest.Server
	events  chan string
	mu      sync.Mutex
	streams int
	asked   int // requests of any kind
}

func newFakePlex(t *testing.T) *fakePlex {
	f := &fakePlex{events: make(chan string, 16)}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.asked++
		f.mu.Unlock()
		if r.Header.Get("X-Plex-Token") == "" || r.Header.Get("X-Plex-Client-Identifier") != "test-install" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.URL.Path == "/api/v2/resources":
			fmt.Fprintf(w, `[{"name":"Sample Server","provides":"server","owned":false,"clientIdentifier":"srv-1","accessToken":"server-token","connections":[{"uri":%q,"local":true,"relay":false}]}]`, f.URL)
		case r.Header.Get("X-Plex-Token") != "server-token":
			w.WriteHeader(401)
		case r.URL.Path == "/identity":
			fmt.Fprint(w, `{"MediaContainer":{"size":0,"apiVersion":"1.2.3","claimed":true,"machineIdentifier":"srv-1","version":"1.43.4.10903-e5521bd8c"}}`)
		case r.URL.Path == "/library/metadata/4":
			fmt.Fprint(w, `{"MediaContainer":{"size":1,"librarySectionTitle":"TV","Metadata":[{"ratingKey":"4","key":"/library/metadata/4","type":"episode","title":"The Pilot","grandparentTitle":"Sample Show","parentTitle":"Season 1","index":2,"parentIndex":1,"duration":300023}]}}`)
		case r.URL.Path == "/:/eventsource/notifications":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			f.mu.Lock()
			f.streams++
			f.mu.Unlock()
			for {
				select {
				case <-r.Context().Done():
					return
				case e := <-f.events:
					fmt.Fprint(w, e)
					w.(http.Flusher).Flush()
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakePlex) play(offset int, state string) {
	f.events <- fmt.Sprintf("event: playing\ndata: {\"PlaySessionStateNotification\":{\"sessionKey\":\"1\",\"clientIdentifier\":\"sample-player\",\"guid\":\"\",\"ratingKey\":\"4\",\"url\":\"\",\"key\":\"/library/metadata/4\",\"viewOffset\":%d,\"state\":%q}}\n\n", offset, state)
}

func (f *fakePlex) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.asked }

func plexEngine(t *testing.T, port int, p *fakePlex) (*Engine, *disc, *logs) {
	e, d, l := pipeEngine(t, port, nil)
	e.opts.PlexTV = p.URL
	e.cfg.PlexToken, e.cfg.PlexClient, e.cfg.PlexServer, e.cfg.PlexAccount = "account-token", "test-install", "srv-1", 12345
	return e, d, l
}

func TestPlexPlayback(t *testing.T) {
	p := newFakePlex(t)
	e, d, l := plexEngine(t, deadPort(t), p)
	e.Start()
	eventually(t, "Plex followed", func() bool { s := e.Status(); return s.Player == "Plex" && s.MPC })
	if !l.has("your Plex server") || !l.has("Following the Plex server \"Sample Server\".") {
		t.Error("the log should say Plex is followed")
	}
	if len(d.list()) != 0 {
		t.Fatal("Discord must not be contacted while nothing plays")
	}
	p.play(60000, "playing")
	eventually(t, "playing", func() bool { s := e.Status(); return s.NowPlaying != nil && !s.Paused })
	if np := *e.Status().NowPlaying; !strings.Contains(np, "Sample Show") || !strings.Contains(np, "S01E02") || !strings.Contains(np, "The Pilot") {
		t.Errorf("now playing %q", np)
	}
	eventually(t, "activity sent", func() bool { return len(d.list()) > 0 && !isClear(d) })
	act := d.list()[len(d.list())-1]["activity"].(map[string]any)
	if assets, _ := act["assets"].(map[string]any); act["name"] != "Sample Show" || act["details"] != "The Pilot" || assets == nil || assets["large_text"] != "Season 1, Episode 2" {
		t.Errorf("activity %v", act)
	}
	if !l.has("Plex detected.") {
		t.Error("the log should name Plex")
	}
	p.play(70000, "paused")
	eventually(t, "paused", func() bool { return e.Status().Paused })
	p.play(70000, "stopped")
	eventually(t, "cleared", func() bool { return isClear(d) && e.Status().NowPlaying == nil })
	for _, line := range l.l {
		if strings.Contains(line, "account-token") || strings.Contains(line, "server-token") {
			t.Errorf("a token reached the log: %s", line)
		}
	}
}

// Plex is asked last: a player on this computer that plays wins, and Plex comes back when it stops.
func TestPlexInThePlayerOrder(t *testing.T) {
	p := newFakePlex(t)
	m := newMPC(t)
	e, _, _ := plexEngine(t, m.port, p)
	e.Start()
	eventually(t, "MPC-HC", func() bool { s := e.Status(); return s.Player == "MPC-HC" && s.NowPlaying != nil })
	p.play(60000, "playing")
	for i := 0; i < 4; i++ {
		eventually(t, "still MPC-HC", func() bool { return e.Status().Player == "MPC-HC" })
	}
	m.set(func(m *mpc) { m.state = 0; m.file = ""; m.filepath = "" })
	eventually(t, "Plex", func() bool {
		s := e.Status()
		return s.Player == "Plex" && s.NowPlaying != nil && strings.Contains(*s.NowPlaying, "Sample Show")
	})
	// an idle MPC-HC that answers is reported before an idle Plex
	p.play(60000, "stopped")
	eventually(t, "idle MPC-HC", func() bool { s := e.Status(); return s.Player == "MPC-HC" && s.NowPlaying == nil })
}

func TestPlexNotAskedWithoutSignIn(t *testing.T) {
	p := newFakePlex(t)
	e, _, l := plexEngine(t, deadPort(t), p)
	e.cfg.PlexToken = ""
	e.Start()
	eventually(t, "a few ticks", func() bool { return l.has("Presence started.") })
	for i := 0; i < 3; i++ {
		eventually(t, "no player", func() bool { return !e.Status().MPC })
	}
	if p.count() != 0 || l.has("Plex") {
		t.Errorf("Plex was asked without a sign-in (%d requests)", p.count())
	}
}

func TestPlexProblemShownAndStoppedWithPresence(t *testing.T) {
	p := newFakePlex(t)
	e, _, _ := plexEngine(t, deadPort(t), p)
	e.cfg.PlexServer = "gone"
	e.Start()
	eventually(t, "hint", func() bool { return strings.Contains(e.Status().Hint, "no longer in your Plex account") })
	e.cfg.PlexServer = "srv-1"
	e.Stop()
	e.ApplySettings(e.Config()) // unchanged: stays stopped
	if s := e.Status(); s.Running || s.Hint != "" {
		t.Fatalf("after stop: %+v", s)
	}
	e.Start()
	eventually(t, "followed", func() bool { return e.Status().Player == "Plex" })
	n := func() int { p.mu.Lock(); defer p.mu.Unlock(); return p.streams }
	if n() != 1 {
		t.Errorf("streams %d", n())
	}
	e.Stop()
	eventually(t, "stream closed", func() bool { return e.Status().Player == "" })
}

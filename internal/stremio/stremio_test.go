package stremio

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeStremio stands in for api.strem.io and link.stremio.com, answering the way stremio-core expects them to.
type fakeStremio struct {
	mu       sync.Mutex
	key      string // the only key accepted
	approved bool   // the link code was approved
	linkURL  string
	items    map[string]map[string]any
	mtimes   map[string]int64
	asked    []string // the paths asked, in order
	bodies   []map[string]any
	loggedIn bool
	failGet  bool // datastoreGet answers with an HTTP error
}

func newFake(t *testing.T) (*fakeStremio, *Client) {
	f := &fakeStremio{key: "KEY", linkURL: "https://www.stremio.com/link?code=ABCD", items: map[string]map[string]any{}, mtimes: map[string]int64{}, loggedIn: true}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, &Client{APIBase: srv.URL, LinkBase: srv.URL}
}

func (f *fakeStremio) put(id, name, typ, video string, offset, dur, mtime int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[id] = map[string]any{"_id": id, "name": name, "type": typ, "removed": false, "temp": false,
		"_mtime": time.UnixMilli(mtime).UTC().Format(time.RFC3339), "poster": "",
		"state": map[string]any{"timeOffset": offset, "duration": dur, "video_id": video, "lastWatched": nil}}
	f.mtimes[id] = mtime
}

func (f *fakeStremio) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, r.Method+" "+r.URL.Path)
	answer := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"result": v}) }
	refuse := func(msg string) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": msg, "code": 1}})
	}
	var body map[string]any
	if r.Method == "POST" {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		f.bodies = append(f.bodies, body)
		if body["authKey"] != f.key || !f.loggedIn {
			refuse("Session not found")
			return
		}
	}
	switch r.URL.Path {
	case "/api/v2/create":
		if r.URL.Query().Get("type") != "Create" {
			http.Error(w, "bad", 400)
			return
		}
		answer(map[string]string{"code": "ABCD", "link": f.linkURL, "qrcode": "data:"})
	case "/api/v2/read":
		if r.URL.Query().Get("type") != "Read" || r.URL.Query().Get("code") != "ABCD" {
			http.Error(w, "bad", 400)
			return
		}
		if !f.approved {
			refuse("Not yet linked")
			return
		}
		answer(map[string]string{"authKey": f.key})
	case "/api/datastoreMeta":
		if body["collection"] != "libraryItem" {
			refuse("bad collection")
			return
		}
		out := [][]any{}
		for id, t := range f.mtimes {
			out = append(out, []any{id, t})
		}
		answer(out)
	case "/api/datastoreGet":
		if f.failGet {
			http.Error(w, "down", 500)
			return
		}
		out := []any{}
		ids, _ := body["ids"].([]any)
		for _, id := range ids {
			if it, ok := f.items[id.(string)]; ok {
				out = append(out, it)
			}
		}
		answer(out)
	case "/api/logout":
		f.loggedIn = false
		answer(map[string]bool{"success": true})
	default:
		http.NotFound(w, r)
	}
}

func TestSignIn(t *testing.T) {
	f, c := newFake(t)
	ctx := context.Background()
	l, err := c.NewLink(ctx)
	if err != nil || l.Code != "ABCD" || l.URL != f.linkURL {
		t.Fatalf("link = %+v, %v", l, err)
	}
	if key, err := c.CheckLink(ctx, l); key != "" || err != nil {
		t.Fatalf("before approval: %q, %v", key, err)
	}
	f.approved = true
	if key, err := c.CheckLink(ctx, l); key != "KEY" || err != nil {
		t.Fatalf("after approval: %q, %v", key, err)
	}
	if err := c.SignOut(ctx, "KEY"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.mtimes(ctx, "KEY"); !errors.Is(err, ErrKey) {
		t.Fatalf("a signed-out key still works: %v", err)
	}
}

func TestLinkPageMustBeStremio(t *testing.T) {
	f, c := newFake(t)
	for _, u := range []string{"http://www.stremio.com/link", "https://stremio.com.example.org/x", "https://evil.example/stremio.com", "https://user@www.stremio.com/"} {
		f.linkURL = u
		if _, err := c.NewLink(context.Background()); !errors.Is(err, ErrBadLink) {
			t.Errorf("%s: %v", u, err)
		}
	}
	f.linkURL = "https://stremio.com/link?code=ABCD"
	if _, err := c.NewLink(context.Background()); err != nil {
		t.Error(err)
	}
}

func TestFileName(t *testing.T) {
	for _, c := range []struct{ id, name, typ, video, want string }{
		{"tt0000001", "Sample Movie", "movie", "tt0000001", "Sample Movie"},
		{"tt0000002", "Sample Show", "series", "tt0000002:1:2", "Sample Show - S01E02"},
		{"tt0000002", "Sample Show", "series", "tt0000002:0:11", "Sample Show - S00E11"},
		{"kitsu:12", "Sample Anime", "series", "kitsu:12:5", "Sample Anime - E05"},
		{"tt0000002", "Sample Show", "series", "", "Sample Show"},
		{"tt0000002", "Sample Show", "series", "other:1:2", "Sample Show"},
		{"tt0000003", "Sample/Show", "series", "tt0000003:x:2", "Sample-Show"},
	} {
		it := Item{ID: c.id, Name: c.name, Type: c.typ}
		it.State.VideoID = c.video
		if got := it.FileName(); got != c.want {
			t.Errorf("%+v: %q, want %q", c, got, c.want)
		}
	}
}

// clockAt is a settable clock for the watcher.
type clockAt struct{ t time.Time }

func (c *clockAt) now() time.Time { return c.t }

func watcher(t *testing.T) (*fakeStremio, *Watcher, *clockAt) {
	f, c := newFake(t)
	clk := &clockAt{t: time.Now()}
	return f, &Watcher{Client: c, Key: "KEY", Every: 20 * time.Second, now: clk.now}, clk
}

func (w *Watcher) mustPoll(t *testing.T) {
	t.Helper()
	if err := w.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func state(w *Watcher) (string, int, int) {
	in := w.Now()
	if in == nil {
		return "<nil>", 0, 0
	}
	return in.File, in.State, in.Position
}

func TestWatcher(t *testing.T) {
	f, w, clk := watcher(t)
	ms := func() int64 { return clk.t.UnixMilli() }
	if in := w.Now(); in != nil {
		t.Fatalf("before the library answered: %+v", in)
	}
	// something watched yesterday, and a show resumed half a minute ago
	f.put("tt0000001", "Sample Movie", "movie", "tt0000001", 3_000_000, 6_000_000, ms()-86_400_000)
	f.put("tt0000002", "Sample Show", "series", "tt0000002:1:2", 600_000, 1_500_000, ms()-30_000)
	w.mustPoll(t)
	if file, st, pos := state(w); file != "Sample Show - S01E02" || st != 2 || pos < 629_000 || pos > 631_000 {
		t.Fatalf("at the start: %s %d %d", file, st, pos)
	}
	// a regular update 90 s after the last one
	clk.t = clk.t.Add(60 * time.Second)
	f.put("tt0000002", "Sample Show", "series", "tt0000002:1:2", 690_000, 1_500_000, ms())
	w.mustPoll(t)
	clk.t = clk.t.Add(10 * time.Second)
	if file, st, pos := state(w); file != "Sample Show - S01E02" || st != 2 || pos < 699_000 || pos > 701_000 {
		t.Fatalf("while it plays: %s %d %d", file, st, pos)
	}
	// paused 30 s later: Stremio updates the entry early, with the time moved on
	clk.t = clk.t.Add(20 * time.Second)
	f.put("tt0000002", "Sample Show", "series", "tt0000002:1:2", 720_000, 1_500_000, ms())
	w.mustPoll(t)
	if _, st, _ := state(w); st != -1 {
		t.Fatalf("after the pause it still shows (state %d)", st)
	}
	// resumed two minutes later: the entry is where it was
	clk.t = clk.t.Add(2 * time.Minute)
	f.put("tt0000002", "Sample Show", "series", "tt0000002:1:2", 720_000, 1_500_000, ms())
	w.mustPoll(t)
	if _, st, pos := state(w); st != 2 || pos != 720_000 {
		t.Fatalf("after the resume: state %d at %d", st, pos)
	}
	// the next episode starts (the video id changes)
	clk.t = clk.t.Add(40 * time.Second)
	f.put("tt0000002", "Sample Show", "series", "tt0000002:1:3", 0, 1_500_000, ms())
	w.mustPoll(t)
	if file, st, _ := state(w); file != "Sample Show - S01E03" || st != 2 {
		t.Fatalf("next episode: %s %d", file, st)
	}
	// then Stremio is closed without a word: the regular updates stop and the status goes after a while (the
	// player's interval is not known yet, so a 180-second one is waited for)
	clk.t = clk.t.Add(3 * time.Minute)
	w.mustPoll(t)
	if _, st, _ := state(w); st != 2 {
		t.Fatalf("given up too early (state %d)", st)
	}
	clk.t = clk.t.Add(60 * time.Second)
	if _, st, _ := state(w); st != -1 {
		t.Fatalf("not given up after the updates stopped (state %d)", st)
	}
}

// player plays an entry like stremio-core: a regular update once every seconds have passed since the last regular
// one, extra updates on pause and resume (they do not move that timer), the library asked every 20 s.
type player struct {
	t        *testing.T
	f        *fakeStremio
	w        *Watcher
	clk      *clockAt
	every    time.Duration
	offset   int64     // where the video is (ms)
	playing  bool      //
	lastPush time.Time // the last regular update
	at       time.Time // the clock when offset was last moved
}

func newPlayer(t *testing.T, every time.Duration) *player {
	f, w, clk := watcher(t)
	p := &player{t: t, f: f, w: w, clk: clk, every: every, offset: 60_000, playing: true, lastPush: clk.t, at: clk.t}
	p.put() // the start
	w.mustPoll(t)
	return p
}

func (p *player) put() {
	p.f.put("tt0000002", "Sample Show", "series", "tt0000002:1:2", p.offset, 3_000_000, p.clk.t.UnixMilli())
}

// run lets d pass second by second, polling every 20 s.
func (p *player) run(d time.Duration) {
	for end := p.clk.t.Add(d); p.clk.t.Before(end); {
		p.clk.t = p.clk.t.Add(time.Second)
		if p.playing {
			p.offset += 1000
			if p.clk.t.Sub(p.lastPush) >= p.every {
				p.lastPush = p.clk.t
				p.put()
			}
		}
		if p.clk.t.Sub(p.at)%(20*time.Second) == 0 {
			p.w.mustPoll(p.t)
		}
	}
}

func (p *player) pause(paused bool) { p.playing = !paused; p.put() }

func (p *player) want(st int, what string) {
	p.t.Helper()
	if _, got, _ := state(p.w); got != st {
		p.t.Fatalf("%s: state %d, want %d", what, got, st)
	}
}

// A player that updates every 180 s (stremio-core 0.65.1 and later) stays shown between its updates, and a short
// pause is seen once the interval is known, without the next regular update being taken for another pause.
func TestWatcher180SecondPlayer(t *testing.T) {
	p := newPlayer(t, PushLong)
	p.want(2, "the start")
	p.run(175 * time.Second)
	p.want(2, "175 s after the start, before the first regular update") // the old 90-second rule gave up at 145 s
	p.run(5*time.Second + 180*time.Second)
	p.want(2, "after two regular updates")
	if p.w.push != PushLong {
		t.Fatalf("interval = %v, want 180 s", p.w.push)
	}
	p.run(170 * time.Second)
	p.want(2, "170 s after a regular update")
	p.run(10*time.Second + 100*time.Second)
	p.pause(true) // 100 s into the interval: a 90-second rule took this for a regular update
	p.run(20 * time.Second)
	p.want(-1, "paused")
	p.pause(false)
	p.run(20 * time.Second)
	p.want(2, "resumed")
	p.run(60 * time.Second) // the regular update comes 180 s after the last regular one, 60 s after the resume
	p.want(2, "the regular update after a resume")
	// stopped without a word: given up after the interval and a little more
	p.playing = false
	p.run(200 * time.Second) // 220 s after the last update
	p.want(2, "too early")
	p.run(40 * time.Second)
	p.want(-1, "the updates stopped")
}

// A player that updates every 90 s (before stremio-core 0.65.1) works as before, and the first regular update after a
// short pause (the timer runs on through pauses) is not taken for another pause.
func TestWatcher90SecondPlayer(t *testing.T) {
	p := newPlayer(t, PushShort)
	p.run(10 * time.Minute)
	p.want(2, "ten minutes of regular updates")
	if p.w.push == PushLong {
		t.Fatal("took a 90-second player for a 180-second one")
	}
	p.run(40 * time.Second) // 10 s after a regular update
	p.pause(true)
	p.run(20 * time.Second)
	p.want(-1, "paused")
	p.pause(false)
	p.run(20 * time.Second)
	p.want(2, "resumed")
	p.run(40 * time.Second)
	p.want(2, "the regular update after a resume")
}

// A 180-second interval learned on one device is unlearned when a 90-second player plays: its first regular update is
// taken for a pause, and the next one shows it kept playing.
func TestWatcherRelearnsTheInterval(t *testing.T) {
	p := newPlayer(t, PushLong)
	p.run(370 * time.Second)
	if p.w.push != PushLong {
		t.Fatalf("interval = %v, want 180 s", p.w.push)
	}
	p.every = PushShort
	p.run(200 * time.Second)
	p.want(2, "a 90-second player")
	if p.w.push != PushShort {
		t.Fatalf("interval = %v, want 90 s", p.w.push)
	}
}

func TestWatcherStartAndEnd(t *testing.T) {
	f, w, clk := watcher(t)
	ms := func() int64 { return clk.t.UnixMilli() }
	f.put("tt0000001", "Sample Movie", "movie", "tt0000001", 1_000_000, 6_000_000, ms()-10*60_000) // paused ten minutes ago
	w.mustPoll(t)
	if _, st, _ := state(w); st != -1 {
		t.Fatalf("an old update shows as playing (state %d)", st)
	}
	clk.t = clk.t.Add(20 * time.Second)
	f.put("tt0000001", "Sample Movie", "movie", "tt0000001", 1_000_000, 6_000_000, ms())
	w.mustPoll(t)
	if file, st, _ := state(w); file != "Sample Movie" || st != 2 {
		t.Fatalf("resumed: %s %d", file, st)
	}
	// watched to the end: Stremio rewinds the entry
	clk.t = clk.t.Add(90 * time.Second)
	f.put("tt0000001", "Sample Movie", "movie", "tt0000001", 0, 6_000_000, ms())
	w.mustPoll(t)
	if _, st, _ := state(w); st != -1 {
		t.Fatalf("still shows after the end (state %d)", st)
	}
	// an entry of type other never shows
	clk.t = clk.t.Add(20 * time.Second)
	f.put("local:1", "Sample Clip", "other", "", 5_000, 60_000, ms())
	w.mustPoll(t)
	if _, st, _ := state(w); st != -1 {
		t.Fatalf("an 'other' entry shows (state %d)", st)
	}
}

// A failed fetch of the changed entries must not lose the change: the next poll finds it again.
func TestWatcherFailedFetchIsRetried(t *testing.T) {
	f, w, clk := watcher(t)
	ms := func() int64 { return clk.t.UnixMilli() }
	f.put("tt0000002", "Sample Show", "series", "tt0000002:1:2", 600_000, 1_500_000, ms()-30_000)
	f.mu.Lock()
	f.failGet = true
	f.mu.Unlock()
	if err := w.poll(context.Background()); err == nil {
		t.Fatal("the failed fetch was not reported")
	}
	if w.Now() != nil {
		t.Fatal("a failed fetch reports a state")
	}
	f.mu.Lock()
	f.failGet = false
	f.mu.Unlock()
	w.mustPoll(t)
	if file, st, _ := state(w); file != "Sample Show - S01E02" || st != 2 {
		t.Fatalf("the change was lost: %s %d", file, st)
	}
}

func TestWatcherRefusedKey(t *testing.T) {
	f, w, _ := watcher(t)
	f.key = "OTHER"
	if err := w.poll(context.Background()); !errors.Is(err, ErrKey) {
		t.Fatalf("poll = %v", err)
	}
	if w.Now() != nil {
		t.Fatal("a refused key reports a state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(w.Problem(), "Sign in to Stremio again") {
		if time.Now().After(deadline) {
			t.Fatalf("problem = %q", w.Problem())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
}

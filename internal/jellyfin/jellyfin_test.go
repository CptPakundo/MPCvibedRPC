package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const me = "6c8f0a0b1c2d4e5f8a9b0c1d2e3f4a5b" // Jellyfin writes ids without dashes

// fakeServer answers like a Jellyfin server (Emby: only under /emby, no ProductName, no Quick Connect). The answers
// follow Jellyfin's API (Jellyfin.Api controllers, SessionInfoDto, QuickConnectResult).
type fakeServer struct {
	*httptest.Server
	emby      bool
	qcOff     bool
	mu        sync.Mutex
	checks    int
	auth      string // the Authorization header of the last request
	sessions  string // the /Sessions answer
	date      time.Time
	loggedOut bool
}

func newFake(t *testing.T, emby bool) *fakeServer {
	f := &fakeServer{emby: emby, sessions: "[]"}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = r.Header.Get("Authorization")
		if emby {
			f.auth = r.Header.Get("X-Emby-Authorization")
		}
		if !f.date.IsZero() {
			w.Header().Set("Date", f.date.UTC().Format(http.TimeFormat))
		}
		path := r.URL.Path
		if emby {
			if !strings.HasPrefix(path, "/emby/") {
				http.NotFound(w, r)
				return
			}
			path = strings.TrimPrefix(path, "/emby")
		}
		token := ""
		if i := strings.Index(f.auth, `Token="`); i >= 0 {
			token = f.auth[i+7:]
			token = token[:strings.IndexByte(token, '"')]
		}
		switch {
		case path == "/System/Info/Public":
			if emby {
				fmt.Fprint(w, `{"LocalAddress":"http://127.0.0.1:8096","ServerName":"Sample Server","Version":"4.9.0.0","Id":"a1b2c3"}`)
			} else {
				fmt.Fprint(w, `{"LocalAddress":"http://127.0.0.1:8096","ServerName":"Sample Server","Version":"12.2.0","ProductName":"Jellyfin Server","OperatingSystem":"","Id":"a1b2c3","StartupWizardCompleted":true}`)
			}
		case path == "/Users/AuthenticateByName" && r.Method == "POST":
			var body struct{ Username, Pw string }
			if err := jsonBody(r, &body); err != nil || body.Username != "sample" || body.Pw != "secret" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprintf(w, `{"User":{"Name":"sample","Id":"%s"},"AccessToken":"tok-pw","ServerId":"a1b2c3"}`, me)
		case path == "/QuickConnect/Initiate" && r.Method == "POST":
			if emby {
				http.NotFound(w, r)
			} else if f.qcOff {
				w.WriteHeader(401)
			} else {
				fmt.Fprint(w, `{"Authenticated":false,"Secret":"qc-secret","Code":"123456","DeviceId":"d","DeviceName":"n","AppName":"MPCvibedRPC","AppVersion":"0.9.21","DateAdded":"2026-10-11T10:00:00Z"}`)
			}
		case path == "/QuickConnect/Connect" && r.URL.Query().Get("secret") == "qc-secret":
			f.checks++
			fmt.Fprintf(w, `{"Authenticated":%t,"Secret":"qc-secret","Code":"123456"}`, f.checks >= 2)
		case path == "/QuickConnect/Connect":
			w.WriteHeader(404)
		case path == "/Users/AuthenticateWithQuickConnect" && r.Method == "POST":
			var body struct{ Secret string }
			if err := jsonBody(r, &body); err != nil || body.Secret != "qc-secret" || f.checks < 2 {
				w.WriteHeader(401)
				return
			}
			fmt.Fprintf(w, `{"User":{"Name":"sample","Id":"%s"},"AccessToken":"tok-qc","ServerId":"a1b2c3"}`, me)
		case path == "/Sessions/Logout" && r.Method == "POST" && token != "":
			f.loggedOut = true
			w.WriteHeader(204)
		case path == "/Sessions" && (token == "tok-pw" || token == "tok-qc"):
			fmt.Fprint(w, f.sessions)
		default:
			w.WriteHeader(401)
		}
	}))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakeServer) set(fn func(f *fakeServer)) { f.mu.Lock(); fn(f); f.mu.Unlock() }

func client(f *fakeServer) *Client {
	return &Client{Base: f.URL, DeviceID: "sample-device", Version: "0.9.21"}
}

func TestAddress(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.20:8096":              "http://192.168.1.20:8096",
		" http://media.example:8096/ ":   "http://media.example:8096",
		"https://media.example/jellyfin": "https://media.example/jellyfin",
	} {
		if got, err := Address(in); err != nil || got != want {
			t.Errorf("Address(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://media.example", "http://user:pw@media.example", "http://media.example/?a=1", "http://"} {
		if got, err := Address(bad); err == nil {
			t.Errorf("Address(%q) = %q, want an error", bad, got)
		}
	}
}

func TestFindJellyfinAndEmby(t *testing.T) {
	f := newFake(t, false)
	c := client(f)
	s, err := c.Find(context.Background())
	if err != nil || s.Product != "Jellyfin" || s.Name != "Sample Server" || c.Base != f.URL {
		t.Fatalf("Jellyfin: %+v %v base %s", s, err, c.Base)
	}
	if !strings.HasPrefix(f.auth, "MediaBrowser ") || !strings.Contains(f.auth, `DeviceId="sample-device"`) || !strings.Contains(f.auth, `Client="MPCvibedRPC"`) {
		t.Fatalf("the client describes itself: %q", f.auth)
	}
	e := newFake(t, true)
	c = client(e)
	s, err = c.Find(context.Background())
	if err != nil || s.Product != "Emby" || c.Base != e.URL+"/emby" {
		t.Fatalf("Emby: %+v %v base %s", s, err, c.Base)
	}
	if !strings.HasPrefix(e.auth, "Emby ") {
		t.Fatalf("Emby reads its own header: %q", e.auth)
	}
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	if _, err := (&Client{Base: other.URL}).Find(context.Background()); !errors.Is(err, ErrNotServer) {
		t.Fatalf("another program: %v", err)
	}
}

func TestPasswordSignIn(t *testing.T) {
	for _, emby := range []bool{false, true} {
		f := newFake(t, emby)
		c := client(f)
		if _, err := c.Find(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := c.SignIn(context.Background(), "sample", "wrong"); !errors.Is(err, ErrLogin) {
			t.Fatalf("a wrong password: %v", err)
		}
		a, err := c.SignIn(context.Background(), "sample", "secret")
		if err != nil || a.Token != "tok-pw" || a.UserID != me || a.UserName != "sample" {
			t.Fatalf("emby=%v: %+v %v", emby, a, err)
		}
		if err := c.SignOut(context.Background(), a.Token); err != nil || !f.loggedOut {
			t.Fatalf("sign-out: %v", err)
		}
	}
}

func TestQuickConnect(t *testing.T) {
	f := newFake(t, false)
	c := client(f)
	p, err := c.StartQuickConnect(context.Background())
	if err != nil || p.Code != "123456" {
		t.Fatalf("%+v %v", p, err)
	}
	a, err := c.CheckQuickConnect(context.Background(), p)
	if err != nil || a != nil {
		t.Fatalf("not approved yet: %+v %v", a, err)
	}
	a, err = c.CheckQuickConnect(context.Background(), p)
	if err != nil || a == nil || a.Token != "tok-qc" || a.UserID != me {
		t.Fatalf("approved: %+v %v", a, err)
	}
	if _, err := c.CheckQuickConnect(context.Background(), &Pending{Code: "1", Secret: "gone"}); !errors.Is(err, ErrExpired) {
		t.Fatalf("an unknown code: %v", err)
	}
	f.set(func(f *fakeServer) { f.qcOff = true })
	if _, err := c.StartQuickConnect(context.Background()); !errors.Is(err, ErrNoQuickConnect) {
		t.Fatalf("switched off: %v", err)
	}
	e := newFake(t, true)
	c = client(e)
	c.Find(context.Background())
	if _, err := c.StartQuickConnect(context.Background()); !errors.Is(err, ErrNoQuickConnect) {
		t.Fatalf("Emby has none: %v", err)
	}
}

func ip(n int) *int { return &n }

func jsonBody(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }

func TestFileName(t *testing.T) {
	for _, c := range []struct {
		it   Item
		want string
	}{
		{Item{Type: "Episode", SeriesName: "Sample Show", ParentIndexNumber: ip(1), IndexNumber: ip(2), Name: "The Sample"}, "Sample Show - S01E02 - The Sample"},
		{Item{Type: "Episode", SeriesName: "Sample Show", ParentIndexNumber: ip(1), IndexNumber: ip(2), Name: "Episode 2"}, "Sample Show - S01E02"},
		{Item{Type: "Episode", SeriesName: "Sample Show", ParentIndexNumber: ip(1), IndexNumber: ip(2), Name: "Sample Show - S01E02"}, "Sample Show - S01E02"},
		{Item{Type: "Episode", SeriesName: "Sample Show", IndexNumber: ip(7), Name: "Sample Day"}, "Sample Show - E07 - Sample Day"},
		{Item{Type: "Episode", SeriesName: "Sample/Show", ParentIndexNumber: ip(2), IndexNumber: ip(10), Name: "A/B"}, "Sample-Show - S02E10 - A-B"},
		{Item{Type: "Movie", Name: "Sample Movie", ProductionYear: 2020}, "Sample Movie (2020)"},
		{Item{Type: "Movie", Name: "Sample Movie (2020)", ProductionYear: 2020}, "Sample Movie (2020)"},
		{Item{Type: "Video", Name: "Sample Clip"}, "Sample Clip"},
	} {
		if got := c.it.FileName(); got != c.want {
			t.Errorf("%+v: got %q want %q", c.it, got, c.want)
		}
	}
	if (&Item{Type: "Audio", MediaType: "Audio"}).IsVideo() || !(&Item{Type: "Episode"}).IsVideo() || !(&Item{Type: "Something", MediaType: "Video"}).IsVideo() {
		t.Fatal("IsVideo")
	}
}

// sessionsJSON is a /Sessions answer: someone else playing a movie, the user's music, the user's paused movie on a TV
// and the user's episode playing in a browser.
func sessionsJSON(checkIn time.Time, paused bool) string {
	ci := checkIn.UTC().Format("2006-01-02T15:04:05.0000000Z")
	return fmt.Sprintf(`[
 {"PlayState":{"PositionTicks":100000000,"IsPaused":false},"Id":"s1","UserId":"99990a0b1c2d4e5f8a9b0c1d2e3f4a5b","UserName":"other","Client":"Jellyfin Web","LastPlaybackCheckIn":"%[1]s",
  "NowPlayingItem":{"Name":"Other Movie","Id":"i1","Type":"Movie","MediaType":"Video","ProductionYear":2019,"RunTimeTicks":60000000000}},
 {"PlayState":{"PositionTicks":50000000,"IsPaused":false},"Id":"s2","UserId":"%[2]s","Client":"Jellyfin Web","LastPlaybackCheckIn":"%[1]s",
  "NowPlayingItem":{"Name":"Sample Song","Id":"i2","Type":"Audio","MediaType":"Audio","RunTimeTicks":2000000000}},
 {"PlayState":{"PositionTicks":200000000,"IsPaused":true},"Id":"s3","UserId":"%[2]s","Client":"Jellyfin Android TV","LastPlaybackCheckIn":"%[1]s",
  "NowPlayingItem":{"Name":"Sample Movie","Id":"i3","Type":"Movie","MediaType":"Video","ProductionYear":2020,"RunTimeTicks":72000000000}},
 {"PlayState":{"PositionTicks":6000000000,"IsPaused":%[3]t},"Id":"s4","UserId":"%[2]s","Client":"Jellyfin Web","LastPlaybackCheckIn":"%[1]s",
  "NowPlayingItem":{"Name":"The Sample","Id":"i4","Type":"Episode","MediaType":"Video","SeriesName":"Sample Show","ParentIndexNumber":1,"IndexNumber":2,"RunTimeTicks":14400000000}},
 {"Id":"s5","UserId":"%[2]s","Client":"MPCvibedRPC","LastPlaybackCheckIn":"0001-01-01T00:00:00.0000000Z","PlayState":{}}
]`, ci, "6c8f0a0b-1c2d-4e5f-8a9b-0c1d2e3f4a5b", paused)
}

func TestWatcherFollowsTheUsersPlayback(t *testing.T) {
	f := newFake(t, false)
	now := time.Date(2026, 10, 11, 10, 0, 10, 0, time.UTC)
	// the player reported 4 seconds before the server answered (by the server's clock, which is 1 hour off ours)
	f.set(func(f *fakeServer) {
		f.sessions = sessionsJSON(now.Add(time.Hour-4*time.Second), false)
		f.date = now.Add(time.Hour)
	})
	w := &Watcher{Client: client(f), Token: "tok-pw", UserID: me}
	w.now = func() time.Time { return now }
	if w.Now() != nil {
		t.Fatal("nothing known before the first answer")
	}
	if err := w.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	in := w.Now()
	if in == nil || in.File != "Sample Show - S01E02 - The Sample" || in.State != 2 || in.Duration != 1440000 {
		t.Fatalf("the playing episode wins over the paused movie: %+v", in)
	}
	if in.Position != 604000 {
		t.Fatalf("position moved on by the report's age: %d", in.Position)
	}
	now = now.Add(3 * time.Second)
	if in := w.Now(); in.Position != 607000 {
		t.Fatalf("and on with the clock: %d", in.Position)
	}

	// paused: the paused movie and the paused episode; the most recent report wins, and the time stands still
	f.set(func(f *fakeServer) { f.sessions = sessionsJSON(now.Add(time.Hour), true) })
	if err := w.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := w.Now()
	now = now.Add(10 * time.Second)
	if b := w.Now(); a.State != 1 || b.Position != a.Position {
		t.Fatalf("paused: %+v then %+v", a, b)
	}

	// nothing of the user's plays
	f.set(func(f *fakeServer) { f.sessions = `[{"Id":"s9","UserId":"` + me + `","PlayState":{}}]` })
	if err := w.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if in := w.Now(); in == nil || in.State != -1 {
		t.Fatalf("nothing playing: %+v", in)
	}
}

func TestWatcherProblems(t *testing.T) {
	f := newFake(t, false)
	var mu sync.Mutex
	var logs []string
	w := &Watcher{Client: client(f), Token: "revoked", UserID: me, Every: 20 * time.Millisecond, retry: 20 * time.Millisecond,
		Log: func(l, m string) { mu.Lock(); logs = append(logs, l+" "+m); mu.Unlock() }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(w.Problem(), "refused the sign-in") {
		if time.Now().After(deadline) {
			t.Fatalf("problem: %q", w.Problem())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if w.Now() != nil {
		t.Fatal("a server that refuses reports nothing")
	}
	cancel()

	w2 := &Watcher{Client: client(f), Token: "tok-pw", UserID: me, Every: 20 * time.Millisecond}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go w2.Run(ctx2)
	for w2.Now() == nil {
		if time.Now().After(deadline) {
			t.Fatal("following")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if w2.Problem() != "" {
		t.Fatal(w2.Problem())
	}
	f.Server.Close()
	for w2.Problem() == "" {
		if time.Now().After(deadline.Add(3 * time.Second)) {
			t.Fatal("an unreachable server is reported")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(w2.Problem(), "Could not reach your Jellyfin server") || w2.Now() != nil {
		t.Fatalf("%q", w2.Problem())
	}
}

package plex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func ip(n int) *int { return &n }

func TestItemName(t *testing.T) {
	cases := []struct {
		it   Item
		want string
	}{
		{Item{Type: "episode", GrandparentTitle: "Sample Show", Title: "The Pilot", ParentIndex: ip(1), Index: ip(2)}, "Sample Show - S01E02 - The Pilot"},
		{Item{Type: "episode", GrandparentTitle: "Sample Show", Title: "Episode 2", ParentIndex: ip(1), Index: ip(2)}, "Sample Show - S01E02"},
		{Item{Type: "episode", GrandparentTitle: "Sample Show", Title: "Special", ParentIndex: ip(0), Index: ip(1)}, "Sample Show - S00E01 - Special"},
		{Item{Type: "episode", GrandparentTitle: "Sample Show", Title: "Long Run", ParentIndex: ip(1), Index: ip(1050)}, "Sample Show - S01E1050 - Long Run"},
		{Item{Type: "episode", GrandparentTitle: "Sample Show", Title: "Date Based"}, "Sample Show - Date Based"},
		{Item{Type: "episode", Title: "Lonely Episode"}, "Lonely Episode"},
		{Item{Type: "movie", Title: "Sample Movie", Year: 2020}, "Sample Movie (2020)"},
		{Item{Type: "movie", Title: "Sample Movie"}, "Sample Movie"},
		{Item{Type: "movie", Title: "Sample/Movie\\Two", Year: 2021}, "Sample-Movie-Two (2021)"},
		{Item{Type: "clip", Title: "Sample Clip"}, "Sample Clip"},
	}
	for _, c := range cases {
		if got := c.it.Name(); got != c.want {
			t.Errorf("%+v: %q want %q", c.it, got, c.want)
		}
	}
	for typ, want := range map[string]bool{"movie": true, "episode": true, "clip": true, "track": false, "photo": false, "": false} {
		if (&Item{Type: typ}).IsVideo() != want {
			t.Errorf("IsVideo(%q) want %v", typ, want)
		}
	}
}

// fakeTV is plex.tv: a PIN that is approved on the third check, the account, and its servers.
func fakeTV(t *testing.T, resources string) (*httptest.Server, *int) {
	checks := 0
	var mu sync.Mutex
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("X-Plex-Client-Identifier") != "test-client" || r.Header.Get("X-Plex-Product") != Product {
			http.Error(w, "headers", 400)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v2/pins" && r.URL.Query().Get("strong") == "true":
			fmt.Fprint(w, `{"id":4242,"code":"samplecode","product":"MPCvibedRPC","trusted":false,"clientIdentifier":"test-client","expiresIn":1800,"authToken":null,"newRegistration":null}`)
		case r.Method == "GET" && r.URL.Path == "/api/v2/pins/4242":
			checks++
			if checks < 3 {
				fmt.Fprint(w, `{"id":4242,"code":"samplecode","expiresIn":1700,"authToken":null}`)
			} else {
				fmt.Fprint(w, `{"id":4242,"code":"samplecode","expiresIn":1700,"authToken":"account-token"}`)
			}
		case r.URL.Path == "/api/v2/pins/1":
			w.WriteHeader(404)
			fmt.Fprint(w, `{"errors":[{"code":1020,"message":"Code not found or expired"}]}`)
		case r.URL.Path == "/api/v2/user":
			if r.Header.Get("X-Plex-Token") != "account-token" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"id":12345,"uuid":"abc","username":"SampleUser","title":"Sample User","email":"user@example.com"}`)
		case r.URL.Path == "/api/v2/resources":
			if r.Header.Get("X-Plex-Token") != "account-token" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, resources)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s, &checks
}

func TestSignIn(t *testing.T) {
	tv, checks := fakeTV(t, `[
		{"name":"Friend's server","provides":"server","owned":false,"clientIdentifier":"srv-b","accessToken":"shared-token","connections":[{"uri":"https://relay.example:443","local":false,"relay":true},{"uri":"https://remote.example:32400","local":false,"relay":false}]},
		{"name":"A phone","provides":"client,player","clientIdentifier":"phone"},
		{"name":"My server","provides":"server","owned":true,"clientIdentifier":"srv-a","accessToken":"own-token","connections":[{"uri":"https://remote.example:1234/","local":false,"relay":false},{"uri":"http://192.168.1.5:32400","local":true,"relay":false}]}
	]`)
	c := &Client{ClientID: "test-client", Version: "0.0.1", TVBase: tv.URL, ResourcesBase: tv.URL}
	ctx := context.Background()
	pin, err := c.NewPin(ctx)
	if err != nil || pin.ID != 4242 || pin.Code != "samplecode" {
		t.Fatalf("pin %+v %v", pin, err)
	}
	if u := c.AuthURL(pin); !strings.HasPrefix(u, "https://app.plex.tv/auth#?") || !strings.Contains(u, "code=samplecode") || !strings.Contains(u, "clientID=test-client") {
		t.Errorf("auth url %s", u)
	}
	for i := 0; i < 2; i++ {
		if tok, err := c.CheckPin(ctx, pin); tok != "" || err != nil {
			t.Fatalf("check %d: %q %v", i, tok, err)
		}
	}
	if tok, err := c.CheckPin(ctx, pin); tok != "account-token" || err != nil || *checks != 3 {
		t.Fatalf("approved: %q %v", tok, err)
	}
	if _, err := c.CheckPin(ctx, &Pin{ID: 1, Code: "x"}); !errors.Is(err, ErrPinExpired) {
		t.Errorf("expired pin: %v", err)
	}
	acc, err := c.Account(ctx, "account-token")
	if err != nil || acc.ID != 12345 || acc.Name != "SampleUser" {
		t.Errorf("account %+v %v", acc, err)
	}
	if _, err := c.Account(ctx, "wrong"); !errors.Is(err, ErrToken) {
		t.Errorf("wrong token: %v", err)
	}
	srv, err := c.Servers(ctx, "account-token")
	if err != nil || len(srv) != 2 || srv[0].ID != "srv-a" || !srv[0].Owned || srv[1].ID != "srv-b" || srv[1].Token != "shared-token" {
		t.Fatalf("servers %+v %v", srv, err)
	}
	if a := srv[0].Addresses(); strings.Join(a, " ") != "http://192.168.1.5:32400 https://remote.example:1234" {
		t.Errorf("own addresses %v", a)
	}
	if a := srv[1].Addresses(); strings.Join(a, " ") != "https://remote.example:32400 https://relay.example:443" {
		t.Errorf("shared addresses %v", a)
	}
}

// fakePMS is a Plex Media Server: identity, metadata, the session list and an event stream the test writes to.
type fakePMS struct {
	*httptest.Server
	events   chan string
	token    string
	sessions string
	mu       sync.Mutex
	streams  int
	metaHits map[string]int
}

func newFakePMS(t *testing.T, id, token string) *fakePMS {
	f := &fakePMS{events: make(chan string, 16), token: token, sessions: `{"MediaContainer":{"size":0}}`, metaHits: map[string]int{}}
	meta := map[string]string{
		"101": `{"MediaContainer":{"size":1,"Metadata":[{"ratingKey":"101","key":"/library/metadata/101","type":"episode","title":"The Pilot","grandparentTitle":"Sample Show","parentIndex":1,"index":2,"year":2019,"duration":1500000}]}}`,
		"102": `{"MediaContainer":{"size":1,"Metadata":[{"ratingKey":"102","key":"/library/metadata/102","type":"movie","title":"Sample Movie","year":2020,"duration":6000000}]}}`,
		"103": `{"MediaContainer":{"size":1,"Metadata":[{"ratingKey":"103","key":"/library/metadata/103","type":"track","title":"Sample Song","grandparentTitle":"Sample Band","duration":200000}]}}`,
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.token != "" && r.Header.Get("X-Plex-Token") != f.token {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/identity":
			fmt.Fprintf(w, `{"MediaContainer":{"size":0,"apiVersion":"1.2.3","claimed":true,"machineIdentifier":%q,"version":"1.43.4.10903-e5521bd8c"}}`, id)
		case strings.HasPrefix(r.URL.Path, "/library/metadata/"):
			k := strings.TrimPrefix(r.URL.Path, "/library/metadata/")
			f.mu.Lock()
			f.metaHits[k]++
			f.mu.Unlock()
			if m, ok := meta[k]; ok {
				fmt.Fprint(w, m)
			} else {
				http.NotFound(w, r)
			}
		case r.URL.Path == "/status/sessions":
			f.mu.Lock()
			fmt.Fprint(w, f.sessions)
			f.mu.Unlock()
		case r.URL.Path == "/:/eventsource/notifications" && r.URL.Query().Get("filters") == "playing":
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
					if e == "END" {
						return // the server ends the stream
					}
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

func playing(client, rk string, offset int, state string) string {
	return fmt.Sprintf("event: playing\ndata: {\"PlaySessionStateNotification\":{\"clientIdentifier\":%q,\"guid\":\"\",\"ratingKey\":%q,\"url\":\"\",\"key\":\"/library/metadata/%s\",\"viewOffset\":%d,\"state\":%q}}\n\n", client, rk, rk, offset, state)
}

const ping = "event: ping\ndata: {}\n\n"

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func startWatcher(t *testing.T, w *Watcher) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
}

func TestWatcherSharedServer(t *testing.T) {
	pms := newFakePMS(t, "srv-b", "shared-token")
	tv, _ := fakeTV(t, fmt.Sprintf(`[{"name":"Friend's server","provides":"server","owned":false,"clientIdentifier":"srv-b","accessToken":"shared-token","connections":[{"uri":"http://127.0.0.1:1","local":true,"relay":false},{"uri":%q,"local":false,"relay":false}]}]`, pms.URL))
	now := time.Unix(1000, 0)
	var mu sync.Mutex
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	var logs []string
	w := &Watcher{Client: &Client{ClientID: "test-client", TVBase: tv.URL, ResourcesBase: tv.URL}, Token: "account-token", Account: 12345, Server: "srv-b",
		Log: func(l, m string) { mu.Lock(); logs = append(logs, l+" "+m); mu.Unlock() }, now: clock}
	if w.Now() != nil {
		t.Fatal("nothing before connecting")
	}
	startWatcher(t, w)
	eventually(t, "connected", func() bool { return w.Now() != nil })
	if in := w.Now(); in.State != -1 || in.File != "" {
		t.Fatalf("idle %+v", in)
	}
	pms.events <- ping
	pms.events <- playing("player-1", "101", 60000, "playing")
	eventually(t, "playing", func() bool { return w.Now().State == 2 })
	in := w.Now()
	if in.File != "Sample Show - S01E02 - The Pilot" || in.Position != 60000 || in.Duration != 1500000 || in.FullDir != "" || in.Rate != 1 {
		t.Fatalf("playing %+v", in)
	}
	advance(4 * time.Second) // between reports the time runs on
	if p := w.Now().Position; p != 64000 {
		t.Errorf("position %d", p)
	}
	pms.events <- playing("player-1", "101", 70000, "paused")
	eventually(t, "paused", func() bool { return w.Now().State == 1 })
	advance(time.Minute)
	if in := w.Now(); in.Position != 70000 {
		t.Errorf("paused position runs on: %+v", in)
	}
	// a second player starts something: the playing one wins over the paused one
	pms.events <- playing("player-2", "102", 1000, "playing")
	eventually(t, "movie", func() bool { return w.Now().File == "Sample Movie (2020)" })
	// music is never "Watching"
	pms.events <- playing("player-2", "103", 0, "playing")
	eventually(t, "music skipped", func() bool { return w.Now().File == "Sample Show - S01E02 - The Pilot" })
	pms.events <- playing("player-2", "103", 0, "stopped")
	pms.events <- playing("player-1", "101", 70000, "stopped")
	eventually(t, "stopped", func() bool { in := w.Now(); return in != nil && in.State == -1 })
	// metadata is asked once per title
	pms.events <- playing("player-1", "101", 1000, "playing")
	eventually(t, "again", func() bool { return w.Now().State == 2 })
	pms.mu.Lock()
	if pms.metaHits["101"] != 1 {
		t.Errorf("metadata asked %d times", pms.metaHits["101"])
	}
	pms.mu.Unlock()
	// a player that goes quiet is forgotten
	advance(sessionTimeout + time.Second)
	if in := w.Now(); in.State != -1 {
		t.Errorf("quiet player still shown: %+v", in)
	}
	mu.Lock()
	joined := strings.Join(logs, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "Following the Plex server \"Friend's server\".") || strings.Contains(joined, "token") {
		t.Errorf("log:\n%s", joined)
	}
}

func TestWatcherOwnServerShowsOnlyMine(t *testing.T) {
	pms := newFakePMS(t, "srv-a", "own-token")
	tv, _ := fakeTV(t, fmt.Sprintf(`[{"name":"My server","provides":"server","owned":true,"clientIdentifier":"srv-a","accessToken":"own-token","connections":[{"uri":%q,"local":true,"relay":false}]}]`, pms.URL))
	pms.sessions = `{"MediaContainer":{"size":2,"Metadata":[
		{"ratingKey":"101","type":"episode","User":{"id":"1","title":"SampleUser"},"Player":{"machineIdentifier":"my-player","state":"playing"}},
		{"ratingKey":"102","type":"movie","User":{"id":"777","title":"Someone Else"},"Player":{"machineIdentifier":"their-player","state":"playing"}}]}}`
	w := &Watcher{Client: &Client{ClientID: "test-client", TVBase: tv.URL, ResourcesBase: tv.URL}, Token: "account-token", Account: 12345}
	startWatcher(t, w)
	eventually(t, "connected", func() bool { return w.Now() != nil })
	pms.events <- playing("their-player", "102", 5000, "playing")
	pms.events <- playing("my-player", "101", 9000, "playing")
	eventually(t, "mine", func() bool { return w.Now().State == 2 })
	time.Sleep(50 * time.Millisecond)
	if f := w.Now().File; f != "Sample Show - S01E02 - The Pilot" {
		t.Fatalf("shown %q", f)
	}
	pms.events <- playing("my-player", "101", 9000, "stopped")
	eventually(t, "only theirs left", func() bool { return w.Now().State == -1 })
}

func TestWatcherProblems(t *testing.T) {
	tv, _ := fakeTV(t, `[]`)
	w := &Watcher{Client: &Client{ClientID: "test-client", TVBase: tv.URL, ResourcesBase: tv.URL}, Token: "wrong"}
	startWatcher(t, w)
	eventually(t, "refused", func() bool { return strings.Contains(w.Problem(), "Sign in to Plex again") })

	w2 := &Watcher{Client: &Client{ClientID: "test-client", TVBase: tv.URL, ResourcesBase: tv.URL}, Token: "account-token"}
	startWatcher(t, w2)
	eventually(t, "no server", func() bool { return strings.Contains(w2.Problem(), "no server") })

	// a fixed address with no server chosen follows whatever answers there, plex.tv or not
	pms := newFakePMS(t, "srv-c", "")
	w3 := &Watcher{Client: &Client{ClientID: "test-client", TVBase: "http://127.0.0.1:1", ResourcesBase: "http://127.0.0.1:1"}, Token: "account-token", Address: pms.URL + "/"}
	startWatcher(t, w3)
	eventually(t, "fixed address", func() bool { return w3.Now() != nil })
	pms.events <- playing("p", "102", 0, "playing")
	eventually(t, "plays", func() bool { return w3.Now().File == "Sample Movie (2020)" })
	if w3.Problem() != "" {
		t.Errorf("problem %q", w3.Problem())
	}
}

func TestWatcherReconnects(t *testing.T) {
	pms := newFakePMS(t, "srv-c", "")
	w := &Watcher{Client: &Client{ClientID: "test-client"}, Address: pms.URL, retry: 20 * time.Millisecond}
	startWatcher(t, w)
	eventually(t, "connected", func() bool { return w.Now() != nil })
	pms.events <- "END"
	eventually(t, "noticed", func() bool { return w.Now() == nil })
	eventually(t, "back", func() bool { pms.mu.Lock(); defer pms.mu.Unlock(); return pms.streams == 2 && w.Now() != nil })
}

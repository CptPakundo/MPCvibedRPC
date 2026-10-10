package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
)

// fakePlexTV hands out a sign-in code that is approved on the third check (or never, when approve is false).
type fakePlexTV struct {
	*httptest.Server
	mu      sync.Mutex
	checks  int
	approve bool
	client  string // the X-Plex-Client-Identifier of the last request
}

func newFakePlexTV(t *testing.T) *fakePlexTV {
	f := &fakePlexTV{approve: true}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.client = r.Header.Get("X-Plex-Client-Identifier")
		tok := r.Header.Get("X-Plex-Token")
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v2/pins":
			fmt.Fprint(w, `{"id":77,"code":"samplecode","expiresIn":1800,"authToken":null}`)
		case r.URL.Path == "/api/v2/pins/77":
			f.checks++
			if f.approve && f.checks >= 3 {
				fmt.Fprint(w, `{"id":77,"code":"samplecode","authToken":"account-token"}`)
			} else {
				fmt.Fprint(w, `{"id":77,"code":"samplecode","authToken":null}`)
			}
		case r.URL.Path == "/api/v2/user" && tok == "account-token":
			fmt.Fprint(w, `{"id":12345,"username":"SampleUser","title":"Sample User"}`)
		case r.URL.Path == "/api/v2/resources" && tok == "account-token":
			fmt.Fprint(w, `[{"name":"Friend's server","provides":"server","owned":false,"clientIdentifier":"srv-b","accessToken":"t2","connections":[]},
				{"name":"My server","provides":"server","owned":true,"clientIdentifier":"srv-a","accessToken":"t1","connections":[]}]`)
		default:
			w.WriteHeader(401)
		}
	}))
	t.Cleanup(f.Server.Close)
	return f
}

func plexTestAccount(t *testing.T, tv *fakePlexTV) (*plexAccount, *store.Store, *[]string) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var logs []string
	lg := func(l, m string) { mu.Lock(); logs = append(logs, l+" "+m); mu.Unlock() }
	eng := engine.New(st.Config(), engine.Options{Log: lg})
	opened := ""
	p := &plexAccount{st: st, eng: eng, log: lg, tv: tv.URL, every: 10 * time.Millisecond, openURL: func(u string) bool { opened = u; return true }}
	t.Cleanup(p.stopWaiting)
	t.Cleanup(func() {
		if !strings.HasPrefix(opened, "https://app.plex.tv/auth#?") {
			t.Errorf("opened %q", opened)
		}
	})
	return p, st, &logs
}

func TestPlexSignInFlow(t *testing.T) {
	tv := newFakePlexTV(t)
	p, st, logs := plexTestAccount(t, tv)
	s, err := p.signIn()
	if err != nil || s["waiting"] != true || !strings.Contains(s["url"].(string), "code=samplecode") || s["signedIn"] != false {
		t.Fatalf("sign-in started: %v %v", s, err)
	}
	id := st.Config().PlexClient
	tv.mu.Lock()
	saw := tv.client
	tv.mu.Unlock()
	if len(id) != 32 || saw != id {
		t.Fatalf("client identifier %q (plex.tv saw %q)", id, saw)
	}
	waitFor(t, "signed in", func() bool { return p.state()["signedIn"] == true })
	s = p.state()
	if s["waiting"] != false || s["user"] != "SampleUser" || s["server"] != "srv-a" || s["serverName"] != "My server" || s["problem"] != "" {
		t.Errorf("state %v", s)
	}
	for k := range s {
		if strings.Contains(fmt.Sprint(s[k]), "account-token") {
			t.Errorf("the window would see the token in %s", k)
		}
	}
	c := st.Config()
	if c.PlexToken != "account-token" || c.PlexAccount != 12345 || c.PlexClient != id || p.eng.Config().PlexToken != "account-token" {
		t.Errorf("saved %+v", c)
	}
	list, err := p.servers()
	if err != nil || len(list) != 2 || list[0]["id"] != "srv-a" || list[1]["owned"] != false {
		t.Errorf("servers %v %v", list, err)
	}
	if err := p.choose("srv-b"); err != nil || st.Config().PlexServerName != "Friend's server" {
		t.Errorf("choose: %v %+v", err, st.Config())
	}
	if err := p.choose("nope"); err == nil {
		t.Error("an unknown server is refused")
	}
	// signing in again keeps the chosen server and the identifier
	tv.mu.Lock()
	tv.checks = 0
	tv.mu.Unlock()
	if _, err := p.signIn(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "signed in again", func() bool { return p.state()["waiting"] == false })
	if c := st.Config(); c.PlexServer != "srv-b" || c.PlexClient != id {
		t.Errorf("after signing in again %+v", c)
	}
	if err := p.signOut(); err != nil {
		t.Fatal(err)
	}
	c = st.Config()
	if c.PlexToken != "" || c.PlexUser != "" || c.PlexServer != "" || c.PlexClient != id || p.eng.Config().PlexToken != "" {
		t.Errorf("after signing out %+v", c)
	}
	if b, _ := os.ReadFile(st.File); strings.Contains(string(b), "account-token") {
		t.Error("the token stays in the settings file")
	}
	for _, l := range *logs {
		if strings.Contains(l, "account-token") || strings.Contains(l, "SampleUser") {
			t.Errorf("log line %q", l)
		}
	}
}

func TestPlexSignInCancelled(t *testing.T) {
	tv := newFakePlexTV(t)
	tv.approve = false
	p, st, _ := plexTestAccount(t, tv)
	if _, err := p.signIn(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a few checks", func() bool { tv.mu.Lock(); defer tv.mu.Unlock(); return tv.checks >= 2 })
	p.stopWaiting()
	if s := p.state(); s["waiting"] != false || s["url"] != "" {
		t.Errorf("cancelled: %v", s)
	}
	tv.mu.Lock()
	n := tv.checks
	tv.approve = true
	tv.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if tv.checks != n || st.Config().PlexToken != "" {
		t.Errorf("a cancelled sign-in went on (%d checks after %d)", tv.checks, n)
	}
}

func TestPlexSignInWithoutPlexTV(t *testing.T) {
	tv := newFakePlexTV(t)
	p, _, _ := plexTestAccount(t, tv)
	p.openURL("https://app.plex.tv/auth#?") // nothing opens when plex.tv is down
	p.tv = "http://127.0.0.1:1"
	if _, err := p.signIn(); err == nil || !strings.Contains(err.Error(), "Could not reach plex.tv") {
		t.Errorf("err %v", err)
	}
	if p.state()["waiting"] != false {
		t.Error("nothing waits")
	}
}

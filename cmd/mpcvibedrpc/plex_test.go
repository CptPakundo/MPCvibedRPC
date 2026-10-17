package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	time.Sleep(50 * time.Millisecond) // a check that was already on its way may still arrive
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

// keyToken is a JWT-shaped token that runs out after left (only its exp is read by the program).
func keyToken(n int, left time.Duration) string {
	c, _ := json.Marshal(map[string]any{"exp": time.Now().Add(left).Unix(), "n": n})
	return "e30." + base64.RawURLEncoding.EncodeToString(c) + ".sig"
}

// With plexJwt on, the sign-in hands plex.tv a key of this install's own and gets a 7-day token, which is renewed
// (nonce, signed JWT) once it nears its end; signing out forgets the key.
func TestPlexKeySignInAndRenewal(t *testing.T) {
	var mu sync.Mutex
	var jwk map[string]any
	left := 24 * time.Hour // the first token runs out within renewBefore
	issued, refreshes := 0, 0
	tv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v2/pins":
			var body struct {
				JWK map[string]any `json:"jwk"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			jwk = body.JWK
			fmt.Fprint(w, `{"id":78,"code":"samplecode"}`)
		case r.URL.Path == "/api/v2/pins/78" && r.URL.Query().Get("deviceJWT") != "" && jwk != nil:
			issued++
			fmt.Fprintf(w, `{"id":78,"authToken":%q}`, keyToken(issued, left))
		case r.URL.Path == "/api/v2/auth/nonce":
			fmt.Fprint(w, `{"nonce":"sample-nonce"}`)
		case r.Method == "POST" && r.URL.Path == "/api/v2/auth/token":
			refreshes++
			issued++
			fmt.Fprintf(w, `{"auth_token":%q}`, keyToken(issued, 7*24*time.Hour))
		case r.URL.Path == "/api/v2/user":
			fmt.Fprint(w, `{"id":12345,"username":"SampleUser","title":"Sample User"}`)
		case r.URL.Path == "/api/v2/resources":
			fmt.Fprint(w, `[{"name":"My server","provides":"server","owned":true,"clientIdentifier":"srv-a","accessToken":"t1","connections":[]}]`)
		default:
			w.WriteHeader(401)
		}
	}))
	t.Cleanup(tv.Close)
	p, st, logs := plexTestAccount(t, &fakePlexTV{Server: tv})
	if err := st.SetHidden([]string{"plexJwt"}, map[string]any{"plexJwt": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.signIn(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "signed in", func() bool { return p.state()["signedIn"] == true })
	c := st.Config()
	mu.Lock()
	sent := jwk
	mu.Unlock()
	if c.PlexKey == "" || sent == nil || sent["crv"] != "Ed25519" || strings.Contains(fmt.Sprint(sent), c.PlexKey[strings.Index(c.PlexKey, ".")+1:]) {
		t.Fatalf("key saved %v, plex.tv got %v (and never the private part)", c.PlexKey != "", sent)
	}
	first := c.PlexToken

	p.renew(context.Background()) // a day left: renewed
	if c = st.Config(); c.PlexToken == first || p.eng.Config().PlexToken != c.PlexToken {
		t.Fatalf("the token was not renewed")
	}
	p.renew(context.Background()) // seven days left: left alone
	mu.Lock()
	n := refreshes
	mu.Unlock()
	if n != 1 {
		t.Fatalf("%d renewals; want 1", n)
	}
	found := false
	for _, l := range *logs {
		found = found || strings.Contains(l, "Plex sign-in renewed.")
		if strings.Contains(l, c.PlexToken) || strings.Contains(l, c.PlexKey) {
			t.Errorf("log line %q shows a secret", l)
		}
	}
	if !found {
		t.Error("the renewal is not logged")
	}
	if err := p.signOut(); err != nil {
		t.Fatal(err)
	}
	if c = st.Config(); c.PlexKey != "" || c.PlexToken != "" {
		t.Errorf("after signing out: key %v, token %v", c.PlexKey != "", c.PlexToken != "")
	}
}

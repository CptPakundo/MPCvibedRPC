package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
)

// fakeJellyfinServer is a Jellyfin server (Emby: under /emby, no Quick Connect) for the sign-in. Quick Connect is
// approved on the second check.
type fakeJellyfinServer struct {
	*httptest.Server
	mu        sync.Mutex
	emby      bool
	checks    int
	loggedOut []string // tokens signed out
	devices   []string
}

func newFakeJellyfinServer(t *testing.T, emby bool) *fakeJellyfinServer {
	f := &fakeJellyfinServer{emby: emby}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		auth := r.Header.Get("Authorization")
		if i := strings.Index(auth, `DeviceId="`); i >= 0 {
			f.devices = append(f.devices, strings.SplitN(auth[i+10:], `"`, 2)[0])
		}
		path := r.URL.Path
		if emby {
			if !strings.HasPrefix(path, "/emby/") {
				http.NotFound(w, r)
				return
			}
			path = strings.TrimPrefix(path, "/emby")
		}
		switch {
		case path == "/System/Info/Public" && emby:
			fmt.Fprint(w, `{"ServerName":"Sample Server","Version":"4.9.0.0","Id":"e1"}`)
		case path == "/System/Info/Public":
			fmt.Fprint(w, `{"ServerName":"Sample Server","Version":"12.2.0","ProductName":"Jellyfin Server","Id":"j1"}`)
		case path == "/QuickConnect/Initiate" && !emby:
			fmt.Fprint(w, `{"Authenticated":false,"Secret":"qc","Code":"654321"}`)
		case path == "/QuickConnect/Connect" && r.URL.Query().Get("secret") == "qc":
			f.checks++
			fmt.Fprintf(w, `{"Authenticated":%t}`, f.checks >= 2)
		case path == "/Users/AuthenticateWithQuickConnect":
			fmt.Fprint(w, `{"User":{"Name":"sample","Id":"u1"},"AccessToken":"tok-qc"}`)
		case path == "/Users/AuthenticateByName":
			var b struct{ Username, Pw string }
			json.NewDecoder(r.Body).Decode(&b)
			if b.Username != "sample" || b.Pw != "secret" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"User":{"Name":"sample","Id":"u1"},"AccessToken":"tok-pw"}`)
		case path == "/Sessions/Logout":
			f.loggedOut = append(f.loggedOut, r.Header.Get("X-Emby-Token"))
			w.WriteHeader(204)
		case path == "/Sessions":
			fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(401)
		}
	}))
	t.Cleanup(f.Server.Close)
	return f
}

func jellyfinTestAccount(t *testing.T) (*jellyfinAccount, *store.Store, *[]string) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var logs []string
	lg := func(l, m string) { mu.Lock(); logs = append(logs, l+" "+m); mu.Unlock() }
	eng := engine.New(st.Config(), engine.Options{Log: lg})
	j := &jellyfinAccount{st: st, eng: eng, log: lg, every: 10 * time.Millisecond}
	t.Cleanup(j.stopWaiting)
	return j, st, &logs
}

func TestJellyfinQuickConnectSignIn(t *testing.T) {
	f := newFakeJellyfinServer(t, false)
	j, st, logs := jellyfinTestAccount(t)
	s, err := j.signIn(strings.TrimPrefix(f.URL, "http://")) // typed without http://
	if err != nil || s["waiting"] != true || s["code"] != "654321" || s["product"] != "Jellyfin" || s["server"] != "Sample Server" {
		t.Fatalf("%v %v", s, err)
	}
	waitFor(t, "signed in", func() bool { return j.state()["signedIn"] == true })
	cfg := st.Config()
	if cfg.JellyfinToken != "tok-qc" || cfg.JellyfinUser != "u1" || cfg.JellyfinAddress != f.URL || cfg.JellyfinProduct != "Jellyfin" ||
		cfg.JellyfinUserName != "sample" || cfg.JellyfinServerName != "Sample Server" || cfg.JellyfinDevice == "" {
		t.Fatalf("saved: %+v", cfg)
	}
	s = j.state()
	if s["waiting"] != false || s["user"] != "sample" || s["token"] != nil {
		t.Fatalf("state: %v", s)
	}
	for _, d := range f.devices {
		if d != cfg.JellyfinDevice {
			t.Fatalf("one device id throughout: %v", f.devices)
		}
	}
	if j.eng.Config().JellyfinToken != "tok-qc" {
		t.Fatal("the engine got the sign-in")
	}

	// signing out ends the session on the server and keeps the device id
	if err := j.signOut(); err != nil {
		t.Fatal(err)
	}
	cfg = st.Config()
	if cfg.JellyfinToken != "" || cfg.JellyfinAddress != "" || cfg.JellyfinDevice == "" || len(f.loggedOut) != 1 || f.loggedOut[0] != "tok-qc" {
		t.Fatalf("signed out: %+v %v", cfg, f.loggedOut)
	}
	if !strings.Contains(strings.Join(*logs, "\n"), "Signed out of Jellyfin.") {
		t.Fatal(*logs)
	}
}

func TestEmbyPasswordSignIn(t *testing.T) {
	f := newFakeJellyfinServer(t, true)
	j, st, logs := jellyfinTestAccount(t)
	s, err := j.signIn(f.URL)
	if err != nil || s["password"] != true || s["waiting"] != false || s["product"] != "Emby" {
		t.Fatalf("Emby asks for a password: %v %v", s, err)
	}
	if _, err := j.signInPassword(f.URL, "sample", "wrong"); err == nil || !strings.Contains(err.Error(), "refused that user name or password") {
		t.Fatalf("a wrong password: %v", err)
	}
	s, err = j.signInPassword(f.URL, "sample", "secret")
	if err != nil || s["signedIn"] != true || s["password"] != false {
		t.Fatalf("%v %v", s, err)
	}
	if cfg := st.Config(); cfg.JellyfinToken != "tok-pw" || cfg.JellyfinAddress != f.URL+"/emby" || cfg.JellyfinProduct != "Emby" {
		t.Fatalf("saved: %+v", cfg)
	}
	for _, l := range *logs {
		if strings.Contains(l, "secret") {
			t.Fatalf("the password reached the log: %q", l)
		}
	}
	raw := fmt.Sprint(st.Values())
	if strings.Contains(raw, "secret") || strings.Contains(raw, "tok-pw") {
		t.Fatal("the window's values hold the sign-in")
	}
}

func TestJellyfinSignInProblems(t *testing.T) {
	j, _, _ := jellyfinTestAccount(t)
	if _, err := j.signIn(""); err == nil {
		t.Fatal("no address")
	}
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	if _, err := j.signIn(other.URL); err == nil || !strings.Contains(err.Error(), "not as a Jellyfin or Emby server") {
		t.Fatalf("another program: %v", err)
	}
	other.Close()
	if _, err := j.signIn(other.URL); err == nil || !strings.Contains(err.Error(), "Could not reach") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("nothing there (and the address is not repeated): %v", err)
	}
}

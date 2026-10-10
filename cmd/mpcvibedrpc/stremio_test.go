package main

import (
	"encoding/json"
	"fmt"
	"io"
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

// fakeStremioLink hands out a sign-in code that is approved on the third check, and ends sessions on logout.
type fakeStremioLink struct {
	*httptest.Server
	mu      sync.Mutex
	checks  int
	page    string
	logouts []string
}

func newFakeStremioLink(t *testing.T) *fakeStremioLink {
	f := &fakeStremioLink{page: "https://www.stremio.com/link?code=SAMPLE"}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/api/v2/create":
			fmt.Fprintf(w, `{"result":{"code":"SAMPLE","link":%q,"qrcode":""}}`, f.page)
		case "/api/v2/read":
			f.checks++
			if f.checks >= 3 && r.URL.Query().Get("code") == "SAMPLE" {
				fmt.Fprint(w, `{"result":{"authKey":"stremio-key"}}`)
			} else {
				fmt.Fprint(w, `{"error":{"message":"Not linked yet","code":0}}`)
			}
		case "/api/logout":
			b, _ := io.ReadAll(r.Body)
			var body map[string]string
			_ = json.Unmarshal(b, &body)
			f.logouts = append(f.logouts, body["authKey"])
			fmt.Fprint(w, `{"result":{"success":true}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Server.Close)
	return f
}

func TestStremioSignInFlow(t *testing.T) {
	f := newFakeStremioLink(t)
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var logs []string
	lg := func(l, m string) { mu.Lock(); logs = append(logs, l+" "+m); mu.Unlock() }
	eng := engine.New(st.Config(), engine.Options{Log: lg})
	opened := ""
	s := &stremioAccount{st: st, eng: eng, log: lg, api: f.URL, every: 10 * time.Millisecond, openURL: func(u string) bool { opened = u; return true }}
	t.Cleanup(s.stopWaiting)

	got, err := s.signIn()
	if err != nil || got["waiting"] != true || got["url"] != f.page || got["signedIn"] != false || opened != f.page {
		t.Fatalf("sign-in started: %v %v (opened %q)", got, err, opened)
	}
	waitFor(t, "signed in", func() bool { return s.state()["signedIn"] == true })
	got = s.state()
	if got["waiting"] != false || got["problem"] != "" {
		t.Errorf("state %v", got)
	}
	for k := range got {
		if strings.Contains(fmt.Sprint(got[k]), "stremio-key") {
			t.Errorf("the window would see the key in %s", k)
		}
	}
	if st.Config().StremioKey != "stremio-key" || eng.Config().StremioKey != "stremio-key" {
		t.Errorf("not saved or not applied: %+v", st.Config())
	}
	if err := s.signOut(); err != nil {
		t.Fatal(err)
	}
	if st.Config().StremioKey != "" || eng.Config().StremioKey != "" {
		t.Error("the key stays after signing out")
	}
	if b, _ := os.ReadFile(st.File); strings.Contains(string(b), "stremio-key") {
		t.Error("the key stays in the settings file")
	}
	f.mu.Lock()
	if len(f.logouts) != 1 || f.logouts[0] != "stremio-key" {
		t.Errorf("the session was not ended on Stremio's side: %v", f.logouts)
	}
	f.mu.Unlock()
	for _, l := range logs {
		if strings.Contains(l, "stremio-key") {
			t.Errorf("log line %q", l)
		}
	}
}

func TestStremioSignInRefusesOtherPages(t *testing.T) {
	f := newFakeStremioLink(t)
	f.page = "https://stremio.example.org/link"
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	opened := false
	s := &stremioAccount{st: st, eng: engine.New(st.Config(), engine.Options{}), log: func(string, string) {}, api: f.URL,
		openURL: func(string) bool { opened = true; return true }}
	if _, err := s.signIn(); err == nil || !strings.Contains(err.Error(), "not on stremio.com") || opened {
		t.Fatalf("err %v, opened %v", err, opened)
	}
	s.api = "http://127.0.0.1:1"
	if _, err := s.signIn(); err == nil || !strings.Contains(err.Error(), "Could not reach Stremio") {
		t.Errorf("err %v", err)
	}
	if s.state()["waiting"] != false {
		t.Error("nothing waits")
	}
}

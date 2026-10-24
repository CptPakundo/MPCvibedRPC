package conns

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// reset empties the list between tests.
func reset() {
	mu.Lock()
	clear(hosts)
	mu.Unlock()
}

// rt answers every request itself: nothing leaves the test.
type rt struct{ urls []string }

func (r *rt) RoundTrip(req *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, req.URL.String())
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}

func TestCountsByHostOnly(t *testing.T) {
	reset()
	defer reset()
	t0 := time.UnixMilli(1_000_000)
	now = func() time.Time { return t0 }
	defer func() { now = time.Now }()

	inner := &rt{}
	c := &http.Client{Transport: Wrap(inner)}
	for _, u := range []string{
		"https://api.tvmaze.com/search/shows?q=Sample%20Show",
		"https://api.tvmaze.com/shows/1/episodes",
		"https://API.GitHub.com/repos/x/y/releases/latest",
		"http://192.168.1.20:8096/Sessions",
		"http://127.0.0.1:13579/variables.html",
		"http://localhost:8080/jsonrpc",
		"http://[::1]:9/x",
	} {
		res, err := c.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		t0 = t0.Add(time.Second)
	}
	if len(inner.urls) != 7 {
		t.Fatalf("the wrapper must pass every request on: %d", len(inner.urls))
	}
	got := List()
	if len(got) != 3 {
		t.Fatalf("want 3 hosts (this computer not counted), got %+v", got)
	}
	if got[0].Host != "192.168.1.20" || got[0].How != "" || got[0].Label != "" {
		t.Errorf("most recent first, an address you entered is unlisted: %+v", got[0])
	}
	if got[1].Host != "api.github.com" || got[1].How != core.HostAsked || got[1].Label != "GitHub" {
		t.Errorf("github: %+v", got[1])
	}
	if got[2].Host != "api.tvmaze.com" || got[2].Count != 2 || got[2].Label != "TVmaze" || got[2].Last != 1_001_000 {
		t.Errorf("tvmaze: %+v", got[2])
	}
	for _, e := range got {
		if strings.ContainsAny(e.Host, "/?") {
			t.Errorf("a host carries a path or query: %q", e.Host)
		}
	}
}

func TestKeepsAtMostMaxHosts(t *testing.T) {
	reset()
	defer reset()
	for i := 0; i < maxHosts+20; i++ {
		Note("host" + strings.Repeat("x", i) + ".test")
	}
	Note("host.test")
	if n := len(List()); n != maxHosts {
		t.Fatalf("%d hosts kept; want %d", n, maxHosts)
	}
}

// The default transport, once wrapped, counts a real request to a test server on this computer as local.
func TestWrapDefault(t *testing.T) {
	reset()
	defer reset()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := &http.Client{Transport: Wrap(nil)}
	res, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(List()) != 0 {
		t.Fatalf("a request to this computer was counted: %+v", List())
	}
}

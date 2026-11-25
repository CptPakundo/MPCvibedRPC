package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/CptPakundo/MPRP/internal/core"
	"github.com/CptPakundo/MPRP/internal/engine"
	"github.com/CptPakundo/MPRP/internal/jsonx"
)

// catalogStub answers every catalog request: IMDb's suggestions list one show, the others nothing.
type catalogStub struct {
	mu    sync.Mutex // the lookups ask several catalogs at once
	hosts []string
}

// asked is a copy of the hosts asked so far.
func (c *catalogStub) asked() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.hosts...)
}

func (c *catalogStub) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.hosts = append(c.hosts, req.URL.Host)
	c.mu.Unlock()
	body := `{}`
	if strings.Contains(req.URL.Path, "/suggestion/") {
		body = `{"d":[{"id":"tt0000003","l":"Sample Show","y":2019,"qid":"tvSeries"}]}`
	}
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func parseObj(t *testing.T, s string) *jsonx.Obj {
	t.Helper()
	v, err := jsonx.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return v.(*jsonx.Obj)
}

// The dialog's endpoints answer with the request number they were sent, so the window can drop an older answer.
func TestMatchEndpoints(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.RequestGapMs = map[string]int{}
	stub := &catalogStub{}
	a := &app{eng: engine.New(cfg, engine.Options{HTTPClient: &http.Client{Transport: stub}, Log: func(string, string) {}})}
	h := a.handlers()
	out, err := h["match-search"].Fn(parseObj(t, `{"q":"Sample Show","n":7}`))
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["n"] != float64(7) || len(m["catalogs"].([]string)) == 0 {
		t.Errorf("answer: %+v", m)
	}
	if s := jsonx.Stringify(toAny(t, m["results"]), 0); !strings.Contains(s, `"entry":"https://www.imdb.com/title/tt0000003/"`) {
		t.Errorf("results: %s", s)
	}
	out, _ = h["match-covers"].Fn(parseObj(t, `{"name":"Sample Show","year":2019,"kind":"series","catalog":"IMDb","n":3}`))
	if m := out.(map[string]any); m["n"] != float64(3) || m["covers"] == nil {
		t.Errorf("covers: %+v", m)
	}
	for _, host := range stub.asked() {
		if core.IsSourceID(host) || !strings.Contains(host, ".") {
			t.Errorf("odd host %q", host)
		}
	}

	// every catalog off: no request, and the window is told so by an empty list of catalogs
	cfg.ShowArtwork = false
	stub2 := &catalogStub{}
	a2 := &app{eng: engine.New(cfg, engine.Options{HTTPClient: &http.Client{Transport: stub2}})}
	out, _ = a2.handlers()["match-search"].Fn(parseObj(t, `{"q":"Sample Show","n":1}`))
	if m := out.(map[string]any); len(m["catalogs"].([]string)) != 0 || len(stub2.asked()) != 0 {
		t.Errorf("with lookups off: %+v, %v", m, stub2.asked())
	}
}

func toAny(t *testing.T, v any) any {
	t.Helper()
	b := jsonMarshal(t, v)
	p, err := jsonx.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The report holds the file's name but never its folder, what it was read as and matched, the fix, the catalogs on
// and the version.
func TestFixReport(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.DisabledSources = []string{"wikipedia"}
	body := parseObj(t, `{"file":"D:\\Videos\\Sample Show\\sample.show.s01e02.1080p.mkv","title":"Sample Show","season":1,"episode":2,"before":"The Sample Show (1998)",
		"fix":{"match":"Sample Show","entry":"https://www.imdb.com/title/tt0000003/","label":"Sample Show (2019) · Show · IMDb","episodeOffset":12}}`)
	got := fixReport(body, cfg, "0.0.0")
	want := strings.Join([]string{
		"File name: sample.show.s01e02.1080p.mkv",
		"Read as: Sample Show, season 1, episode 2",
		"Matched by itself: The Sample Show (1998)",
		`The fix: Sample Show (2019) · Show · IMDb (https://www.imdb.com/title/tt0000003/); episodes +12; for every file read as "Sample Show"`,
		"Catalogs on: TMDB (no key), IMDb, Cinemeta, TVmaze, AniList, Kitsu",
		"Version: 0.0.0",
	}, "\n")
	if got != want {
		t.Errorf("report:\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "Videos") || strings.Contains(got, `\`) {
		t.Error("the report holds a folder")
	}
	got = fixReport(parseObj(t, `{"fix":{"match":"sample.movie.mkv","file":true,"search":"Sample Film","cover":"https://img.example/a.jpg"}}`), cfg, "0.0.0")
	if !strings.Contains(got, "File name: (none") || !strings.Contains(got, `The fix: searched as "Sample Film"; their own cover image; for this file only`) {
		t.Errorf("a fix made by hand:\n%s", got)
	}
}

func jsonMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

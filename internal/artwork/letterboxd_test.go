package artwork

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CptPakundo/MPRP/internal/core"
)

// fakeLetterboxd is Cinemeta with one film (tt0000001) and Letterboxd answering with lb (the status for that id:
// 302 to its page, 200 for "not found", or a block such as 403).
type fakeLetterboxd struct {
	mu   sync.Mutex
	urls []string
	lb   int
}

func (f *fakeLetterboxd) asked(host string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.urls {
		if strings.Contains(u, "://"+host+"/") {
			n++
		}
	}
	return n
}

func (f *fakeLetterboxd) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.urls = append(f.urls, req.URL.String())
	lb := f.lb
	f.mu.Unlock()
	res := &http.Response{StatusCode: 200, Header: http.Header{}, Request: req}
	body := "{}"
	switch {
	case req.URL.Host == "v3-cinemeta.strem.io" && strings.HasPrefix(req.URL.Path, "/catalog/movie/"):
		body = `{"metas":[{"imdb_id":"tt0000001","name":"Sample Movie","releaseInfo":"2020","poster":"https://example.invalid/p.jpg"}]}`
	case req.URL.Host == "letterboxd.com" && req.URL.Path == "/imdb/tt0000001/":
		res.StatusCode = lb
		if lb == http.StatusFound {
			res.Header.Set("Location", "https://letterboxd.com/film/sample-movie/")
		}
		body = "<html></html>"
	case req.URL.Host == "letterboxd.com":
		body = "<html>not found</html>"
	}
	res.Body = io.NopCloser(strings.NewReader(body))
	return res, nil
}

func letterboxdArtwork(t *testing.T, on bool, lb int) (*Artwork, *fakeLetterboxd, string) {
	cfg := core.DefaultConfig()
	cfg.ArtworkSources, cfg.EpisodeSources, cfg.RichInfo = []string{"cinemeta"}, nil, false
	cfg.ArtworkWaitMs = 10000
	cfg.LetterboxdLink = on
	fake := &fakeLetterboxd{lb: lb}
	file := filepath.Join(t.TempDir(), "c.json")
	return New(&cfg, Options{CacheFile: file, Client: &http.Client{Transport: fake}}), fake, file
}

func sampleMovie() *core.Media {
	m := core.NewMedia()
	m.Title, m.Year = "Sample Movie", 2020
	return m
}

// With the switch on, a film Letterboxd has gets its page, asked once; the button and the lines link it.
func TestLetterboxdPageFound(t *testing.T) {
	a, fake, _ := letterboxdArtwork(t, true, http.StatusFound)
	defer a.Close()
	art := a.Lookup(sampleMovie())
	if art == nil || art.Letterboxd != "https://letterboxd.com/film/sample-movie/" {
		t.Fatalf("art = %+v (requests %v)", art, fake.urls)
	}
	if a.Lookup(sampleMovie()).Letterboxd == "" || fake.asked("letterboxd.com") != 1 {
		t.Fatalf("a cached title asked Letterboxd again: %v", fake.urls)
	}
	cfg := a.cfg
	if u, site := core.TitleLink(cfg, art); u != art.Letterboxd || site != "Letterboxd" {
		t.Errorf("TitleLink = %q, %q", u, site)
	}
	off := *cfg
	off.LetterboxdLink = false
	if u, site := core.TitleLink(&off, art); u != "https://www.imdb.com/title/tt0000001/" || site != "IMDb" {
		t.Errorf("switch off: TitleLink = %q, %q", u, site)
	}
	off.LinkButton = false
	if u, _ := core.TitleLink(&off, art); u != "" {
		t.Errorf("linkButton off: TitleLink = %q", u)
	}
	b, _ := json.Marshal(art)
	var back core.Art
	if err := json.Unmarshal(b, &back); err != nil || back.Letterboxd != art.Letterboxd {
		t.Errorf("cache round trip: %s -> %+v (%v)", b, back, err)
	}
}

// A title Letterboxd does not have is remembered as such and keeps the catalog link.
func TestLetterboxdNotFound(t *testing.T) {
	a, fake, _ := letterboxdArtwork(t, true, http.StatusOK)
	defer a.Close()
	art := a.Lookup(sampleMovie())
	if art == nil || art.Letterboxd != "" || !art.LbChecked {
		t.Fatalf("art = %+v (requests %v)", art, fake.urls)
	}
	a.Lookup(sampleMovie())
	if n := fake.asked("letterboxd.com"); n != 1 {
		t.Errorf("Letterboxd asked %d times; the answer should be kept", n)
	}
	if u, site := core.TitleLink(a.cfg, art); site != "IMDb" || u == "" {
		t.Errorf("TitleLink = %q, %q", u, site)
	}
	b, _ := json.Marshal(art)
	var back core.Art
	if err := json.Unmarshal(b, &back); err != nil || !back.LbChecked {
		t.Errorf("cache round trip lost the checked mark: %s", b)
	}
}

// A blocked or failed request is not taken for "no page": nothing is remembered, and it is asked again later.
func TestLetterboxdBlocked(t *testing.T) {
	a, fake, _ := letterboxdArtwork(t, true, http.StatusForbidden)
	defer a.Close()
	art := a.Lookup(sampleMovie())
	if art == nil || art.Letterboxd != "" || art.LbChecked {
		t.Fatalf("art = %+v (requests %v)", art, fake.urls)
	}
}

// Switched off (the default), Letterboxd is never contacted.
func TestLetterboxdOffAsksNothing(t *testing.T) {
	if core.DefaultConfig().LetterboxdLink {
		t.Fatal("letterboxdLink must be off by default")
	}
	a, fake, _ := letterboxdArtwork(t, false, http.StatusFound)
	defer a.Close()
	if art := a.Lookup(sampleMovie()); art == nil || art.Letterboxd != "" {
		t.Fatalf("art = %+v", art)
	}
	if n := fake.asked("letterboxd.com"); n != 0 {
		t.Errorf("Letterboxd asked %d times with the switch off", n)
	}
}

// Only a redirect to a film page on Letterboxd itself counts.
func TestLetterboxdOddRedirect(t *testing.T) {
	for _, loc := range []string{"https://example.invalid/film/x/", "https://letterboxd.com/sign-in/"} {
		a, _, _ := letterboxdArtwork(t, true, 0)
		a.net.Client = &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {loc}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		})}
		if page, ok := a.letterboxdPage("/imdb/tt0000001/"); page != "" || ok {
			t.Errorf("%s: page %q, ok %v", loc, page, ok)
		}
		a.Close()
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

package artwork

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// A film named "<Title>: A <Franchise> Mystery" loses its colon in a file name; the catalogs list "<Title>".
func TestFranchiseSubtitle(t *testing.T) {
	movie := func(title string, year int) *core.Media {
		m := core.NewMedia()
		m.Title, m.Year = title, year
		return m
	}
	got := franchiseSubtitle(movie("Sample Case Closed A Sample Detective Mystery", 2020), "Sample Case Closed A Sample Detective Mystery")
	if len(got) != 1 || got[0].query != "Sample Case Closed" || got[0].subtitle != "A Sample Detective Mystery" {
		t.Fatalf("got %+v", got)
	}
	got = franchiseSubtitle(movie("Rogue Sample A Star Sample Story", 2016), "Rogue Sample A Star Sample Story")
	if len(got) != 1 || got[0].query != "Rogue Sample" || got[0].subtitle != "A Star Sample Story" {
		t.Fatalf("story: %+v", got)
	}
	for _, c := range []struct {
		title string
		year  int
	}{
		{"Sample Case Closed A Sample Detective Mystery", core.NA}, // no year: the catalog's year cannot confirm it
		{"Sample A Sample Detective Mystery", 2020},                // the title alone is too short to be distinctive
		{"A Sample Story", 2020},                                   // nothing before the subtitle
		{"Sample Movie Something Else", 2020},                      // no subtitle of that form
	} {
		if got := franchiseSubtitle(movie(c.title, c.year), c.title); got != nil {
			t.Errorf("%q (%d): want nothing, got %+v", c.title, c.year, got)
		}
	}
	ep := movie("Sample Show A Sample Detective Mystery", 2020)
	ep.IsEpisode = true
	if got := franchiseSubtitle(ep, ep.Title); got != nil {
		t.Errorf("episodes are left alone: %+v", got)
	}
}

// fakeTVmaze knows one show, only under its English name, with three seasons.
type fakeTVmaze struct {
	mu   sync.Mutex
	urls []string
}

func (f *fakeTVmaze) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.urls = append(f.urls, req.URL.String())
	f.mu.Unlock()
	body := "[]"
	switch {
	case req.URL.Host == "api.tvmaze.com" && req.URL.Path == "/search/shows" && req.URL.Query().Get("q") == "Sample Quest: Another World":
		body = `[{"show":{"id":77,"name":"Sample Quest: Another World","premiered":"2016-04-04"}}]`
	case req.URL.Host == "api.tvmaze.com" && req.URL.Path == "/shows/77/episodes":
		var eps []string
		for s, n := range []int{25, 25, 16} {
			for e := 1; e <= n; e++ {
				eps = append(eps, `{"season":`+core.Itoa(s+1)+`,"number":`+core.Itoa(e)+`,"type":"regular","name":"Sample Episode S`+core.Itoa(s+1)+`E`+core.Itoa(e)+`"}`)
			}
		}
		body = "[" + strings.Join(eps, ",") + "]"
	}
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// A show matched on AniList under its romaji name is asked about again under its English name, and a running episode
// number is placed in the right season.
func TestEpisodeTitleUnderEnglishName(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.EpisodeSources = []string{"tvmaze"}
	fake := &fakeTVmaze{}
	a := New(&cfg, Options{CacheFile: filepath.Join(t.TempDir(), "c.json"), Client: &http.Client{Transport: fake}})
	r := newResolver(a)
	hit := &core.Art{Name: "Sample Kara Hajimeru", Type: "series", Source: "AniList", Year: 2016, SeasonNo: core.NA,
		Alt: &core.ArtAlt{English: "Sample Quest: Another World", Romaji: "Sample Kara Hajimeru"}, IDs: &core.ArtIDs{}}
	media := core.NewMedia()
	media.Title, media.IsEpisode, media.Episode = "Sample Kara Hajimeru", true, 30 // season 2, episode 5
	ep := r.Find(media, hit, "Sample Kara Hajimeru", func(string, string) {})
	if ep == nil || ep.Title != "Sample Episode S2E5" || ep.Via != "Sample Quest: Another World" {
		t.Fatalf("running number 30: got %+v (requests %v)", ep, fake.urls)
	}

	// a show the TV catalogs are asked about by IMDb id is not searched by name again
	fake.urls = nil
	hit.IDs = &core.ArtIDs{Imdb: "tt0000001"}
	_ = r.Find(media, hit, "Sample Kara Hajimeru", func(string, string) {})
	for _, u := range fake.urls {
		if strings.Contains(u, "Another%20World") || strings.Contains(u, "Another+World") {
			t.Errorf("searched by the English name although there is an IMDb id: %s", u)
		}
	}
}

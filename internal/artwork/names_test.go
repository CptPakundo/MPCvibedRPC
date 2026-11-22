package artwork

import (
	"io"
	"net/http"
	"path/filepath"
	"strconv"
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
				eps = append(eps, `{"season":`+strconv.Itoa(s+1)+`,"number":`+strconv.Itoa(e)+`,"type":"regular","name":"Sample Episode S`+strconv.Itoa(s+1)+`E`+strconv.Itoa(e)+`"}`)
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

// A season matched under a name of its own ("Sample Kara Hajimeru II: Subtitle") that no episode source knows is
// asked about as that season of the show, by its English name without the season ("... Season 2").
func TestEpisodeTitleOfSplitSeason(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.EpisodeSources = []string{"tvmaze"}
	fake := &fakeTVmaze{}
	a := New(&cfg, Options{CacheFile: filepath.Join(t.TempDir(), "c.json"), Client: &http.Client{Transport: fake}})
	r := newResolver(a)
	hit := &core.Art{Name: "Sample Kara Hajimeru II: Subtitle", Type: "series", Source: "AniList", Year: 2018, SeasonNo: core.NA,
		Split: true, Alt: &core.ArtAlt{English: "Sample Quest: Another World Season 2", Romaji: "Sample Kara Hajimeru II: Subtitle"},
		IDs: &core.ArtIDs{}}
	media := core.NewMedia()
	media.Title, media.IsEpisode, media.Season, media.Episode = "Sample Kara Hajimeru", true, 2, 5
	ep := r.Find(media, hit, "Sample Kara Hajimeru", func(string, string) {})
	if ep == nil || ep.Title != "Sample Episode S2E5" || ep.Via != "Sample Quest: Another World" {
		t.Fatalf("season 2 episode 5: got %+v (requests %v)", ep, fake.urls)
	}
	// the English name's season must be the file's: another season is not guessed
	media.Season = 3
	if ep := r.Find(media, hit, "Sample Kara Hajimeru", func(string, string) {}); ep != nil {
		t.Errorf("season 3 file, season 2 entry: got %+v", ep)
	}
}

func TestSeasonOfName(t *testing.T) {
	for _, c := range []struct {
		name, base string
		n          int
	}{
		{"Sample Quest: Another World Season 2", "Sample Quest: Another World", 2},
		{"Sample Quest 2nd Season", "Sample Quest", 2},
		{"Sample Quest - Season 10", "Sample Quest", 10},
		{"Sample Quest Part 2", "", core.NA}, // a part is not a season
		{"Sample Quest", "", core.NA},
		{"Season 2", "", core.NA},
	} {
		if base, n := seasonOfName(c.name); base != c.base || n != c.n {
			t.Errorf("seasonOfName(%q) = %q, %d; want %q, %d", c.name, base, n, c.base, c.n)
		}
	}
}

// "Show II: Subtitle" is season 2, and "Show: Subtitle Part 2" next to a listed "Show: Subtitle" is the second half of
// that entry, not season 2.
func TestSeasonEntryRomanAndParts(t *testing.T) {
	cands := []*Cand{
		{Titles: []string{"Sample Kara: Isekai Subtitle"}, Year: 2021, Poster: "s1"},
		{Titles: []string{"Sample Kara: Isekai Subtitle Part 2"}, Year: 2021, Poster: "s1p2"},
		{Titles: []string{"Sample Kara II: Isekai Subtitle"}, Year: 2023, Poster: "s2"},
		{Titles: []string{"Sample Kara II: Isekai Subtitle Part 2"}, Year: 2024, Poster: "s2p2"},
	}
	if got := pickSeasonEntry(cands, "Sample Kara", 2, ""); got == nil || got.Cand.Poster != "s2" {
		t.Errorf("season 2: got %+v", got)
	}
	if got := pickSeasonEntry(cands, "Sample Kara", 3, ""); got != nil {
		t.Errorf("season 3: got %+v", got.Cand)
	}
	// a part with nothing listed before it still stands for its number, as before
	solo := []*Cand{{Titles: []string{"Sample Kara: Part 2"}, Year: 2022, Poster: "p2"}}
	if got := pickSeasonEntry(solo, "Sample Kara", 2, ""); got == nil || got.Cand.Poster != "p2" {
		t.Errorf("lone part 2: got %+v", got)
	}
}

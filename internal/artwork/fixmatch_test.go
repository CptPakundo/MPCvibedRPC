package artwork

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPRP/internal/core"
)

// A junk name (a bare number such as a Blu-ray's "00062") is never looked up, unless the user fixed it in Fix a match.
func TestFixAMatchWinsOverJunk(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.ArtworkSources, cfg.EpisodeSources, cfg.RichInfo = []string{}, []string{}, false // no catalog is asked
	media := core.ParseMedia("00062.m2ts", &cfg, nil)
	if !media.Junk {
		t.Fatalf("00062.m2ts should be a junk name: %+v", media)
	}
	if art := New(&cfg, Options{}).Lookup(media); art != nil {
		t.Fatalf("a junk name without a fix was looked up: %+v", art)
	}

	const poster = "https://images.example/sample-movie.jpg"
	cfg.MatchFixes = []core.MatchFix{{Match: "00062", Cover: poster}}
	art := New(&cfg, Options{}).Lookup(media)
	if art == nil || art.Poster != poster || art.Source != "override" {
		t.Fatalf("the pinned cover of a junk name was not used: %+v", art)
	}

	cfg.MatchFixes = []core.MatchFix{{Match: "00062", Search: "Sample Movie"}}
	a := New(&cfg, Options{})
	if a.FixFor(media) == nil || a.FixFor(core.ParseMedia("00063.m2ts", &cfg, nil)) != nil {
		t.Fatal("a fix should mark exactly its own title as fixed")
	}
}

// Old artworkAliases and artworkOverrides lines act as before while config.json has no matchFixes: the same search,
// the same cache key, the same pinned cover.
func TestLegacyLinesActAsBefore(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.ArtworkSources, cfg.EpisodeSources = []string{}, []string{}
	cfg.ArtworkAliases = map[string]string{"sample movie": "Sample Film", "Sample Show": "Sample Series"}
	cfg.ArtworkOverrides = map[string]string{"sample show": "https://img.example/show.jpg", "other": "not an address"}
	if cfg.MatchFixes != nil {
		t.Fatal("the default must be nil (no matchFixes key)")
	}
	a := New(&cfg, Options{})
	movie := core.ParseMedia("Sample Movie (2020).mkv", &cfg, nil)
	j := a.newLookupJob(movie)
	if j.query != "Sample Film" || j.key != keyVersion+"m|"+normTitle("Sample Movie")+"|2020|"+normTitle("Sample Film")+"||" {
		t.Errorf("alias: query %q, key %q", j.query, j.key)
	}
	plain := core.ParseMedia("Another Movie (2020).mkv", &cfg, nil)
	if j := a.newLookupJob(plain); j.key != keyVersion+"m|"+normTitle("Another Movie")+"|2020|"+normTitle("")+"||" || j.fix != nil {
		t.Errorf("a title nobody fixed must keep its key: %q", j.key)
	}
	show := core.ParseMedia("Sample Show S01E02.mkv", &cfg, nil)
	if art := a.Lookup(show); art == nil || art.Source != "override" || art.Poster != "https://img.example/show.jpg" {
		t.Errorf("a title in both maps keeps its pinned cover (it always won): %+v", art)
	}
	// a pinned cover that is no image address is not used, as before
	var warned []string
	a = New(&cfg, Options{Log: func(_, m string) { warned = append(warned, m) }})
	if art := a.Lookup(core.ParseMedia("Other.mkv", &cfg, nil)); art != nil {
		t.Errorf("a bad cover address was used: %+v", art)
	}
	if strings.Count(strings.Join(warned, " / "), "http(s) image address") != 1 {
		t.Errorf("warnings: %q", warned)
	}

	// an empty matchFixes (saved by the window) means no fixes: the old lines are no longer read
	cfg.MatchFixes = []core.MatchFix{}
	if New(&cfg, Options{}).FixFor(movie) != nil {
		t.Error("with matchFixes saved, the legacy lines must not be read")
	}
}

// A fix for one file wins over a fix for its title, and only matches that file name (any case).
func TestFileFixBeatsTitleFix(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.MatchFixes = []core.MatchFix{
		{Match: "Sample Show", Search: "Sample Series"},
		{Match: "sample.show.s01e02.mkv", File: true, Search: "Sample Special"},
	}
	a := New(&cfg, Options{})
	m := core.ParseMedia("Sample.Show.S01E02.mkv", &cfg, nil)
	if fx := a.FixFor(m); fx == nil || fx.Search != "Sample Series" {
		t.Fatalf("without a file name the title fix applies: %+v", fx)
	}
	m.File = "Sample.Show.S01E02.mkv"
	if fx := a.FixFor(m); fx == nil || fx.Search != "Sample Special" {
		t.Errorf("the file fix should win: %+v", fx)
	}
	m2 := core.ParseMedia("Sample.Show.S01E03.mkv", &cfg, nil)
	m2.File = "Sample.Show.S01E03.mkv"
	if fx := a.FixFor(m2); fx == nil || fx.Search != "Sample Series" {
		t.Errorf("another episode gets the title fix: %+v", fx)
	}
}

// fakeCatalogs answers by path, counts requests per host and when they came.
type fakeCatalogs struct {
	mu     sync.Mutex
	routes []fakeRoute
	hosts  map[string][]time.Time
	paths  []string
}

type fakeRoute struct {
	re   *regexp.Regexp
	body string
}

func newFakeCatalogs(routes ...string) *fakeCatalogs {
	f := &fakeCatalogs{hosts: map[string][]time.Time{}}
	for i := 0; i+1 < len(routes); i += 2 {
		f.routes = append(f.routes, fakeRoute{regexp.MustCompile(routes[i]), routes[i+1]})
	}
	return f
}

func (f *fakeCatalogs) RoundTrip(req *http.Request) (*http.Response, error) {
	where := req.URL.Host + req.URL.RequestURI()
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		where += " " + string(b)
	}
	f.mu.Lock()
	f.hosts[req.URL.Host] = append(f.hosts[req.URL.Host], time.Now())
	f.paths = append(f.paths, where)
	f.mu.Unlock()
	body, status := "{}", 200
	for _, r := range f.routes {
		if r.re.MatchString(where) {
			body = r.body
			break
		}
	}
	if body == "404" {
		body, status = `{"status":404}`, 404
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func (f *fakeCatalogs) count(host string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.hosts[host])
}

func (f *fakeCatalogs) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.paths)
}

const (
	imdbEntryJSON  = `{"d":[{"id":"tt0000001","l":"The Sample Show","y":1998,"qid":"tvSeries","i":{"imageUrl":"https://img.example/ts.jpg"}}]}`
	tvmazeShowJSON = `{"id":42,"name":"Sample Show","premiered":"2019-01-01","url":"https://www.tvmaze.com/shows/42/sample-show","image":{"medium":"https://img.example/ss.jpg"},"externals":{}}`
	tvmazeSeasons  = `[{"number":1,"name":"","image":{"medium":"https://img.example/ss-s1.jpg"}}]`
)

func pinConfig() core.Config {
	cfg := core.DefaultConfig()
	cfg.RequestGapMs = map[string]int{}
	cfg.EpisodeTitles, cfg.RichInfo, cfg.LinkButton = false, false, false
	cfg.AnimeTitles = "file"
	return cfg
}

// A pinned page is read by its id, from the catalog it belongs to, and a show gets its season's poster.
func TestPinnedEntry(t *testing.T) {
	fake := newFakeCatalogs(
		`^v3\.sg\.media-imdb\.com/suggestion/t/tt0000001\.json`, imdbEntryJSON,
		`^api\.tvmaze\.com/shows/42/seasons`, tvmazeSeasons,
		`^api\.tvmaze\.com/shows/42$`, tvmazeShowJSON,
		`^graphql\.anilist\.co.*Media\(id`, `{"data":{"Media":{"siteUrl":"https://anilist.co/anime/7","format":"MOVIE","title":{"romaji":"Sample Eiga","english":"Sample Movie"},"startDate":{"year":2020},"coverImage":{"large":"https://img.example/sm.jpg"}}}}`,
		`^kitsu\.io/api/edge/anime\?filter\[slug\]=sample-anime`, `{"data":[{"id":"9","attributes":{"subtype":"TV","canonicalTitle":"Sample Anime","slug":"sample-anime","startDate":"2018-04-01","posterImage":{"large":"https://img.example/sa.jpg"}}}]}`,
	)
	cases := []struct {
		entry, file, name, source, typ, poster string
	}{
		{"https://www.imdb.com/title/tt0000001/", "Sample Show S01E02.mkv", "The Sample Show", "IMDb", "series", "https://img.example/ts.jpg"},
		{"https://www.tvmaze.com/shows/42/sample-show", "Sample Show S01E02.mkv", "Sample Show", "TVmaze", "series", "https://img.example/ss-s1.jpg"},
		{"https://anilist.co/anime/7/sample-eiga", "Sample Eiga.mkv", "Sample Eiga", "AniList", "movie", "https://img.example/sm.jpg"},
		{"https://kitsu.io/anime/sample-anime", "Sample Anime - 03.mkv", "Sample Anime", "Kitsu", "series", "https://img.example/sa.jpg"},
	}
	for _, c := range cases {
		cfg := pinConfig()
		media := core.ParseMedia(c.file, &cfg, nil)
		cfg.MatchFixes = []core.MatchFix{{Match: media.Title, Entry: c.entry}}
		before := fake.total()
		art := New(&cfg, Options{Client: &http.Client{Transport: fake}}).Lookup(media)
		if art == nil || art.Name != c.name || art.Source != c.source || art.Type != c.typ || art.Poster != c.poster {
			t.Errorf("%s: %+v", c.entry, art)
			continue
		}
		if fake.total()-before > 2 {
			t.Errorf("%s: %d requests, want one or two (no search)", c.entry, fake.total()-before)
		}
		if !isPinned(&cfg.MatchFixes[0], art) {
			t.Errorf("%s: not recognised as the pinned entry: %+v", c.entry, art)
		}
	}

	// a user's image over a pinned entry
	cfg := pinConfig()
	media := core.ParseMedia("Sample Show S01E02.mkv", &cfg, nil)
	cfg.MatchFixes = []core.MatchFix{{Match: "sample show", Entry: "https://www.imdb.com/title/tt0000001/", Cover: "https://img.example/mine.jpg"}}
	if art := New(&cfg, Options{Client: &http.Client{Transport: fake}}).Lookup(media); art == nil || art.Name != "The Sample Show" || art.Poster != "https://img.example/mine.jpg" {
		t.Errorf("cover over a pinned entry: %+v", art)
	}
}

// A pinned page that is gone, or whose catalog is off under Privacy, falls back to searching the read title, with a
// warning; a switched-off catalog is never asked.
func TestPinnedEntryFallsBack(t *testing.T) {
	fake := newFakeCatalogs(
		`^v3\.sg\.media-imdb\.com/suggestion/t/tt0000002\.json`, `{"d":[]}`,
		`^v3\.sg\.media-imdb\.com/suggestion/s/sample%20show\.json`, `{"d":[{"id":"tt0000003","l":"Sample Show","y":2019,"qid":"tvSeries","i":{"imageUrl":"https://img.example/found.jpg"}}]}`,
		`^v3-cinemeta\.strem\.io/meta/`, `{}`,
		`^api\.tvmaze\.com/lookup/shows`, `404`,
	)
	for _, off := range [][]string{nil, {"imdb", "cinemeta", "tvmaze"}} {
		cfg := pinConfig()
		cfg.ArtworkSources = []string{"imdb"}
		cfg.DisabledSources = off
		if off != nil {
			cfg.ArtworkSources = []string{"imdb", "anilist"}
			cfg.DisabledSources = []string{"cinemeta", "tvmaze"} // the page's catalogs but IMDb are off
		}
		media := core.ParseMedia("Sample Show S01E02.mkv", &cfg, nil)
		cfg.MatchFixes = []core.MatchFix{{Match: "Sample Show", Entry: "https://www.imdb.com/title/tt0000002/"}}
		var logs []string
		var mu sync.Mutex
		before := fake.count("api.tvmaze.com")
		art := New(&cfg, Options{Client: &http.Client{Transport: fake}, Log: func(l, m string) { mu.Lock(); logs = append(logs, l+" "+m); mu.Unlock() }}).Lookup(media)
		if art == nil || art.Name != "Sample Show" || art.Poster != "https://img.example/found.jpg" {
			t.Errorf("off %v: no fallback: %+v", off, art)
		}
		mu.Lock()
		if !strings.Contains(strings.Join(logs, "\n"), "WARN The entry pinned for \"Sample Show\" was not found") {
			t.Errorf("off %v: no warning: %q", off, logs)
		}
		mu.Unlock()
		if off != nil && fake.count("api.tvmaze.com") != before {
			t.Errorf("TVmaze is off but was asked")
		}
	}
}

// The dialog's search asks at most three catalogs that are on, through the usual network path, and gives each entry
// once, exact names first; the same search again asks nothing.
func TestSuggest(t *testing.T) {
	fake := newFakeCatalogs(
		`^v3\.sg\.media-imdb\.com/suggestion/s/sample%20show\.json`, `{"d":[{"id":"tt0000004","l":"Sample Show Returns","y":2021,"qid":"tvSeries"},{"id":"tt0000003","l":"Sample Show","y":2019,"qid":"tvSeries","i":{"imageUrl":"https://img.example/a.jpg"}},{"id":"tt0000005","l":"Sample Show: The Movie","y":2022,"qid":"movie"}]}`,
		`^api\.tvmaze\.com/search/shows`, `[{"show":{"id":42,"name":"Sample Show","premiered":"2019-01-01","url":"https://www.tvmaze.com/shows/42/sample-show","externals":{"imdb":"tt0000003"}}},{"show":{"id":43,"name":"Sample Show","premiered":"2024-01-01","url":"https://www.tvmaze.com/shows/43/sample-show"}}]`,
		`^graphql\.anilist\.co`, `{"data":{"Page":{"media":[]}}}`,
	)
	cfg := pinConfig()
	a := New(&cfg, Options{Client: &http.Client{Transport: fake}})
	cats := a.SuggestCatalogs()
	var names []string
	for _, p := range cats {
		names = append(names, p.Label)
	}
	if strings.Join(names, ",") != "IMDb,TVmaze,AniList" {
		t.Errorf("catalogs %v, want IMDb, TVmaze and AniList (TMDB has no key, Cinemeta lists IMDb's entries)", names)
	}
	list, err := a.Suggest("Sample Show")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range list {
		got = append(got, s.Name+"|"+s.Catalog+"|"+s.Kind+"|"+s.Entry)
	}
	want := []string{
		"Sample Show|IMDb|series|https://www.imdb.com/title/tt0000003/",
		"Sample Show|TVmaze|series|https://www.tvmaze.com/shows/43/sample-show",
		"Sample Show Returns|IMDb|series|https://www.imdb.com/title/tt0000004/",
		"Sample Show: The Movie|IMDb|movie|https://www.imdb.com/title/tt0000005/",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("suggestions:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if fake.count("v3-cinemeta.strem.io") != 0 || fake.count("kitsu.io") != 0 || fake.count("en.wikipedia.org") != 0 {
		t.Errorf("a catalog beyond the three was asked: %v", fake.paths)
	}
	n := fake.total()
	if _, err := a.Suggest("sample show"); err != nil || fake.total() != n {
		t.Errorf("the same search asked again (%d new requests)", fake.total()-n)
	}
	if l, _ := a.Suggest("S"); l != nil {
		t.Errorf("one character is no search: %v", l)
	}

	// Privacy: a switched-off catalog is not asked; with every one off nothing is
	cfg2 := pinConfig()
	cfg2.DisabledSources = []string{"imdb", "tvmaze"}
	a2 := New(&cfg2, Options{Client: &http.Client{Transport: fake}})
	before := fake.count("v3.sg.media-imdb.com") + fake.count("api.tvmaze.com")
	_, _ = a2.Suggest("Sample Show Two")
	if fake.count("v3.sg.media-imdb.com")+fake.count("api.tvmaze.com") != before {
		t.Error("a switched-off catalog was asked")
	}
	cfg3 := pinConfig()
	cfg3.ShowArtwork = false
	if c := New(&cfg3, Options{}).SuggestCatalogs(); len(c) != 0 {
		t.Errorf("with lookups off no catalog may be asked: %v", c)
	}

	// each host's request gap applies
	cfg4 := pinConfig()
	cfg4.RequestGapMs = map[string]int{"api.tvmaze.com": 150}
	a4 := New(&cfg4, Options{Client: &http.Client{Transport: fake}})
	start := fake.count("api.tvmaze.com")
	_, _ = a4.Suggest("Sample Gap One")
	_, _ = a4.Suggest("Sample Gap Two")
	fake.mu.Lock()
	ts := fake.hosts["api.tvmaze.com"][start:]
	fake.mu.Unlock()
	if len(ts) != 2 || ts[1].Sub(ts[0]) < 120*time.Millisecond {
		t.Errorf("TVmaze's request gap was not kept: %v", ts)
	}
}

// The cover step asks the other catalogs once each, picks their match of the title, and gives at most five covers.
func TestCovers(t *testing.T) {
	fake := newFakeCatalogs(
		`^v3-cinemeta\.strem\.io/catalog/series/top/search=Sample%20Show`, `{"metas":[{"name":"Sample Show","releaseInfo":"2019","poster":"https://img.example/cm.jpg","imdb_id":"tt0000003"}]}`,
		`^api\.tvmaze\.com/search/shows`, `[{"show":{"id":42,"name":"Sample Show","premiered":"2019-01-01","url":"https://www.tvmaze.com/shows/42","image":{"medium":"https://img.example/tv.jpg"}}}]`,
		`^v3\.sg\.media-imdb\.com`, `{"d":[{"id":"tt0000003","l":"Sample Show","y":2019,"qid":"tvSeries","i":{"imageUrl":"https://img.example/imdb.jpg"}}]}`,
		`^kitsu\.io`, `{"data":[{"id":"1","attributes":{"subtype":"TV","canonicalTitle":"Sample Show","startDate":"2019-01-01","posterImage":{"large":"https://img.example/tv.jpg"}}}]}`,
	)
	cfg := pinConfig()
	a := New(&cfg, Options{Client: &http.Client{Transport: fake}})
	covers := a.Covers("Sample Show", 2019, "series", "IMDb")
	var got []string
	for _, c := range covers {
		got = append(got, c.Catalog+" "+c.URL)
	}
	if strings.Join(got, ", ") != "Cinemeta https://img.example/cm.jpg, TVmaze https://img.example/tv.jpg" {
		t.Errorf("covers: %v", got)
	}
	if fake.count("v3.sg.media-imdb.com") != 0 {
		t.Error("the pinned entry's own catalog was asked")
	}
	n := fake.total()
	a.Covers("Sample Show", 2019, "series", "IMDb")
	if fake.total() != n {
		t.Error("the same covers were asked again")
	}
}

// Offsets move the numbers read from the file (after the folder's count), and 0 and 0 change nothing.
func TestOffsets(t *testing.T) {
	cfg := core.DefaultConfig()
	m := core.ParseMedia("Sample Show S02E02.mkv", &cfg, nil)
	cfg.MatchFixes = []core.MatchFix{{Match: "Sample Show", EpisodeOffset: 12}}
	a := New(&cfg, Options{})
	got, shifted := a.Offset(m)
	if !shifted || got.Season != 2 || got.Episode != 14 || got.Label != "S02E14" || m.Episode != 2 {
		t.Errorf("offset: %+v (shifted %v); the original must stay %d", got, shifted, m.Episode)
	}

	withFolder := *m
	withFolder.BadgeEpisode = 5 // folderEpisodeNumbers counted this file as the fifth
	if got, _ := a.Offset(&withFolder); got.BadgeEpisode != 17 || got.Episode != 14 {
		t.Errorf("with the folder's count: %+v", got)
	}

	var warned []string
	cfg.MatchFixes = []core.MatchFix{{Match: "Sample Show", SeasonOffset: -5}}
	a = New(&cfg, Options{Log: func(_, msg string) { warned = append(warned, msg) }})
	if got, shifted := a.Offset(m); shifted || got != m || len(warned) != 1 {
		t.Errorf("below 1: %+v %v %q", got, shifted, warned)
	}

	cfg.MatchFixes = []core.MatchFix{{Match: "Sample Show", Search: "Sample Series"}}
	if got, shifted := New(&cfg, Options{}).Offset(m); shifted || got != m {
		t.Error("0 and 0 must be today's path")
	}
	film := core.ParseMedia("Sample Show (2020).mkv", &cfg, nil)
	cfg.MatchFixes = []core.MatchFix{{Match: "Sample Show", EpisodeOffset: 3}}
	if got, shifted := New(&cfg, Options{}).Offset(film); shifted || got != film {
		t.Error("a film has no episode to move")
	}
}

// "Why this title?" says a title was fixed, and how.
func TestExplainAFix(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.MatchFixes = []core.MatchFix{{Match: "Sample Show", Entry: "https://www.tvmaze.com/shows/42/sample-show"}}
	a := New(&cfg, Options{})
	show := core.ParseMedia("Sample Show - S01E02.mkv", &cfg, nil)
	hit := core.NewArt()
	hit.Type, hit.Name, hit.Source, hit.URL = "series", "Sample Show", "TVmaze", "https://www.imdb.com/title/tt0000003/"
	hit.IDs = &core.ArtIDs{Tvmaze: "42"}
	got := strings.Join(a.Explain(show, hit), "\n")
	if !strings.Contains(got, `Fixed by you in Fix a match (every file read as "Sample Show").`) || !strings.Contains(got, `Matched as you chose: TVmaze "Sample Show" (a show).`) {
		t.Errorf("pinned:\n%s", got)
	}
	hit.IDs = nil
	if got := strings.Join(a.Explain(show, hit), "\n"); !strings.Contains(got, `Matched by TVmaze: "Sample Show"`) {
		t.Errorf("a search after the pinned page failed is no pinned match:\n%s", got)
	}
}

package artwork

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// fakeAnimeCatalogs answers like TVmaze, Jikan (MyAnimeList) and Cinemeta for a long-running anime, "Sample Saga"
// (1999, 150 episodes), that shares its name with a 2023 live-action series which TVmaze lists first. Cinemeta files
// a special among the regular episodes of its first season, so counting through it lands one episode short.
type fakeAnimeCatalogs struct {
	mu      sync.Mutex
	urls    []string
	jikanUp bool
}

func (f *fakeAnimeCatalogs) asked(part string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.urls {
		if strings.Contains(u, part) {
			n++
		}
	}
	return n
}

func (f *fakeAnimeCatalogs) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.urls = append(f.urls, req.URL.String())
	jikanUp := f.jikanUp
	f.mu.Unlock()
	q, path := req.URL.Query().Get("q"), req.URL.Path
	body := "[]"
	switch {
	case req.URL.Host == "api.jikan.moe" && !jikanUp:
		return nil, &MockError{Name: "TimeoutError", Msg: "The operation was aborted due to timeout"}
	case req.URL.Host == "api.jikan.moe" && path == "/v4/anime" && q == "Sample Saga":
		body = `{"data":[{"mal_id":901,"title":"Sample Saga","type":"TV","year":2015,"titles":[{"type":"Default","title":"Sample Saga"}]},
			{"mal_id":21,"title":"Sample Saga","title_english":"Sample Saga","type":"TV","year":null,"aired":{"prop":{"from":{"year":1999}}}},
			{"mal_id":459,"title":"Sample Saga: The Movie","type":"Movie","year":2000}]}`
	case req.URL.Host == "api.jikan.moe" && path == "/v4/anime/21/episodes":
		page := req.URL.Query().Get("page")
		var eps []string
		for n := 1; n <= 100; n++ {
			num := (atoiOr0(page)-1)*100 + n
			if num > 150 {
				break
			}
			title := `"MAL episode ` + core.Itoa(num) + `"`
			if num == 150 {
				title = "null" // not aired yet
			}
			eps = append(eps, `{"mal_id":`+core.Itoa(num)+`,"title":`+title+`}`)
		}
		body = `{"pagination":{"last_visible_page":2},"data":[` + strings.Join(eps, ",") + `]}`
	case req.URL.Host == "api.tvmaze.com" && path == "/search/shows" && q == "Sample Saga":
		body = `[{"show":{"id":5001,"name":"Sample Saga","type":"Scripted","premiered":"2023-08-31"}},
			{"show":{"id":1505,"name":"Sample Saga","type":"Animation","premiered":"1999-10-20"}}]`
	case req.URL.Host == "api.tvmaze.com" && path == "/shows/5001/episodes":
		var eps []string
		for e := 1; e <= 8; e++ {
			eps = append(eps, `{"season":1,"number":`+core.Itoa(e)+`,"type":"regular","name":"Live-action episode `+core.Itoa(e)+`"}`)
		}
		body = "[" + strings.Join(eps, ",") + "]"
	case req.URL.Host == "api.tvmaze.com" && path == "/shows/1505/episodes":
		var eps []string
		for n := 1; n <= 150; n++ { // seasons by year, 50 a year
			eps = append(eps, `{"season":`+core.Itoa(1998+(n-1)/50+1)+`,"number":`+core.Itoa((n-1)%50+1)+`,"type":"regular","name":"TVmaze episode `+core.Itoa(n)+`"}`)
		}
		body = "[" + strings.Join(eps, ",") + "]"
	case req.URL.Host == "v3-cinemeta.strem.io" && strings.HasPrefix(path, "/catalog/series/top/search="):
		body = `{"metas":[{"imdb_id":"tt0000777","name":"Sample Saga","releaseInfo":"1999-"}]}`
	case req.URL.Host == "v3-cinemeta.strem.io" && path == "/meta/series/tt0000777.json":
		var v []string
		n := 0
		for s := 1; s <= 3; s++ {
			for e := 1; e <= 50; e++ {
				name := ""
				if s == 1 && e == 10 {
					name = "Special" // a special filed as a regular episode
				} else {
					n++
					name = "Cinemeta episode " + core.Itoa(n)
				}
				v = append(v, `{"season":`+core.Itoa(s)+`,"episode":`+core.Itoa(e)+`,"name":"`+name+`"}`)
			}
		}
		body = `{"meta":{"videos":[` + strings.Join(v, ",") + `]}}`
	}
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func atoiOr0(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// sagaHit is the cover match AniList gives for the anime: the whole show, first aired in 1999.
func sagaHit() *core.Art {
	return &core.Art{Name: "Sample Saga", Type: "series", Source: "AniList", Year: 1999, SeasonNo: core.NA,
		Alt: &core.ArtAlt{English: "Sample Saga", Romaji: "Sample Saga"}, IDs: &core.ArtIDs{}}
}

func sagaEpisode(n int) *core.Media {
	m := core.NewMedia()
	m.Title, m.IsEpisode, m.Episode, m.Anime = "Sample Saga", true, n, true
	return m
}

func animeResolver(t *testing.T, sources []string, jikanUp bool) (*Resolver, *fakeAnimeCatalogs) {
	cfg := core.DefaultConfig()
	if sources != nil {
		cfg.EpisodeSources = sources
	}
	fake := &fakeAnimeCatalogs{jikanUp: jikanUp}
	a := New(&cfg, Options{CacheFile: filepath.Join(t.TempDir(), "c.json"), Client: &http.Client{Transport: fake}})
	return newResolver(a), fake
}

// MyAnimeList numbers an anime's episodes 1, 2, 3... like the files do, so it is asked first for anime: episode 120
// of a long-running show is simply its number 120 (on its second page), from the entry that started in 1999.
func TestAnimeEpisodeTitleFromMyAnimeListFirst(t *testing.T) {
	r, fake := animeResolver(t, nil, true)
	ep := r.Find(sagaEpisode(120), sagaHit(), "Sample Saga", func(string, string) {})
	if ep == nil || ep.Title != "MAL episode 120" || ep.Source != "MyAnimeList" {
		t.Fatalf("got %+v (requests %v)", ep, fake.urls)
	}
	if fake.asked("/anime/21/episodes?page=2") != 1 || fake.asked("tvmaze") != 0 || fake.asked("cinemeta") != 0 {
		t.Errorf("MyAnimeList first, the right entry and page, nothing else: %v", fake.urls)
	}
	// the entry is looked up once
	_ = r.Find(sagaEpisode(121), sagaHit(), "Sample Saga", func(string, string) {})
	if fake.asked("/v4/anime?q=") != 1 {
		t.Errorf("searched again: %v", fake.urls)
	}
	// an episode without a title yet goes on to the next source
	if ep := r.Find(sagaEpisode(150), sagaHit(), "Sample Saga", func(string, string) {}); ep == nil || ep.Source != "TVmaze" {
		t.Errorf("untitled episode: %+v", ep)
	}
}

// With MyAnimeList down, TVmaze is asked, and among two shows of the same name it takes the one from the matched
// show's year (the file names none), not the live-action series it lists first. Its seasons are years, so a running
// number is counted through them.
func TestAnimeEpisodeTitleFromTheRightTVmazeShow(t *testing.T) {
	r, fake := animeResolver(t, nil, false)
	failed := false
	ep := r.Find(sagaEpisode(120), sagaHit(), "Sample Saga", func(string, string) { failed = true })
	if ep == nil || ep.Title != "TVmaze episode 120" || ep.Source != "TVmaze" || !ep.Absolute {
		t.Fatalf("got %+v (requests %v)", ep, fake.urls)
	}
	if !failed || fake.asked("/shows/5001/") != 0 {
		t.Errorf("MyAnimeList's failure is reported, and the live-action series is never read: %v", fake.urls)
	}
	// a year in the file name still decides
	m := sagaEpisode(3)
	m.Year = 2023
	if ep := r.Find(m, &core.Art{Name: "Sample Saga", Type: "series", Source: "IMDb", Year: 2023, SeasonNo: core.NA, IDs: &core.ArtIDs{}}, "Sample Saga", func(string, string) {}); ep == nil || ep.Title != "Live-action episode 3" {
		t.Errorf("the file's own year: %+v", ep)
	}
}

// Counting through Cinemeta's seasons is not trusted for anime (a special filed among the episodes shifts the count
// by one); for other shows it is used as before.
func TestCinemetaRunningNumbersNotUsedForAnime(t *testing.T) {
	r, fake := animeResolver(t, []string{"cinemeta"}, true)
	if ep := r.Find(sagaEpisode(120), sagaHit(), "Sample Saga", func(string, string) {}); ep != nil {
		t.Errorf("anime got a counted Cinemeta title: %+v", ep)
	}
	m := sagaEpisode(120)
	m.Anime = false
	hit := sagaHit()
	hit.Source, hit.Alt = "IMDb", nil
	if ep := r.Find(m, hit, "Sample Saga", func(string, string) {}); ep == nil || ep.Source != "Cinemeta" || !ep.Absolute {
		t.Errorf("a regular show keeps its counted title: %+v (requests %v)", ep, fake.urls)
	}
	// with a season in the file name nothing is counted, and anime keeps Cinemeta
	s := sagaEpisode(5)
	s.Season = 2
	if ep := r.Find(s, sagaHit(), "Sample Saga", func(string, string) {}); ep == nil || ep.Title != "Cinemeta episode 54" {
		t.Errorf("season 2 episode 5: %+v", ep)
	}
}

// MyAnimeList lists a later season as an entry of its own, so it is not asked for one; and never for other shows.
func TestMyAnimeListOnlyForAnimeWithoutLaterSeasons(t *testing.T) {
	r, fake := animeResolver(t, []string{"jikan"}, true)
	s := sagaEpisode(5)
	s.Season = 2
	if ep := r.Find(s, sagaHit(), "Sample Saga", func(string, string) {}); ep != nil || fake.asked("jikan") != 0 {
		t.Errorf("season 2: %+v, %v", ep, fake.urls)
	}
	m := sagaEpisode(5)
	m.Anime = false
	hit := sagaHit()
	hit.Source, hit.Alt = "TVmaze", nil
	if ep := r.Find(m, hit, "Sample Saga", func(string, string) {}); ep != nil || fake.asked("jikan") != 0 {
		t.Errorf("not anime: %+v, %v", ep, fake.urls)
	}
	// off on the Privacy tab: never asked
	r.cfg.DisabledSources = []string{"jikan"}
	if ep := r.Find(sagaEpisode(5), sagaHit(), "Sample Saga", func(string, string) {}); ep != nil || fake.asked("jikan") != 0 {
		t.Errorf("switched off: %+v, %v", ep, fake.urls)
	}
}

// A title found by counting through Cinemeta's seasons is dropped from the cache (and looked up again); others stay.
func TestCachedCinemetaRunningNumbersAreLookedUpAgain(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c.json")
	b, _ := json.Marshal(map[string]any{
		"ep|v3|s|sample saga|||||120": map[string]any{"title": "Cinemeta episode 119", "season": 3, "number": 20, "absolute": true, "source": "Cinemeta"},
		"ep|v3|s|sample show||1|||2":  map[string]any{"title": "Kept", "season": 1, "number": 2, "absolute": false, "source": "Cinemeta"},
		"ep|v3|s|sample saga|||||121": map[string]any{"title": "TVmaze episode 121", "season": 2001, "number": 21, "absolute": true, "source": "TVmaze"},
	})
	os.WriteFile(file, b, 0o644)
	cfg := core.DefaultConfig()
	a := New(&cfg, Options{CacheFile: file})
	if _, ok := a.disk["ep|v3|s|sample saga|||||120"]; ok {
		t.Error("a counted Cinemeta title was kept")
	}
	if e := a.disk["ep|v3|s|sample show||1|||2"]; e == nil || e.ep == nil || e.ep.Title != "Kept" {
		t.Error("a Cinemeta title by season and number is kept")
	}
	if e := a.disk["ep|v3|s|sample saga|||||121"]; e == nil || e.ep == nil {
		t.Error("other sources are kept")
	}
}

// When a source did not answer, the missing title is asked for again after two minutes instead of thirty.
func TestEpisodeAskedAgainSoonAfterAFailure(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.EpisodeSources, cfg.ArtworkSources, cfg.AnimeSources, cfg.ArtworkWaitMs = []string{"jikan"}, []string{"anilist"}, []string{"anilist"}, 5000
	fake := &fakeAnimeCatalogs{} // Jikan down; AniList is not answered either (nothing is matched)
	a := New(&cfg, Options{Client: &http.Client{Transport: fake}})
	m := sagaEpisode(120)
	a.Lookup(m)
	for i := 0; i < 300; i++ {
		a.mu.Lock()
		n := len(a.inflight)
		a.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, until := range a.misses {
		if strings.HasPrefix(k, "ep|") {
			if d := time.Until(until); d > 3*time.Minute {
				t.Errorf("asked again only after %v", d)
			}
			return
		}
	}
	t.Skip("no episode lookup was made without a cover match")
}

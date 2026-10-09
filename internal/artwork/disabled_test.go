package artwork

import (
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

func TestBaseHost(t *testing.T) {
	cases := map[string]string{
		"https://api.tvmaze.com":                "api.tvmaze.com",
		"https://kitsu.io/api/edge":             "kitsu.io",
		"https://graphql.anilist.co":            "graphql.anilist.co",
		"https://{lang}.wikipedia.org":          "wikipedia.org",
		"http://127.0.0.1:8080/x?y=1":           "127.0.0.1:8080",
		"https://bulbapedia.bulbagarden.net/w/": "bulbapedia.bulbagarden.net",
	}
	for in, want := range cases {
		if got := baseHost(in); got != want {
			t.Errorf("baseHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBlockedHostFilter(t *testing.T) {
	cfg := core.DefaultConfig()
	if blockedHostFilter(&cfg) != nil {
		t.Fatal("with nothing disabled there is no filter")
	}
	cfg.DisabledSources = []string{"wikipedia", "tvmaze", "TMDB"}
	f := blockedHostFilter(&cfg)
	for host, want := range map[string]bool{
		"en.wikipedia.org": true, "he.wikipedia.org": true, "wikipedia.org": true, "api.tvmaze.com": true,
		"api.themoviedb.org": true, "API.TVMAZE.COM": true,
		"v3-cinemeta.strem.io": false, "graphql.anilist.co": false, "notwikipedia.org": false, "tvmaze.com.evil.example": false,
	} {
		if got := f(host); got != want {
			t.Errorf("blocked(%q) = %v, want %v", host, got, want)
		}
	}
}

// hostRecorder answers every request with an empty JSON object and remembers which hosts were asked.
type hostRecorder struct {
	mu    sync.Mutex
	hosts map[string]int
}

func (h *hostRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	h.mu.Lock()
	if h.hosts == nil {
		h.hosts = map[string]int{}
	}
	h.hosts[strings.ToLower(req.URL.Host)]++
	h.mu.Unlock()
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
}

func (h *hostRecorder) contacted(host string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for k, v := range h.hosts {
		if k == host || strings.HasSuffix(k, "."+host) {
			n += v
		}
	}
	return n
}

func (h *hostRecorder) total() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, v := range h.hosts {
		n += v
	}
	return n
}

// lookupAll runs lookups for an ordinary episode, an anime-style file and a movie, and waits for them to settle.
func lookupAll(t *testing.T, cfg core.Config) *hostRecorder {
	t.Helper()
	rec := &hostRecorder{}
	aw := New(&cfg, Options{Client: &http.Client{Transport: rec}})
	for _, file := range []string{"Some.Show.S01E02.720p.mkv", "[Group] Some Anime - 05 [1080p].mkv", "Some.Movie.2001.1080p.mkv"} {
		aw.Lookup(core.ParseMedia(file, &cfg, nil))
		for i := 0; i < 300; i++ {
			aw.mu.Lock()
			n := len(aw.inflight)
			aw.mu.Unlock()
			if n == 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	return rec
}

func TestDisabledServicesAreNeverContacted(t *testing.T) {
	base := core.DefaultConfig()
	base.TmdbAPIKey = "test-key" // so TMDB is a candidate too
	base.EpisodeTitles = true

	control := lookupAll(t, base)
	if control.total() == 0 {
		t.Fatal("control run made no requests at all; the test would prove nothing")
	}
	used := map[string]bool{}
	for _, s := range core.Sources {
		if control.contacted(s.Host) > 0 {
			used[s.ID] = true
		}
	}
	if len(used) < 3 {
		t.Fatalf("control run only reached %v", used)
	}

	// each service switched off in turn: it is never asked, and the others still are
	for _, s := range core.Sources {
		cfg := base
		cfg.DisabledSources = []string{s.ID}
		rec := lookupAll(t, cfg)
		if n := rec.contacted(s.Host); n != 0 {
			t.Errorf("%s was switched off but received %d request(s)", s.ID, n)
		}
		if rec.total() == 0 {
			t.Errorf("switching off %s stopped every lookup", s.ID)
		}
	}

	// everything switched off: nothing leaves the program
	cfg := base
	for _, s := range core.Sources {
		cfg.DisabledSources = append(cfg.DisabledSources, s.ID)
	}
	if rec := lookupAll(t, cfg); rec.total() != 0 {
		t.Errorf("all services off, but %d request(s) were made: %v", rec.total(), rec.hosts)
	}
}

func TestNetBlocksAtTheLowestLevel(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.DisabledSources = []string{"cinemeta"}
	rec := &hostRecorder{}
	aw := New(&cfg, Options{Client: &http.Client{Transport: rec}})
	// even a direct call (as the rich-info and runtime lookups make) cannot reach the service
	if _, err := aw.net.getJSON(cfg.CinemetaBase+"/meta/movie/tt0000001.json", reqInit{}); err == nil {
		t.Fatal("a request to a disabled service must fail")
	} else if !strings.Contains(err.Error(), "privacy") {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.total() != 0 {
		t.Fatal("the blocked request reached the network")
	}
	if _, err := aw.net.getJSON(cfg.TvmazeBase+"/search/shows?q=x", reqInit{}); err != nil {
		t.Fatalf("a service that is on must still work: %v", err)
	}
}

func TestClearCache(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c.json")
	cfg := core.DefaultConfig()
	aw := New(&cfg, Options{CacheFile: file, Client: &http.Client{Transport: &hostRecorder{}}})
	art := core.NewArt()
	art.Name = "Something"
	aw.mu.Lock()
	aw.disk["k"] = &diskEntry{art: art}
	aw.misses["m"] = time.Now().Add(time.Hour)
	aw.runtimes["tt1"] = 90
	aw.mu.Unlock()
	aw.save()
	if _, err := aw.net.getJSON(cfg.TvmazeBase+"/search/shows?q=x", reqInit{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the cache file should exist before clearing: %v", err)
	}
	aw.net.mu.Lock()
	warm := len(aw.net.cache)
	aw.net.mu.Unlock()
	if warm == 0 {
		t.Fatal("the network cache should be warm before clearing")
	}

	aw.ClearCache()

	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("the cache file should be gone, got %v", err)
	}
	aw.mu.Lock()
	d, m, r := len(aw.disk), len(aw.misses), len(aw.runtimes)
	aw.mu.Unlock()
	aw.net.mu.Lock()
	n := len(aw.net.cache)
	aw.net.mu.Unlock()
	if d != 0 || m != 0 || r != 0 || n != 0 {
		t.Errorf("everything should be forgotten: disk=%d misses=%d runtimes=%d net=%d", d, m, r, n)
	}
	// a finder started afterwards has nothing to load
	again := New(&cfg, Options{CacheFile: file})
	if len(again.disk) != 0 {
		t.Errorf("a new finder loaded %d entries from a cleared cache", len(again.disk))
	}
}
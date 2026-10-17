package artwork

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// fakeSpecials answers like Cinemeta for "Sample Show" (tt0000555): two specials as season 0, then a season of six.
type fakeSpecials struct{}

func (fakeSpecials) RoundTrip(req *http.Request) (*http.Response, error) {
	body := "[]"
	switch {
	case req.URL.Host == "v3-cinemeta.strem.io" && req.URL.Path == "/catalog/series/top/search=Sample Show.json":
		body = `{"metas":[{"imdb_id":"tt0000555","name":"Sample Show","releaseInfo":"2010-","poster":"https://images.example/show.jpg"}]}`
	case req.URL.Host == "v3-cinemeta.strem.io" && req.URL.Path == "/catalog/series/top/search=Sample Show OVA 2.json":
		body = `{"metas":[{"imdb_id":"tt0000777","name":"Sample Show OVA 2","releaseInfo":"2012","poster":"https://images.example/ova.jpg"}]}` // listed on its own
	case req.URL.Host == "v3-cinemeta.strem.io" && req.URL.Path == "/meta/series/tt0000555.json":
		v := []string{`{"season":0,"episode":1,"name":"Sample Special One"}`, `{"season":0,"episode":2,"name":"Sample Special Two"}`}
		for e := 1; e <= 6; e++ {
			v = append(v, `{"season":1,"episode":`+strconv.Itoa(e)+`,"name":"Regular episode `+strconv.Itoa(e)+`"}`)
		}
		body = `{"meta":{"videos":[` + strings.Join(v, ",") + `]}}`
	}
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// A special (S00E02) gets its name from Cinemeta's season 0 without a TMDB key; a regular episode, and a file without
// a season that is counted through the show, never land on a special.
func TestSpecialsFromCinemeta(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.EpisodeSources = []string{"cinemeta"}
	a := New(&cfg, Options{CacheFile: filepath.Join(t.TempDir(), "c.json"), Client: &http.Client{Transport: fakeSpecials{}}})
	r := newResolver(a)
	hit := &core.Art{Name: "Sample Show", Type: "series", Source: "IMDb", Year: 2010, SeasonNo: core.NA, IDs: &core.ArtIDs{}}
	find := func(file string) *core.ArtEpisode {
		t.Helper()
		m := core.ParseMedia(file, &cfg, nil)
		return r.Find(m, hit, "Sample Show", func(string, string) {})
	}

	special := core.ParseMedia("Sample Show - S00E02.mkv", &cfg, nil)
	if special.Season != 0 || special.Episode != 2 {
		t.Fatalf("S00E02 parsed as season %d, episode %d", special.Season, special.Episode)
	}
	if ep := find("Sample Show - S00E02.mkv"); ep == nil || ep.Title != "Sample Special Two" || ep.Season != 0 || ep.Number != 2 {
		t.Errorf("special: %+v", ep)
	}
	if ep := find("Sample Show - S01E02.mkv"); ep == nil || ep.Title != "Regular episode 2" {
		t.Errorf("regular episode: %+v", ep)
	}
	if ep := find("Sample Show - 03.mkv"); ep == nil || ep.Title != "Regular episode 3" {
		t.Errorf("counted episode: %+v", ep)
	}
}

// A special named in words ("Sample Show - Special 02", "OVA 2", "SP2") that no catalog lists on its own is found as
// the show's special: the show's cover, S00E02 and the special's name. A title that is listed on its own keeps it.
func TestNamedSpecialFallback(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.ArtworkSources, cfg.EpisodeSources, cfg.AnimeSources = []string{"cinemeta"}, []string{"cinemeta"}, []string{"cinemeta"}
	cfg.RichInfo, cfg.ArtworkWaitMs = false, 5000
	for _, file := range []string{"Sample Show - Special 02.mkv", "[Group] Sample Show - SP2 [1080p].mkv", "Sample.Show.OAD.02.mkv", "Sample Show S01 OVA 2.mkv", "Sample Show Season 1 - Special 2.mkv"} {
		a := New(&cfg, Options{CacheFile: filepath.Join(t.TempDir(), "c.json"), Client: &http.Client{Transport: fakeSpecials{}}})
		m := core.ParseMedia(file, &cfg, nil)
		if m.IsEpisode {
			t.Fatalf("%s: the parser now reads it as an episode; the frozen parse data keeps it a title", file)
		}
		art := a.Lookup(m)
		if art == nil || !art.Special || art.Name != "Sample Show" || art.Episode == nil || art.Episode.Title != "Sample Special Two" {
			t.Errorf("%s: art %+v", file, art)
			continue
		}
		act := core.BuildActivity(core.NewInfo(file, "", "", 2, 1000, 60000, 1), &cfg, 0, m, art)
		if b, _ := json.Marshal(act); !strings.Contains(string(b), "Season 0, Episode 2") || !strings.Contains(string(b), "Sample Special Two") {
			t.Errorf("%s: activity %s", file, b)
		}
		// The flag survives the cache file.
		var back core.Art
		if b, _ := json.Marshal(art); json.Unmarshal(b, &back) != nil || !back.Special {
			t.Errorf("%s: Special lost in JSON: %s", file, b)
		}
	}

	a := New(&cfg, Options{CacheFile: filepath.Join(t.TempDir(), "c.json"), Client: &http.Client{Transport: fakeSpecials{}}})
	if art := a.Lookup(core.ParseMedia("Sample Show OVA 2.mkv", &cfg, nil)); art == nil || art.Special || art.Name != "Sample Show OVA 2" {
		t.Errorf("a title listed on its own: %+v", art)
	}
	for _, title := range []string{"Sample Movie", "Special 2", "Sample Show Specialist 2", "Sample ShowOVA2"} {
		if sp := core.NamedSpecial(&core.Media{Title: title}); sp != nil {
			t.Errorf("%q read as a special: %+v", title, sp)
		}
	}
}

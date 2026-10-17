package artwork

import (
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
	case req.URL.Host == "v3-cinemeta.strem.io" && strings.HasPrefix(req.URL.Path, "/catalog/series/top/search="):
		body = `{"metas":[{"imdb_id":"tt0000555","name":"Sample Show","releaseInfo":"2010-"}]}`
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

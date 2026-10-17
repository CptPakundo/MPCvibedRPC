package artwork

import (
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

func TestExplain(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.ArtworkAliases = map[string]string{"Sample Movie": "Sample Film"}
	a := New(&cfg, Options{})
	movie := core.ParseMedia("Sample Movie (2020).mkv", &cfg, nil)
	show := core.ParseMedia("Sample Show - S01E02.mkv", &cfg, nil)
	join := func(l []string) string { return strings.Join(l, "\n") }

	hit := core.NewArt()
	hit.Type, hit.Name, hit.Source, hit.Year = "movie", "Sample Film", "Cinemeta", 2020
	hit.Info = &core.ArtInfo{Genres: []string{"Drama"}}
	got := join(a.Explain(movie, hit))
	for _, want := range []string{`Searched as "Sample Film"`, `Matched by Cinemeta: "Sample Film" (2020, a movie).`, "Genres and rating from Cinemeta."} {
		if !strings.Contains(got, want) {
			t.Errorf("movie: missing %q in\n%s", want, got)
		}
	}

	sh := core.NewArt()
	sh.Type, sh.Name, sh.Source, sh.ViaRank = "series", "Sample Show", "TVmaze", true
	sh.Episode = &core.ArtEpisode{Title: "Pilot", Source: "TVmaze", Via: "Sample Show"}
	got = join(a.Explain(show, sh))
	if !strings.Contains(got, `Matched by TVmaze: "Sample Show" (a show), the best known of that name`) ||
		!strings.Contains(got, "Episode title from TVmaze.") || strings.Contains(got, "looked up as") {
		t.Errorf("show:\n%s", got)
	}
	sh.Episode = nil
	if got = join(a.Explain(show, sh)); !strings.Contains(got, "No episode title found") {
		t.Errorf("show without an episode title:\n%s", got)
	}

	ov := core.NewArt()
	ov.Source = "override"
	if got = join(a.Explain(show, ov)); !strings.Contains(got, "Cover pinned by you") {
		t.Errorf("override:\n%s", got)
	}

	// nothing found: still looking, a slow service, or no match with the closest results
	if got = join(a.Explain(show, nil)); got != "Still being looked up." {
		t.Errorf("before any lookup: %q", got)
	}
	key := a.newLookupJob(show).key
	a.mu.Lock()
	a.softMiss(key)
	a.mu.Unlock()
	if got = join(a.Explain(show, nil)); !strings.Contains(got, "slow or offline") {
		t.Errorf("soft miss: %q", got)
	}
	a.mu.Lock()
	a.misses[key] = time.Now().Add(30 * time.Minute)
	delete(a.soft, key)
	a.closest[normTitle(show.Title)] = []string{`TVmaze: "Sample Shows"`}
	a.mu.Unlock()
	if got = join(a.Explain(show, nil)); !strings.Contains(got, "No catalog matched.") || !strings.Contains(got, `Closest results: TVmaze: "Sample Shows".`) {
		t.Errorf("no match: %q", got)
	}

	cfg.ShowArtwork = false
	if got = join(a.Explain(show, nil)); !strings.Contains(got, "Cover lookups are off") {
		t.Errorf("lookups off: %q", got)
	}
	var none *Artwork
	if none.Explain(show, nil) != nil {
		t.Error("a nil finder should explain nothing")
	}
}

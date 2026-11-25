package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPRP/internal/artwork"
	"github.com/CptPakundo/MPRP/internal/core"
)

// What the Fix this match dialog starts from names the file but never its folder; a fix's offset moves the numbers
// the card shows and the catalogs are asked; a fix for the file wins over one for its title.
func TestBuildWithAFix(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.ArtworkSources, cfg.EpisodeSources, cfg.EpisodeTitles = []string{}, []string{}, false // nothing is asked online
	cfg.MatchFixes = []core.MatchFix{
		{Match: "Sample Show", Cover: "https://img.example/show.jpg", EpisodeOffset: 12},
		{Match: "Sample.Show.S01E03.mkv", File: true, Cover: "https://img.example/three.jpg"},
	}
	art := artwork.New(&cfg, artwork.Options{})
	info := &core.Info{File: "Sample.Show.S01E02.mkv", Dir: []string{"Season 1"}, FullDir: "/videos/private/Season 1", State: 2, Duration: 1500000, Rate: 1, Player: "MPC-HC"}
	b := buildActivity(info, &cfg, art, nil, time.Now())
	if b.match == nil || b.match.File != "Sample.Show.S01E02.mkv" || b.match.Title != "Sample Show" || b.match.Season != 1 || b.match.Episode != 2 || !b.match.Fixed || b.match.Now != "your image" {
		t.Fatalf("match: %+v", b.match)
	}
	if b.activity.Assets == nil || b.activity.Assets.LargeText != "Season 1, Episode 14" || b.activity.Assets.LargeImage != "https://img.example/show.jpg" {
		t.Errorf("the offset should move the badge: %+v", b.activity.Assets)
	}
	if why := strings.Join(b.why, "\n"); !strings.Contains(why, "looked up as S01E14") || strings.Contains(why, "private") {
		t.Errorf("why:\n%s", why)
	}

	info.File = "Sample.Show.S01E03.mkv"
	b = buildActivity(info, &cfg, art, nil, time.Now())
	if b.activity.Assets.LargeImage != "https://img.example/three.jpg" || b.activity.Assets.LargeText != "Season 1, Episode 3" {
		t.Errorf("the file fix should win (and has no offset): %+v", b.activity.Assets)
	}

	info.File = "sample song.mp3"
	if b = buildActivity(info, &cfg, art, nil, time.Now()); b.match != nil {
		t.Errorf("a song has nothing to fix: %+v", b.match)
	}
	cfg.HideTitle = true
	info.File = "Sample.Show.S01E02.mkv"
	if b = buildActivity(info, &cfg, art, nil, time.Now()); b.match != nil {
		t.Errorf("a hidden title has nothing to fix: %+v", b.match)
	}
}

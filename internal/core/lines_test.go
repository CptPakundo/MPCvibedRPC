package core

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// linesTable is testdata/lines.json, which the page's test reads too (internal/assets): the page's sample and the card
// must fill a line the same way.
type linesTable struct {
	Values map[string]map[string]string `json:"values"`
	Cases  []struct {
		Values, Tpl, Want string
	} `json:"cases"`
}

func TestExpandFollowsTheSharedTable(t *testing.T) {
	b, err := os.ReadFile("testdata/lines.json")
	if err != nil {
		t.Fatal(err)
	}
	var tab linesTable
	if err := json.Unmarshal(b, &tab); err != nil {
		t.Fatal(err)
	}
	if len(tab.Cases) < 10 {
		t.Fatalf("only %d cases", len(tab.Cases))
	}
	for _, c := range tab.Cases {
		vals, ok := tab.Values[c.Values]
		if !ok {
			t.Fatalf("no values %q", c.Values)
		}
		if got := Expand(c.Tpl, vals); got != c.Want {
			t.Errorf("Expand(%q) = %q, want %q", c.Tpl, got, c.Want)
		}
		if u16len(Expand(c.Tpl, vals)) > 128 {
			t.Errorf("Expand(%q) is longer than 128", c.Tpl)
		}
	}
}

// sample: an episode with a catalog title and info, and a film, as the design's table has them.
func sampleEpisode() (*Info, *Art) {
	info := &Info{File: "Sample.Show.S01E02.mkv", State: 2, Position: 60000, Duration: 2700000, Rate: 1}
	art := NewArt()
	art.Type, art.Name, art.Source, art.Poster = "series", "Sample Show", "TVmaze", "https://img.example/show.jpg"
	art.Info = &ArtInfo{Genres: []string{"Drama", "Comedy"}, Rating: 8.3}
	art.Episode = &ArtEpisode{Title: "Sample Episode", Season: 1, Number: 2, Source: "TVmaze"}
	return info, art
}

func sampleMovie() (*Info, *Art) {
	info := &Info{File: "Sample.Movie.2020.mkv", State: 2, Position: 60000, Duration: 6730000, Rate: 1}
	art := NewArt()
	art.Type, art.Name, art.Source, art.Poster, art.Year = "movie", "Sample Movie", "Cinemeta", "https://img.example/movie.jpg", 2020
	art.Info = &ArtInfo{Genres: []string{"Drama", "Comedy"}, Rating: 7.1, Director: "Sample Name"}
	return info, art
}

func withLines(mode string, custom map[string]string) Config {
	cfg := DefaultConfig()
	cfg.AppName = "VLC media player"
	cfg.TextLines = mode
	if custom != nil {
		cfg.CustomLines = custom
	}
	return cfg
}

func TestTextLinesPresets(t *testing.T) {
	cases := []struct {
		mode                          string
		epDetails, epState            string
		mvDetails, mvState, mvLargeTx string
	}{
		{"standard", "Sample Episode", "Drama, Comedy · ★ 8.3", "2020 · Drama, Comedy", "★ 7.1 · Dir. Sample Name", "Sample Movie (2020)"},
		{"short", "S01E02 · Sample Episode", "", "2020", "Dir. Sample Name", "Sample Movie (2020)"},
		{"detailed", "S01E02 · Sample Episode", "Drama, Comedy · ★ 8.3 · on VLC media player", "2020 · Drama, Comedy · 1:52:10", "★ 7.1 · Dir. Sample Name · on VLC media player", "Sample Movie (2020)"},
		{"something new", "Sample Episode", "Drama, Comedy · ★ 8.3", "2020 · Drama, Comedy", "★ 7.1 · Dir. Sample Name", "Sample Movie (2020)"}, // read as Standard
	}
	for _, c := range cases {
		cfg := withLines(c.mode, nil)
		info, art := sampleEpisode()
		a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
		if a.Name != "Sample Show" || a.Details != c.epDetails || a.State != c.epState {
			t.Errorf("%s episode: name %q, details %q, state %q", c.mode, a.Name, a.Details, a.State)
		}
		if a.Assets == nil || a.Assets.LargeText != "Season 1, Episode 2" {
			t.Errorf("%s episode: the cover text must stay the badge's: %+v", c.mode, a.Assets)
		}
		info, art = sampleMovie()
		a = BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
		if a.Name != "Sample Movie" || a.Details != c.mvDetails || a.State != c.mvState || a.Assets.LargeText != c.mvLargeTx {
			t.Errorf("%s film: name %q, details %q, state %q, cover %q", c.mode, a.Name, a.Details, a.State, a.Assets.LargeText)
		}
	}
}

func TestTextLinesCustom(t *testing.T) {
	custom := map[string]string{
		"episodeDetails": "{title} {code} · {episodeTitle}",
		"episodeState":   "[{seasonName}] · {genres} · [on {player}]",
		"movieDetails":   "{director} · {titel}",
		"movieCover":     "{title} · {length}",
	}
	cfg := withLines("custom", custom)
	info, art := sampleEpisode()
	a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
	if a.Details != "Sample Show S01E02 · Sample Episode" || a.State != "Drama, Comedy · on VLC media player" {
		t.Errorf("episode: %q / %q", a.Details, a.State)
	}
	if a.Timestamps == nil || a.Timestamps.End == 0 {
		t.Errorf("the progress bar must stay: %+v", a.Timestamps)
	}
	info, art = sampleMovie()
	a = BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
	if a.Details != "Dir. Sample Name · {titel}" || a.State != "" || a.Assets.LargeText != "Sample Movie · 1:52:10" {
		t.Errorf("film: %q / %q / %q (a missing line is an empty line)", a.Details, a.State, a.Assets.LargeText)
	}

	// an empty cover text is today's; a film without a year drops the brackets
	cfg.CustomLines = map[string]string{"movieDetails": "{year}", "movieCover": "  "}
	info, art = sampleMovie()
	art.Year = NA
	info.File = "Sample.Movie.mkv"
	a = BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
	if a.Details != "" || a.Assets.LargeText != "Sample Movie" {
		t.Errorf("film without a year: details %q, cover %q", a.Details, a.Assets.LargeText)
	}

	// a film without catalog data: no cover of its own, so no cover text of the user's (the logo's text stays)
	a = BuildActivity(&Info{File: "Sample Movie (2020).mkv", State: 2, Duration: 6000000, Rate: 1}, &cfg, 1_700_000_000_000, nil, nil)
	if a.Details != "2020" || a.Assets == nil || a.Assets.LargeText != cfg.AppName {
		t.Errorf("film without catalog data: %q, %+v", a.Details, a.Assets)
	}
}

// Text lines never reach the pause line, the stream form, a hidden title, a status without the title as its name,
// or a song (the engine builds songs with "standard").
func TestTextLinesLeaveTheFixedPartsAlone(t *testing.T) {
	custom := map[string]string{"episodeDetails": "X {title}", "episodeState": "Y {code}", "movieDetails": "X", "movieState": "Y", "movieCover": "Z"}
	cfg := withLines("custom", custom)
	std := withLines("standard", nil)

	info, art := sampleEpisode()
	info.State = 1
	a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
	if a.Details != "X Sample Show" || !strings.HasPrefix(a.State, "⏸ ") {
		t.Errorf("paused: %q / %q, want the custom first line and the pause line", a.Details, a.State)
	}

	same := func(what string) {
		t.Helper()
		info, art := sampleEpisode()
		if got, want := BuildActivity(info, &cfg, 1_700_000_000_000, nil, art), BuildActivity(info, &std, 1_700_000_000_000, nil, art); mustJSON(got) != mustJSON(want) {
			t.Errorf("%s changed:\n%s\n%s", what, mustJSON(got), mustJSON(want))
		}
	}
	cfg.StreamTitle, std.StreamTitle = "always", "always"
	same("the stream form")
	cfg.StreamTitle, std.StreamTitle = "off", "off"

	cfg.TitleAsName, std.TitleAsName = false, false
	same("without the title as the name, the status")
	cfg.TitleAsName, std.TitleAsName = true, true

	cfg.ActivityType, std.ActivityType = "playing", "playing" // the Playing style has no title in the name either
	same("the Playing style")
	cfg.ActivityType, std.ActivityType = "watching", "watching"

	cfg.HideTitle, std.HideTitle = true, true
	info, _ = sampleEpisode()
	if got, want := HiddenActivity(info, &cfg, 1_700_000_000_000), HiddenActivity(info, &std, 1_700_000_000_000); mustJSON(got) != mustJSON(want) {
		t.Errorf("a hidden title changed:\n%s\n%s", mustJSON(got), mustJSON(want))
	}
}

// A special's own words fill {code}; Standard's fallbacks are not template rules.
func TestTextLinesSpecial(t *testing.T) {
	cfg := withLines("short", nil)
	info := &Info{File: "Sample Show - Special 03.mkv", State: 2, Duration: 1500000, Rate: 1}
	art := NewArt()
	art.Type, art.Name, art.Source, art.Special = "series", "Sample Show", "TVmaze", true
	a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
	if a == nil || a.Details != "Special 3" || a.State != "" {
		t.Errorf("special: %+v", a)
	}
}

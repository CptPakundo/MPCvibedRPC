package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// Discord labels a screen share with the streamer's Playing activity (its name) and with no other kind. So the
// stream title sends "Playing <title>", whichever style is chosen, and keeps the episode in the first line.
func TestStreamTitleIsAPlayingActivityNamedAfterTheTitle(t *testing.T) {
	for _, style := range []string{"watching", "playing"} {
		cfg := DefaultConfig()
		cfg.ActivityType, cfg.StreamTitle = style, "always"
		info := &Info{File: "Sample.Show.S01E02.mkv", State: 2, Position: 600000, Duration: 2700000, Rate: 1}
		a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, nil)
		if a == nil {
			t.Fatalf("%s: nil activity", style)
		}
		if a.Type != 0 || a.StatusDisplayType != nil {
			t.Errorf("%s: type=%d display=%v, want a plain Playing activity", style, a.Type, a.StatusDisplayType)
		}
		if a.Name != "Sample Show" {
			t.Errorf("%s: name=%q, want the title", style, a.Name)
		}
		if a.Details != "S01E02" {
			t.Errorf("%s: details=%q, want the episode", style, a.Details)
		}
		if a.Timestamps == nil || a.Timestamps.Start != 1_700_000_000_000-600000 || a.Timestamps.End != 0 {
			t.Errorf("%s: timestamps=%+v, want a timer from the start", style, a.Timestamps)
		}
		b, _ := json.Marshal(a)
		if strings.Contains(string(b), `"type"`) {
			t.Errorf("%s: the type is left out for Playing, as in the Playing style: %s", style, b)
		}

		cfg.ShowRemainingTime = true
		if a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, nil); a.Timestamps == nil || a.Timestamps.Start != 0 || a.Timestamps.End == 0 {
			t.Errorf("%s: with the remaining time, timestamps=%+v", style, a.Timestamps)
		}

		info.State = 1
		if a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, nil); a == nil || a.Name != "Sample Show" || a.Type != 0 || a.Timestamps != nil {
			t.Errorf("%s: paused: %+v", style, a)
		}
	}
}

func TestStreamTitleOfAFilm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StreamTitle = "always"
	info := &Info{File: "Sample Movie (2020).mkv", State: 2, Position: 1000, Duration: 6000000, Rate: 1}
	a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, nil)
	if a == nil || a.Type != 0 || a.Name != "Sample Movie" || a.Details != "2020" {
		t.Errorf("got %+v", a)
	}
}

// The default is "auto": the stream form only while Discord shares the screen, which the engine finds out. On its own
// "auto" builds the usual status, and so does "off": the title is the name only in the Watching style.
func TestStreamTitleIsAutomaticByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.StreamTitle != "auto" {
		t.Fatalf("the default is %q, want auto: the status changes only while a stream is on", cfg.StreamTitle)
	}
	for _, mode := range []string{"auto", "off", "", "something else"} {
		cfg.StreamTitle = mode
		if a := BuildActivity(&Info{File: "Sample.Show.S01E02.mkv", State: 2, Position: 600000, Duration: 2700000, Rate: 1}, &cfg, 1_700_000_000_000, nil, nil); a.Type != 3 {
			t.Errorf("%q builds type %d, want the Watching status", mode, a.Type)
		}
	}
	cfg.StreamTitle = "auto"
	info := &Info{File: "Sample.Show.S01E02.mkv", State: 2, Position: 600000, Duration: 2700000, Rate: 1}
	if a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, nil); a.Type != 3 || a.Name != "Sample Show" {
		t.Errorf("watching: type=%d name=%q", a.Type, a.Name)
	}
	cfg.ActivityType = "playing"
	if a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, nil); a.Type != 0 || a.Name != "" {
		t.Errorf("playing: type=%d name=%q, want Discord's own name for the application", a.Type, a.Name)
	}
}

// A hidden title is hidden on a stream too: the status keeps its style and names nothing.
func TestHiddenActivityIgnoresTheStreamTitle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StreamTitle = "always"
	info := &Info{File: "Very.Private.Show.S03E04.mkv", State: 2, Position: 600000, Duration: 2700000, Rate: 1}
	a := HiddenActivity(info, &cfg, 1_700_000_000_000)
	if a == nil || a.Type != 3 || a.Name != "" {
		t.Fatalf("got %+v, want the Watching status without a name", a)
	}
	b, _ := json.Marshal(a)
	if js := strings.ToLower(string(b)); strings.Contains(js, "private") || strings.Contains(js, "s03e04") {
		t.Errorf("the hidden status mentions the file: %s", b)
	}
}

// Discord gives a Playing status only a running clock, so the stream form says where the video is in its second
// line: a text bar, the length and a speed other than 1x. The rating and director move to the first line.
func TestStreamFormShowsWhereTheVideoIs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StreamTitle = "always"
	art := &Art{Type: "movie", Name: "Sample Movie", Year: 2020, SeasonNo: NA, EpInEntry: NA,
		Info: &ArtInfo{Genres: []string{"Drama"}, Rating: 7.2, Director: "Sample Director"}}
	info := &Info{File: "Sample Movie (2020).mkv", State: 2, Position: 3000000, Duration: 6000000, Rate: 1}
	media := ParseMedia(info.File, &cfg, nil)
	a := BuildActivity(info, &cfg, 1_700_000_000_000, media, art)
	if a.State != "▰▰▰▰▰▰▱▱▱▱▱▱ of 1:40:00" {
		t.Errorf("state = %q", a.State)
	}
	if !strings.HasPrefix(a.Details, "2020 · Drama") || !strings.Contains(a.Details, "7.2") || !strings.Contains(a.Details, "Sample Director") {
		t.Errorf("details = %q, want year, genres, rating and director", a.Details)
	}
	if got := StreamProgress(&Info{State: 2, Position: 0, Duration: 60000, Rate: 1.5}, &cfg); got != "▱▱▱▱▱▱▱▱▱▱▱▱ of 1:00 · 1.5×" {
		t.Errorf("at 1.5x: %q", got)
	}
	info.State = 1
	if got := StreamProgress(info, &cfg); got != "" {
		t.Errorf("paused: %q, want nothing (the pause line has its own bar)", got)
	}
	info.State = 2
	cfg.TextProgressBar = false
	if a := BuildActivity(info, &cfg, 1_700_000_000_000, media, art); strings.Contains(a.State, "▰") {
		t.Errorf("textProgressBar off: state = %q", a.State)
	}

	cfg = DefaultConfig()
	cfg.StreamTitle = "always"
	ep := &Info{File: "Sample.Show.S01E02.mkv", State: 2, Position: 1350000, Duration: 2700000, Rate: 1}
	if a := BuildActivity(ep, &cfg, 1_700_000_000_000, nil, nil); !strings.HasPrefix(a.State, "▰▰▰▰▰▰▱▱▱▱▱▱ of 45:00") {
		t.Errorf("an episode: state = %q", a.State)
	}
	cfg.StreamTitle = "off"
	if a := BuildActivity(ep, &cfg, 1_700_000_000_000, nil, nil); strings.Contains(a.State, "▰") {
		t.Errorf("the Watching status got the text bar: %q", a.State)
	}
}

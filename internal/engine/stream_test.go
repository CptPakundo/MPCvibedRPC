package engine

import (
	"testing"
	"time"

	"github.com/CptPakundo/MPRP/internal/core"
)

// The stream title is for videos: a song stays "Listening to" with it on.
func TestStreamTitleLeavesMusicAlone(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.StreamTitle = "always"
	song := &core.Info{File: "Sample Song.mp3", State: 2, Position: 1000, Duration: 200000, Rate: 1}
	if b := buildActivity(song, &cfg, nil, nil, time.Now()); b.activity == nil || b.activity.Type != 2 {
		t.Errorf("a song: %+v, want a Listening activity", b.activity)
	}
	video := &core.Info{File: "Sample Movie (2020).mkv", State: 2, Position: 1000, Duration: 6000000, Rate: 1}
	if b := buildActivity(video, &cfg, nil, nil, time.Now()); b.activity == nil || b.activity.Type != 0 || b.activity.Name != "Sample Movie" {
		t.Errorf("a video: %+v, want Playing with the title as its name", b.activity)
	}
}

// The stream form is sent again when its text bar fills another cell (streamBar against barShown); a song, a hidden
// title and the usual status have no such bar, so they are not sent more often.
func TestStreamBarOnlyForTheStreamForm(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.StreamTitle = "always"
	video := &core.Info{File: "Sample Movie (2020).mkv", State: 2, Position: 0, Duration: 1200000, Rate: 1}
	start := streamBar(video, &cfg)
	if start == "" {
		t.Fatal("no bar for a video in the stream form")
	}
	video.Position = 40000 // under half a cell (100 s at 12 cells over 20 minutes): no new update
	if streamBar(video, &cfg) != start {
		t.Error("the bar changed within its first cell")
	}
	video.Position = 150000
	if streamBar(video, &cfg) == start {
		t.Error("the bar did not change after a cell filled")
	}
	song := &core.Info{File: "Sample Song.mp3", State: 2, Duration: 200000, Rate: 1}
	hidden := cfg
	hidden.HideTitle = true
	off := cfg
	off.StreamTitle = "off"
	if streamBar(song, &cfg) != "" || streamBar(video, &hidden) != "" || streamBar(video, &off) != "" {
		t.Error("a song, a hidden title or the usual status has a stream bar")
	}
}

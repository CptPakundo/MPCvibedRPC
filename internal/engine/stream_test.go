package engine

import (
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
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

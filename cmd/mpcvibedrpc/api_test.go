package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/assets"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
)

// Every endpoint the settings page calls exists.
func TestEveryWindowCallHasAHandler(t *testing.T) {
	h := (&app{}).handlers()
	calls := regexp.MustCompile(`(?:api|act)\('([a-z-]+)'`).FindAllStringSubmatch(string(assets.UI), -1)
	if len(calls) < 20 {
		t.Fatalf("found only %d calls in the page", len(calls))
	}
	for _, c := range calls {
		if _, ok := h[c[1]]; !ok {
			t.Errorf("the page calls /api/%s, which has no handler", c[1])
		}
	}
}

func TestTrayTooltip(t *testing.T) {
	title := "Sample Show S01E02"
	long := strings.Repeat("x", 80)
	for _, c := range []struct {
		s    engine.Status
		want string
	}{
		{engine.Status{}, "MPCvibedRPC - Not showing on Discord"},
		{engine.Status{Running: true, NowPlaying: &title}, "MPCvibedRPC - Sample Show S01E02"},
		{engine.Status{Running: true, Discord: "standby"}, "MPCvibedRPC - Ready"},
		{engine.Status{Running: true, Discord: "waiting"}, "MPCvibedRPC - Waiting for Discord"},
		{engine.Status{Running: true, NowPlaying: &long}, ("MPCvibedRPC - " + long)[:62]},
	} {
		if got := trayTooltip(c.s); got != c.want {
			t.Errorf("tooltip %q, want %q", got, c.want)
		}
	}
}

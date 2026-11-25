package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/CptPakundo/MPRP/internal/assets"
	"github.com/CptPakundo/MPRP/internal/engine"
)

// Every endpoint the settings page calls exists.
func TestEveryWindowCallHasAHandler(t *testing.T) {
	h := (&app{}).handlers()
	page := assets.UI + assets.JS
	names := []string{"start", "stop"} // chosen by the Show on Discord button
	for _, c := range regexp.MustCompile(`(?:api|act)\('([a-z-]+)'`).FindAllStringSubmatch(page, -1) {
		names = append(names, c[1])
	}
	for _, c := range regexp.MustCompile(`signInBox\('([a-z]+)'`).FindAllStringSubmatch(page, -1) {
		names = append(names, c[1], c[1]+"-signin", c[1]+"-cancel", c[1]+"-signout")
	}
	if len(names) < 30 {
		t.Fatalf("found only %d calls in the page", len(names))
	}
	for _, n := range names {
		if _, ok := h[n]; !ok {
			t.Errorf("the page calls /api/%s, which has no handler", n)
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
		{engine.Status{}, "MPRP - Not showing on Discord"},
		{engine.Status{Running: true, NowPlaying: &title}, "MPRP - Sample Show S01E02"},
		{engine.Status{Running: true, Discord: "standby"}, "MPRP - Ready"},
		{engine.Status{Running: true, Discord: "waiting"}, "MPRP - Waiting for Discord"},
		{engine.Status{Running: true, NowPlaying: &long}, ("MPRP - " + long)[:62]},
	} {
		if got := trayTooltip(c.s); got != c.want {
			t.Errorf("tooltip %q, want %q", got, c.want)
		}
	}
}

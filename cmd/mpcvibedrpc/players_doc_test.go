package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every supported player and service is named in every text that lists them: a new player that is missing from one
// fails here, instead of being found in the docs by a user.
func TestPlayersAreNamedEverywhere(t *testing.T) {
	players := []string{"MPC-HC", "MPC-BE", "MPC-QT", "mpv", "VLC", "PotPlayer", "IINA", "Kodi", "Plex", "Stremio", "Jellyfin", "Emby"}
	files := []string{
		"README.md", "SECURITY.md",
		"docs/how-it-works.md", "docs/troubleshooting.md", "docs/development.md", "docs/config.md",
		"cmd/mpcvibedrpc/main.go",   // the header comment
		"tools/mkrsrc/main.go",      // the exe's file description
		"internal/assets/ui.js",     // the welcome and help texts
		"internal/engine/engine.go", // the package comment and Status.Player
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range players {
			if !strings.Contains(string(b), p) {
				t.Errorf("%s does not name %s", f, p)
			}
		}
	}
}

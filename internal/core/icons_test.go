package core

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every logo the program names is in docs/players (served from there once on master), and is a square PNG.
func TestPlayerIconsExist(t *testing.T) {
	for player, name := range playerIcons {
		u := PlayerIcon(player)
		if !strings.HasPrefix(u, "https://") || !strings.HasSuffix(u, "/"+name+".png") {
			t.Errorf("%s: %q", player, u)
		}
		f, err := os.Open(filepath.Join("..", "..", "docs", "players", name+".png"))
		if err != nil {
			t.Errorf("%s: %v", player, err)
			continue
		}
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err != nil || cfg.Width != cfg.Height || cfg.Width < 64 {
			t.Errorf("%s: %dx%d %v", player, cfg.Width, cfg.Height, err)
		}
	}
	if PlayerIcon("MPC-HC") != "" || PlayerIcon("Some Player") != "" {
		t.Error("only the listed players have a logo")
	}
}

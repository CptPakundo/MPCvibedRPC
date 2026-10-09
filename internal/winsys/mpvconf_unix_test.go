//go:build !windows

package winsys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
)

// mpv's macOS and Linux layout: ~/.config/mpv (or $XDG_CONFIG_HOME/mpv), and the socket written as a full path.
func TestEnableMpvIPCUnix(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("PATH", "")
	conf := filepath.Join(cfg, "mpv", "mpv.conf")

	if !dirExists("/Applications/mpv.app") { // a Mac with mpv installed counts as having mpv
		if r := enableMpvIPC("mpvsocket", nil); r.Message != "" || r.Edited != "" {
			t.Fatalf("no mpv: %+v", r)
		}
		if _, err := os.Stat(filepath.Join(cfg, "mpv")); err == nil {
			t.Fatal("no folder may be created for an mpv that is not there")
		}
	}

	os.MkdirAll(filepath.Join(cfg, "mpv"), 0o755)
	r := enableMpvIPC("mpvsocket", nil)
	if !r.OK || r.Edited != conf || r.Using != "mpvsocket" {
		t.Fatalf("set up: %+v", r)
	}
	b, _ := os.ReadFile(conf)
	if v, _ := MpvConfValue(string(b), "input-ipc-server"); v != pipe.Path("mpvsocket") || !filepath.IsAbs(v) {
		t.Fatalf("written: %q (a bare name would be relative to mpv's working folder)", b)
	}
	if r := enableMpvIPC("mpvsocket", nil); r.Edited != "" || r.Using != pipe.Path("mpvsocket") {
		t.Fatalf("second time: %+v", r)
	}

	os.WriteFile(conf, []byte("input-ipc-server=~/.mpv-socket\n"), 0o644)
	if r := enableMpvIPC("mpvsocket", nil); r.Edited != "" || r.Using != "~/.mpv-socket" || !r.OK {
		t.Fatalf("user's name: %+v", r)
	}
	if home, _ := os.UserHomeDir(); pipe.Path("~/.mpv-socket") != filepath.Join(home, ".mpv-socket") {
		t.Errorf("~ is the home folder: %s", pipe.Path("~/.mpv-socket"))
	}
	if r := enableMpvIPC("", nil); r.Message != "" {
		t.Fatalf("off: %+v", r)
	}
}

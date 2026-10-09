//go:build windows

package winsys

import (
	"os"
	"path/filepath"
	"testing"
)

// mpv's Windows layout: portable_config next to a portable mpv, else %APPDATA%\mpv.
func TestEnableMpvIPC(t *testing.T) {
	ad := t.TempDir()
	t.Setenv("APPDATA", ad)
	t.Setenv("PATH", "")

	// no mpv anywhere: nothing is written
	if r := enableMpvIPC("mpvsocket", nil); r.Message != "" || r.Edited != "" {
		t.Fatalf("no mpv: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(ad, "mpv")); err == nil {
		t.Fatal("no folder may be created for an mpv that is not there")
	}

	// a running portable mpv: its portable_config is used
	exe := t.TempDir()
	os.MkdirAll(filepath.Join(exe, "portable_config"), 0o755)
	r := enableMpvIPC("mpvsocket", []string{exe})
	conf := filepath.Join(exe, "portable_config", "mpv.conf")
	if !r.OK || r.Edited != conf || r.Using != "mpvsocket" {
		t.Fatalf("portable: %+v", r)
	}
	b, _ := os.ReadFile(conf)
	if v, _ := MpvConfValue(string(b), "input-ipc-server"); v != "mpvsocket" {
		t.Fatalf("written: %q", b)
	}
	// again: nothing changes, the name in use is reported
	if r := enableMpvIPC("mpvsocket", []string{exe}); r.Edited != "" || r.Using != "mpvsocket" {
		t.Fatalf("second time: %+v", r)
	}

	// an installed mpv with the user's own name: kept and reported
	os.MkdirAll(filepath.Join(ad, "mpv"), 0o755)
	os.WriteFile(filepath.Join(ad, "mpv", "mpv.conf"), []byte("input-ipc-server=mine\n"), 0o644)
	if r := enableMpvIPC("mpvsocket", nil); r.Edited != "" || r.Using != "mine" || !r.OK {
		t.Fatalf("user's name: %+v", r)
	}
	// mpv looked for under no name (the user turned it off): left alone
	if r := enableMpvIPC("", nil); r.Message != "" {
		t.Fatalf("off: %+v", r)
	}
}

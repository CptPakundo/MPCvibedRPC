package winsys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMpvConfValue(t *testing.T) {
	for text, want := range map[string]string{
		"input-ipc-server=mpvsocket\n":                                          "mpvsocket",
		"\ufeff# c\r\nvo=gpu\r\n input-ipc-server = \"\\\\.\\pipe\\mine\" \r\n": `\\.\pipe\mine`,
		"--input-ipc-server=x\n":                                                "x",
		"#input-ipc-server=old\nvo=gpu\n":                                       "",
		"vo=gpu\n[profile]\ninput-ipc-server=inprofile\n":                       "",
		"": "",
	} {
		got, _ := MpvConfValue(text, "input-ipc-server")
		if got != want {
			t.Errorf("MpvConfValue(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestAddMpvConfLine(t *testing.T) {
	got := AddMpvConfLine("\ufeffvo=gpu\r\n[p]\r\nfs=yes\r\n", "input-ipc-server", "mpvsocket")
	if !strings.HasPrefix(got, "\ufeff# ") || !strings.Contains(got, "\r\ninput-ipc-server=mpvsocket\r\nvo=gpu\r\n[p]") {
		t.Errorf("got %q", got)
	}
	if v, ok := MpvConfValue(got, "input-ipc-server"); !ok || v != "mpvsocket" {
		t.Errorf("the added line must be read back, got %q", v)
	}
	got = AddMpvConfLine("vo=gpu\n", "input-ipc-server", "mpvsocket")
	if strings.Contains(got, "\r") || !strings.HasSuffix(got, "input-ipc-server=mpvsocket\nvo=gpu\n") {
		t.Errorf("LF file: got %q", got)
	}
}

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

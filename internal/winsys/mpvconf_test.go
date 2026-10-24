package winsys

import (
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
		got, _ := mpvConfValue(text, "input-ipc-server")
		if got != want {
			t.Errorf("mpvConfValue(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestAddMpvConfLine(t *testing.T) {
	got := addMpvConfLine("\ufeffvo=gpu\r\n[p]\r\nfs=yes\r\n", "input-ipc-server", "mpvsocket")
	if !strings.HasPrefix(got, "\ufeff# ") || !strings.Contains(got, "\r\ninput-ipc-server=mpvsocket\r\nvo=gpu\r\n[p]") {
		t.Errorf("got %q", got)
	}
	if v, ok := mpvConfValue(got, "input-ipc-server"); !ok || v != "mpvsocket" {
		t.Errorf("the added line must be read back, got %q", v)
	}
	got = addMpvConfLine("vo=gpu\n", "input-ipc-server", "mpvsocket")
	if strings.Contains(got, "\r") || !strings.HasSuffix(got, "input-ipc-server=mpvsocket\nvo=gpu\n") {
		t.Errorf("LF file: got %q", got)
	}
}

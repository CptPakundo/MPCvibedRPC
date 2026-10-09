package winsys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlayersIniCandidates(t *testing.T) {
	pf, ad := t.TempDir(), t.TempDir()
	t.Setenv("ProgramFiles", pf)
	t.Setenv("ProgramFiles(x86)", "")
	t.Setenv("APPDATA", ad)
	touch := func(parts ...string) string {
		f := filepath.Join(parts...)
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("[x]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	hc := touch(pf, "MPC-HC", "mpc-hc64.ini")
	be1 := touch(pf, "MPC-BE x64", "mpc-be64.ini")
	be2 := touch(ad, "MPC-BE", "mpc-be.ini")
	touch(ad, "MPC-HC", "mpc-hc.ini") // MPC-HC does not keep a user ini there

	if got := players[0].iniCandidates(nil); len(got) != 1 || got[0] != hc {
		t.Errorf("MPC-HC: %v", got)
	}
	got := players[1].iniCandidates(nil)
	if len(got) != 2 || got[0] != be1 || got[1] != be2 {
		t.Errorf("MPC-BE: %v", got)
	}
	if !players[1].installed(nil, got) {
		t.Error("MPC-BE with settings should count as installed")
	}
}

func TestPlayerBelongs(t *testing.T) {
	for exe, want := range map[string][2]bool{
		`C:\Program Files\MPC-HC\mpc-hc64.exe`:     {true, false},
		`C:\Program Files\MPC-BE x64\mpc-be64.exe`: {false, true},
		`C:\Program Files\MPC-BE\MPC-BE.EXE`:       {false, true},
		`C:\Program Files\Other\mpc-be-helper.exe`: {false, false},
	} {
		if got := [2]bool{players[0].belongs(exe), players[1].belongs(exe)}; got != want {
			t.Errorf("%s: %v, want %v", exe, got, want)
		}
	}
}

// MPC-BE keeps its switch in [WebServer] (keys EnableWebServer and Port), unlike MPC-HC's [Settings].
func TestMpcBeIniEdit(t *testing.T) {
	p := players[1]
	in := "[Settings]\nLanguage=en\n[WebServer]\nEnableWebServer=0\nPort=13579\nLocalhostOnly=1\n"
	out := SetIniValue(SetIniValue(in, p.section, p.enableKey, "1"), p.section, p.portKey, "13580")
	want := "[Settings]\nLanguage=en\n[WebServer]\nEnableWebServer=1\nPort=13580\nLocalhostOnly=1\n"
	if out != want {
		t.Errorf("got %q", out)
	}
}
func TestParseRegDword(t *testing.T) {
	out := "\r\nHKEY_CURRENT_USER\\Software\\MPC-HC\\MPC-HC\\Settings\r\n    EnableWebServer    REG_DWORD    0x1\r\n\r\n"
	if n := parseRegDword(out); n != 1 {
		t.Errorf("got %d", n)
	}
	if n := parseRegDword("    WebServerPort    REG_DWORD    0x350b\r\n"); n != 13579 {
		t.Errorf("port: got %d", n)
	}
	if n := parseRegDword("ERROR: The system was unable to find the specified registry key or value."); n != -1 {
		t.Errorf("missing: got %d", n)
	}
}

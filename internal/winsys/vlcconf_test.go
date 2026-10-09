package winsys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shape of the vlcrc VLC 3.0.24 writes (BOM, LF, a commented default per option, [module] sections), shortened.
const vlcrcDefault = "\ufeff###\n###  vlc 3.0.24\n###\n\n" +
	"[lua] # Lua interpreter\n\n# Lua interface (string)\n#intf=\n\n# Password (string)\n#http-password=\n\n" +
	"[http] # HTTP input\n\n#http-reconnect=0\n\n" +
	"[core] # core program\n\n# HTTP server address (string)\n#http-host=\n\n# HTTP server port (integer)\n#http-port=8080\n\n" +
	"# Extra interface modules (string)\n#extraintf=\n"

func TestVlcrcValue(t *testing.T) {
	text := vlcrcDefault + "extraintf=rc\r\nextraintf=http:rc\n#http-password=old\n"
	if v, ok := VlcrcValue(text, "extraintf"); !ok || v != "http:rc" {
		t.Errorf("the last line wins, like in VLC: %q %v", v, ok)
	}
	if v, ok := VlcrcValue(text, "http-password"); ok {
		t.Errorf("a commented line is not a value: %q", v)
	}
	if _, ok := VlcrcValue(text, "http"); ok {
		t.Error("only whole keys match")
	}
}

func TestSetVlcrcValue(t *testing.T) {
	// next to VLC's commented default, in its own section
	got := SetVlcrcValue(vlcrcDefault, "lua", "http-password", "pw")
	if !strings.Contains(got, "#http-password=\nhttp-password=pw\n") || !strings.HasPrefix(got, "\ufeff###") {
		t.Errorf("got:\n%s", got)
	}
	// an existing value is replaced in place
	got = SetVlcrcValue(got, "lua", "http-password", "new")
	if strings.Count(got, "http-password=") != 2 || !strings.Contains(got, "\nhttp-password=new\n") {
		t.Errorf("got:\n%s", got)
	}
	// no commented default: at the top of the section
	got = SetVlcrcValue("[core] # core program\r\n\r\n#other=1\r\n", "core", "extraintf", "http")
	if got != "[core] # core program\r\nextraintf=http\r\n\r\n#other=1\r\n" {
		t.Errorf("CRLF kept, under the section: %q", got)
	}
	// no section either (or an empty file): one is added
	if got = SetVlcrcValue("", "lua", "http-password", "pw"); got != "[lua]\nhttp-password=pw\n" {
		t.Errorf("empty file: %q", got)
	}
	if got = SetVlcrcValue("[qt]\nqt-x=1\n", "core", "extraintf", "http"); got != "[qt]\nqt-x=1\n\n[core]\nextraintf=http\n" {
		t.Errorf("new section: %q", got)
	}
}

func readFile(t *testing.T, f string) string {
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEnableVlcHTTPFresh(t *testing.T) {
	f := filepath.Join(t.TempDir(), "vlc", "vlcrc") // VLC never saved its settings: no file, no folder yet
	r := enableVlcHTTP(f, "", 8080, "")
	if !r.OK || r.Edited != f || r.Port != 8080 || len(r.Password) != 24 || !strings.Contains(r.Message, "for this PC only") {
		t.Fatalf("got %+v", r)
	}
	text := readFile(t, f)
	for k, want := range map[string]string{"extraintf": "http", "http-password": r.Password, "http-host": "127.0.0.1"} {
		if v, _ := VlcrcValue(text, k); v != want {
			t.Errorf("%s = %q, want %q", k, v, want)
		}
	}
	if _, set := VlcrcValue(text, "http-port"); set {
		t.Error("VLC's default port needs no line")
	}
	if !vlcReady(text) {
		t.Error("should be ready now")
	}
	// pressing the button again changes nothing and reports the same password
	again := enableVlcHTTP(f, text, 8080, "")
	if again.Edited != "" || again.Password != r.Password || !strings.Contains(again.Message, "already on") {
		t.Errorf("second run: %+v", again)
	}
}

func TestEnableVlcHTTPKeepsTheUsersChoices(t *testing.T) {
	f := filepath.Join(t.TempDir(), "vlcrc")
	// the user had the remote-control interface, a password, a port and a network address of their own
	text := vlcrcDefault + "extraintf=rc\nhttp-password=mine\nhttp-port=9090\nhttp-host=192.168.1.5\n"
	r := enableVlcHTTP(f, text, 8080, "ignored")
	if !r.OK || r.Password != "mine" || r.Port != 9090 || strings.Contains(r.Message, "this PC only") {
		t.Fatalf("got %+v", r)
	}
	out := readFile(t, f)
	if v, _ := VlcrcValue(out, "extraintf"); v != "rc:http" {
		t.Errorf("extraintf = %q", v)
	}
	if v, _ := VlcrcValue(out, "http-host"); v != "192.168.1.5" {
		t.Errorf("http-host = %q", v)
	}
	if !strings.HasPrefix(out, "\ufeff") {
		t.Error("the byte-order mark must stay")
	}
}

func TestEnableVlcHTTPUsesOurSettings(t *testing.T) {
	f := filepath.Join(t.TempDir(), "vlcrc")
	r := enableVlcHTTP(f, vlcrcDefault, 18080, "typed")
	out := readFile(t, f)
	if r.Password != "typed" || r.Port != 18080 {
		t.Errorf("got %+v", r)
	}
	if v, _ := VlcrcValue(out, "http-port"); v != "18080" {
		t.Errorf("http-port = %q", v)
	}
	if !strings.Contains(out, "#http-port=8080\nhttp-port=18080\n") {
		t.Errorf("the port goes next to VLC's default line:\n%s", out)
	}
}

// A web interface that is on but has no password is closed by VLC: a password is added, nothing else is touched.
func TestEnableVlcHTTPOnWithoutPassword(t *testing.T) {
	f := filepath.Join(t.TempDir(), "vlcrc")
	r := enableVlcHTTP(f, "extraintf=http\n", 8080, "")
	out := readFile(t, f)
	if !r.OK || r.Password == "" || strings.Contains(r.Message, "this PC only") {
		t.Errorf("got %+v", r)
	}
	if _, set := VlcrcValue(out, "http-host"); set {
		t.Error("a web interface the user had on keeps its address")
	}
}

func TestVlcHTTPOn(t *testing.T) {
	for v, want := range map[string]bool{"http": true, "rc:http": true, "luahttp": true, "http,rc": true, "": false, "rc": false, "httpx": false} {
		if vlcHTTPOn(v) != want {
			t.Errorf("%q: want %v", v, want)
		}
	}
}

func TestVlcConfPaths(t *testing.T) {
	dir := t.TempDir()
	portable := filepath.Join(dir, "vlc-portable")
	if err := os.MkdirAll(filepath.Join(portable, "portable"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDATA", filepath.Join(dir, "appdata"))
	paths, present := vlcConfPaths([]string{portable})
	if !present || len(paths) < 2 || paths[0] != filepath.Join(portable, "portable", "vlcrc") || paths[len(paths)-1] != filepath.Join(dir, "appdata", "vlc", "vlcrc") {
		t.Errorf("got %v %v", paths, present)
	}
}

package winsys

import (
	"bufio"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
)

func TestLoginItemPaths(t *testing.T) {
	t.Setenv("MPCRPC_HOME", "")
	home := filepath.Join("home", "someone")
	if got, want := loginItemPath("darwin", home, ""), filepath.Join(home, "Library", "LaunchAgents", "io.github.cptpakundo.mpcvibedrpc.plist"); got != want {
		t.Errorf("macOS: %s, want %s", got, want)
	}
	if got, want := loginItemPath("linux", home, ""), filepath.Join(home, ".config", "autostart", "mpcvibedrpc.desktop"); got != want {
		t.Errorf("Linux: %s, want %s", got, want)
	}
	if got, want := loginItemPath("linux", home, filepath.Join("cfg")), filepath.Join("cfg", "autostart", "mpcvibedrpc.desktop"); got != want {
		t.Errorf("Linux with XDG_CONFIG_HOME: %s, want %s", got, want)
	}
	if loginItemPath("linux", "", "") != "" || loginItemPath("darwin", "", "x") != "" {
		t.Error("no home folder, no login item")
	}
	// a copy with a data folder of its own gets a login item of its own
	t.Setenv("MPCRPC_HOME", filepath.Join("tmp", "test-copy"))
	p := loginItemPath("linux", home, "")
	if !strings.Contains(p, "mpcvibedrpc-") || p == filepath.Join(home, ".config", "autostart", "mpcvibedrpc.desktop") {
		t.Errorf("isolated copy: %s", p)
	}
}

func TestLoginItemContent(t *testing.T) {
	t.Setenv("MPCRPC_HOME", "")
	plist := loginItem("darwin", "/Applications/MPCvibedRPC.app/Contents/MacOS/MPCvibedRPC", "")
	for _, want := range []string{"<string>io.github.cptpakundo.mpcvibedrpc</string>",
		"<string>/Applications/MPCvibedRPC.app/Contents/MacOS/MPCvibedRPC</string>", "<string>--background</string>", "<key>RunAtLoad</key>\n\t<true/>"} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist lacks %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "MPCRPC_HOME") {
		t.Error("no data folder of its own, no environment")
	}
	if v, err := parsePlist(strings.NewReader(plist)); err != nil || v.(map[string]any)["RunAtLoad"] != true {
		t.Errorf("the plist must be readable: %v %v", v, err)
	}
	// odd characters are escaped
	plist = loginItem("darwin", "/Apps/A & B <x>/MPCvibedRPC", "/data/My \"copy\"")
	if !strings.Contains(plist, "A &amp; B &lt;x&gt;") || !strings.Contains(plist, "<key>MPCRPC_HOME</key>") {
		t.Errorf("escaping:\n%s", plist)
	}
	if v, err := parsePlist(strings.NewReader(plist)); err != nil {
		t.Errorf("escaped plist unreadable: %v", err)
	} else if args := v.(map[string]any)["ProgramArguments"].([]any); args[0] != "/Apps/A & B <x>/MPCvibedRPC" {
		t.Errorf("program: %v", args)
	}

	desktop := loginItem("linux", "/opt/MPCvibedRPC/MPCvibedRPC", "")
	for _, want := range []string{"[Desktop Entry]\n", "Type=Application\n", `Exec="/opt/MPCvibedRPC/MPCvibedRPC" --background` + "\n", "Name=MPCvibedRPC\n"} {
		if !strings.Contains(desktop, want) {
			t.Errorf("desktop entry lacks %q:\n%s", want, desktop)
		}
	}
	desktop = loginItem("linux", "/opt/a b/MPCvibedRPC", "/data/x")
	if !strings.Contains(desktop, `Exec=env "MPCRPC_HOME=/data/x" "/opt/a b/MPCvibedRPC" --background`) {
		t.Errorf("with a data folder:\n%s", desktop)
	}
}

func TestExecQuote(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/app":      `"/usr/bin/app"`,
		"/a b/app":          `"/a b/app"`,
		`/x"y/app`:          `"/x\\"y/app"`,
		"/cost$5/app":       `"/cost\\$5/app"`,
		"/back`tick/app":    "\"/back\\\\`tick/app\"",
		`/back\slash/app`:   `"/back\\\\slash/app"`,
		"/100%/app":         `"/100%%/app"`,
		"/line\nbreak/app":  `"/line break/app"`,
		"/ünïcode/app":      `"/ünïcode/app"`,
		"/semi;colon/app":   `"/semi;colon/app"`,
		"/apostrophe's/app": `"/apostrophe's/app"`,
	}
	for in, want := range cases {
		if got := execQuote(in); got != want {
			t.Errorf("execQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestMatchExes(t *testing.T) {
	mac := "/sbin/launchd\n/Applications/IINA.app/Contents/MacOS/IINA\n/Applications/VLC.app/Contents/MacOS/VLC\n/usr/libexec/xpcproxy\n/opt/homebrew/bin/mpv\n"
	linux := "systemd\nbash\ncelluloid\nvlc\nharuna\nmpvpaper\n"
	all := append(append(append([]player{}, otherPlayers...), unixPlayers...), players...)
	names := func(exes []string) []string {
		var out []string
		for _, p := range all {
			for _, e := range exes {
				if p.belongs(e) {
					out = append(out, p.name)
					break
				}
			}
		}
		return out
	}
	if got := names(matchExes(mac, all)); !reflect.DeepEqual(got, []string{"mpv", "VLC", "IINA"}) {
		t.Errorf("macOS: %v", got)
	}
	if got := names(matchExes(linux, all)); !reflect.DeepEqual(got, []string{"VLC", "Celluloid", "Haruna"}) {
		t.Errorf("Linux: %v", got)
	}
	if got := matchExes(mac, []player{vlcPlayer}); len(got) != 1 {
		t.Errorf("VLC only: %v", got)
	}
}

func TestOsReleaseName(t *testing.T) {
	text := "NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nID=ubuntu\n"
	if got := osReleaseName(bufio.NewScanner(strings.NewReader(text))); got != "Ubuntu 24.04.1 LTS" {
		t.Errorf("got %q", got)
	}
	if got := osReleaseName(bufio.NewScanner(strings.NewReader("PRETTY_NAME=Arch Linux\n"))); got != "Arch Linux" {
		t.Errorf("unquoted: %q", got)
	}
	if got := osReleaseName(bufio.NewScanner(strings.NewReader("ID=x\n"))); got != "" {
		t.Errorf("missing: %q", got)
	}
}

// What "defaults export com.colliderli.iina -" writes (shortened to the parts that matter, plus a few others).
const iinaExport = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>SUEnableAutomaticChecks</key>
	<false/>
	<key>enableAdvancedSettings</key>
	<%s/>
	<key>recentDocuments</key>
	<array>
		<data>
		Ym9va21hcms=
		</data>
	</array>
	<key>subTextSize</key>
	<real>32.5</real>
	<key>userOptions</key>
	<array>%s</array>
	<key>windowCount</key>
	<integer>3</integer>
</dict>
</plist>
`

func iinaWith(advanced bool, rows ...[2]string) []byte {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString("\n\t\t<array>\n\t\t\t<string>" + r[0] + "</string>\n\t\t\t<string>" + r[1] + "</string>\n\t\t</array>")
	}
	adv := "false"
	if advanced {
		adv = "true"
	}
	return []byte(strings.Replace(strings.Replace(iinaExport, "%s", adv, 1), "%s", b.String(), 1))
}

func TestReadIINA(t *testing.T) {
	s := readIINA(iinaWith(true, [2]string{"hwdec", "auto"}, [2]string{"sub-scale", "1.2"}))
	if !s.Advanced || !reflect.DeepEqual(s.Options, [][]string{{"hwdec", "auto"}, {"sub-scale", "1.2"}}) {
		t.Errorf("got %+v", s)
	}
	if s := readIINA(nil); s.Advanced || s.Options != nil {
		t.Errorf("nothing exported: %+v", s)
	}
	if s := readIINA([]byte("not a plist")); s.Advanced || s.Options != nil {
		t.Errorf("garbage: %+v", s)
	}
	v, err := parsePlist(strings.NewReader(string(iinaWith(false))))
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	if m["windowCount"] != int64(3) || m["subTextSize"] != 32.5 || m["SUEnableAutomaticChecks"] != false || len(m["recentDocuments"].([]any)) != 1 {
		t.Errorf("other values: %v", m)
	}
}

func TestPlanIINA(t *testing.T) {
	path := pipe.Path("iina-mpvsocket")
	// nothing set up: the option is added after the user's own
	opts, using := planIINA(iinaSettings{Options: [][]string{{"hwdec", "auto"}}}, "iina-mpvsocket")
	if using != "iina-mpvsocket" || !reflect.DeepEqual(opts, [][]string{{"hwdec", "auto"}, {"input-ipc-server", path}}) {
		t.Errorf("fresh: %v %q", opts, using)
	}
	// a connection the user set up is kept and adopted
	opts, using = planIINA(iinaSettings{Advanced: true, Options: [][]string{{"input-ipc-server", "/tmp/my-iina"}}}, "iina-mpvsocket")
	if opts != nil || using != "/tmp/my-iina" {
		t.Errorf("kept: %v %q", opts, using)
	}
	// an empty value does not count
	opts, _ = planIINA(iinaSettings{Options: [][]string{{"input-ipc-server", " "}}}, "x")
	if len(opts) != 2 {
		t.Errorf("empty value: %v", opts)
	}
}

func TestOldStylePlist(t *testing.T) {
	got := oldStylePlist([][]string{{"hwdec", "auto"}, {"input-ipc-server", `/tmp/a "b"\c`}})
	want := `(("hwdec", "auto"), ("input-ipc-server", "/tmp/a \"b\"\\c"))`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if oldStylePlist(nil) != "()" {
		t.Error("an empty list")
	}
}

func TestMpvConfPathsUnix(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	paths, _ := mpvConfPathsUnix("/home/someone")
	if !reflect.DeepEqual(paths, []string{filepath.Join(cfg, "mpv", "mpv.conf")}) {
		t.Errorf("XDG: %v", paths)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	paths, _ = mpvConfPathsUnix("/home/someone")
	if !reflect.DeepEqual(paths, []string{filepath.Join("/home/someone", ".config", "mpv", "mpv.conf")}) {
		t.Errorf("home: %v", paths)
	}
}

func TestTranslocatedAppGetsNoLoginItem(t *testing.T) {
	if !translocated("/private/var/folders/ab/xyz/T/AppTranslocation/0A1B/d/MPCvibedRPC.app/Contents/MacOS/MPCvibedRPC") {
		t.Error("a translocated app must be recognised")
	}
	if translocated("/Applications/MPCvibedRPC.app/Contents/MacOS/MPCvibedRPC") {
		t.Error("an app in Applications is not translocated")
	}
}

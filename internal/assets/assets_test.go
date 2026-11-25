package assets

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/CptPakundo/MPRP/internal/store"
)

// The page's script must parse: no Go test runs it, and one stray apostrophe would leave the window blank.
func TestPageScriptParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the page's script cannot be checked here")
	}
	js := filepath.Join(t.TempDir(), "ui.js")
	if err := os.WriteFile(js, []byte(JS), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", js).CombinedOutput(); err != nil {
		t.Fatalf("node --check: %v\n%s", err, out)
	}
}

// The page's script behaves: testdata/page_test.js runs ui.js in a small fake DOM built from ui.html, with a stubbed
// fetch playing the program's API, and checks the saver, the status sync, Text lines and Fix a match. The state it
// starts from carries the real settings layout and a store's values. Like TestPageScriptParses it needs Node.js (CI
// runners have it).
func TestPageBehaviour(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the page's script cannot be run here")
	}
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixes := []map[string]any{
		{"match": "Sample Show", "entry": "https://www.imdb.com/title/tt0000003/", "label": "Sample Show (2019) · Show · IMDb", "episodeOffset": 12},
		{"match": "sample.movie.2020.720p.mkv", "file": true, "cover": "https://img.example/sample-movie.jpg"},
	}
	if err := st.SetSettings(map[string]any{"matchFixes": fixes}); err != nil {
		t.Fatal(err)
	}
	app := st.App()
	app.SeenVersion, app.WelcomeSeen = "0.0.0", true
	state := map[string]any{
		"version": "0.0.0", "schema": store.SectionsFor("windows"), "os": "windows", "values": st.Values(), "app": app,
		"status": map[string]any{"running": true, "discord": "connected", "playerAnswers": true, "player": "MPC-HC", "nowPlaying": "Sample Show",
			"paused": false, "preview": nil, "hideTitle": false, "streamTitle": "auto"},
		"autoStart": false, "dataDir": "", "update": nil, "changelog": []any{}, "stats": nil, "tray": false,
	}
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cmd := exec.Command(node, filepath.Join("testdata", "page_test.js"), write("ui.html", UI), write("ui.js", JS), write("state.json", string(b)),
		filepath.Join("..", "core", "testdata", "lines.json"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the page's behaviour tests failed: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

// The page policy (script-src 'self', style-src 'self') allows no inline script or style: the markup must hold none,
// and must load the two files the server serves. The script reads the token from the meta tag.
func TestPageHasNoInlineCode(t *testing.T) {
	if strings.Count(UI, "<script") != 1 || !strings.Contains(UI, `<script src="/ui.js" defer></script>`) {
		t.Error("the page should load /ui.js and hold no other <script>")
	}
	if strings.Contains(UI, "<style") || !strings.Contains(UI, `<link rel="stylesheet" href="/ui.css">`) {
		t.Error("the page should load /ui.css and hold no <style> block")
	}
	if re := regexp.MustCompile(`(?i)\s(style|on[a-z]+)\s*=`); re.MatchString(UI) {
		t.Errorf("the page has an inline style or event attribute: %q", re.FindString(UI))
	}
	if strings.Count(UI, "__TOKEN__") != 1 || !strings.Contains(UI, `<meta name="token" content="__TOKEN__">`) {
		t.Error("the token belongs in the page's meta tag only")
	}
	if strings.Contains(JS, "__TOKEN__") || !strings.Contains(JS, `meta[name="token"]`) {
		t.Error("the script should read the token from the meta tag")
	}
	if re := regexp.MustCompile(`setAttribute\(\s*['"]style['"]|\.cssText|style\s*=`); re.MatchString(JS) {
		t.Errorf("the script sets an inline style attribute: %q", re.FindString(JS))
	}
}

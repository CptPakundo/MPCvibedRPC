package winsys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsesProfile(t *testing.T) {
	const p = `C:\Users\Someone\AppData\Local\MPCvibedRPC\window`
	cases := []struct {
		cmd  string
		want bool
	}{
		{`"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" --app=http://127.0.0.1:47654/ "--user-data-dir=` + p + `" --no-first-run`, true},
		{`msedge.exe --type=gpu-process --user-data-dir=` + p + ` --lang=en-GB`, true},
		{`msedge.exe --user-data-dir=` + p, true},
		{`msedge.exe --user-data-dir=` + p + `\`, true},
		{`msedge.exe --USER-data-DIR=` + `c:\users\someone\appdata\local\mpcvibedrpc\WINDOW`, true},
		{`msedge.exe --user-data-dir=` + p + `2 --lang=en`, false},   // a different folder with the same prefix
		{`msedge.exe --user-data-dir=` + p + `-old`, false},          // likewise
		{`msedge.exe --user-data-dir=C:\Users\Someone\Other`, false}, // another profile
		{`msedge.exe --app=http://127.0.0.1:1/`, false},              // the user's own Edge
		{``, false},
	}
	for _, c := range cases {
		if got := usesProfile(c.cmd, p); got != c.want {
			t.Errorf("usesProfile(%q) = %v, want %v", c.cmd, got, c.want)
		}
	}
	if usesProfile(`msedge.exe --user-data-dir=x`, "") {
		t.Error("an empty profile must never match")
	}
}

func readState(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "Local State"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("Local State is not valid JSON: %v\n%s", err, raw)
	}
	return m
}

func boolAt(m map[string]any, key string) (bool, bool) {
	sub, _ := m[key].(map[string]any)
	v, ok := sub["enabled"].(bool)
	return v, ok
}

func TestPrepareProfileFresh(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "window")
	prepareProfile(dir)
	m := readState(t, dir)
	for _, k := range []string{"startup_boost", "background_mode"} {
		if v, ok := boolAt(m, k); !ok || v {
			t.Errorf("%s.enabled should be false, got %v (set=%v)", k, v, ok)
		}
	}
}

func TestPrepareProfileKeepsOtherSettings(t *testing.T) {
	dir := t.TempDir()
	existing := `{"startup_boost":{"enabled":true,"last_browser_open_time":"13435995412192726"},"other":{"big":12345678901234567890,"name":"x"},"keep":true}`
	if err := os.WriteFile(filepath.Join(dir, "Local State"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	prepareProfile(dir)
	raw, _ := os.ReadFile(filepath.Join(dir, "Local State"))
	m := readState(t, dir)
	if v, _ := boolAt(m, "startup_boost"); v {
		t.Error("startup_boost was not switched off")
	}
	if sb, _ := m["startup_boost"].(map[string]any); sb["last_browser_open_time"] != "13435995412192726" {
		t.Errorf("a sibling setting was lost: %v", sb)
	}
	if m["keep"] != true {
		t.Error("an unrelated setting was lost")
	}
	if !strings.Contains(string(raw), "12345678901234567890") {
		t.Errorf("a large number was changed: %s", raw)
	}
}

func TestPrepareProfileNoRewriteWhenRight(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Local State")
	right := `{"startup_boost":{"enabled":false},"background_mode":{"enabled":false},"x":1}`
	if err := os.WriteFile(file, []byte(right), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	prepareProfile(dir)
	st, _ := os.Stat(file)
	if !st.ModTime().Equal(old) {
		t.Error("the file was rewritten although the settings were already right")
	}
}

func TestPrepareProfileRepairsDamagedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Local State"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	prepareProfile(dir)
	if v, ok := boolAt(readState(t, dir), "startup_boost"); !ok || v {
		t.Error("a damaged file should be replaced by valid settings")
	}
}

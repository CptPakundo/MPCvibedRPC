package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The release text comes from the newest changelog entry; a hand-written block is added when its file exists; the
// wrong version is refused.
func TestBuild(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "changelog.json")
	os.WriteFile(changelog, []byte(`[
  {"version": "9.9.9", "date": "2026-01-02", "items": ["🧹 Sample line one.", "🔒 Sample line two."]},
  {"version": "9.9.8", "date": "2026-01-01", "items": ["Older."]}
]`), 0o644)
	advisory := filepath.Join(dir, "advisory.md")

	text, err := build("9.9.9", changelog, advisory)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## MPCvibedRPC 9.9.9\n", "- 🧹 Sample line one.\n- 🔒 Sample line two.\n", "### Get it", "MPCvibedRPC.exe"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Older") {
		t.Error("an older entry leaked in")
	}

	os.WriteFile(advisory, []byte("> **If you run a sample server:** do the sample thing.\r\n"), 0o644)
	text, err = build("9.9.9", changelog, advisory)
	if err != nil || !strings.Contains(text, "- 🔒 Sample line two.\n\n> **If you run a sample server:** do the sample thing.\n") {
		t.Fatalf("advisory not placed after the lines: %v\n%s", err, text)
	}

	if _, err := build("9.9.8", changelog, advisory); err == nil {
		t.Fatal("an older version must be refused")
	}
	if _, err := build("", changelog, advisory); err == nil {
		t.Fatal("no version must be refused")
	}
}

// The real changelog renders: the newest entry's version is what TestChangelog pins to main.go.
func TestRealChangelogRenders(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "assets", "changelog.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(raw), `"version": "`)
	if i < 0 {
		t.Fatal("no version in the changelog")
	}
	v := string(raw[i+len(`"version": "`):])
	v = v[:strings.Index(v, `"`)]
	text, err := build(v, path, filepath.Join(t.TempDir(), "none.md"))
	if err != nil || !strings.HasPrefix(text, "## MPCvibedRPC "+v+"\n") {
		t.Fatalf("%v\n%s", err, text)
	}
}

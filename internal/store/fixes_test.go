package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CptPakundo/MPRP/internal/core"
)

// The legacy Fix a match lines become fixes once: a title in both maps is one fix, the old keys stay in the file, and
// a second start changes nothing.
func TestMigrateMatchFixes(t *testing.T) {
	dir := t.TempDir()
	old := `{
  "artworkAliases": {"sample movie": "Sample Film", "sample show": "Sample Series"},
  "artworkOverrides": {"sample show": "https://img.example/show.jpg", "another sample": "https://img.example/another.jpg"},
  "titleAsName": false
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := openStore(t, dir)
	c0 := s.Config()
	legacy := c0.Fixes() // what the program used before the migration: the maps read as fixes
	n, err := s.MigrateMatchFixes()
	if err != nil || n != 3 {
		t.Fatalf("migrated %d (%v), want 3 titles", n, err)
	}
	cfg := s.Config()
	want := []core.MatchFix{
		{Match: "sample movie", Search: "Sample Film"},
		{Match: "sample show", Search: "Sample Series", Cover: "https://img.example/show.jpg"},
		{Match: "another sample", Cover: "https://img.example/another.jpg"},
	}
	if !reflect.DeepEqual(cfg.MatchFixes, want) {
		t.Errorf("matchFixes = %+v\nwant %+v", cfg.MatchFixes, want)
	}
	if len(legacy) != 3 || !sameFixes(legacy, want) {
		t.Errorf("before the migration the maps must act as the same fixes: %+v", legacy)
	}
	if len(cfg.ArtworkAliases) != 2 || len(cfg.ArtworkOverrides) != 2 || cfg.TitleAsName {
		t.Errorf("the old keys and other settings must stay: %+v %+v", cfg.ArtworkAliases, cfg.ArtworkOverrides)
	}
	b1, _ := os.ReadFile(s.File)
	if n, err := s.MigrateMatchFixes(); n != 0 || err != nil {
		t.Errorf("a second run migrated %d (%v)", n, err)
	}
	if b2, _ := os.ReadFile(s.File); string(b1) != string(b2) {
		t.Error("a second run changed the file")
	}
	// Remove in the window saves the list without it; the old line does not come back
	if err := s.SetSettings(map[string]any{"matchFixes": []any{}}); err != nil {
		t.Fatal(err)
	}
	if c1 := s.Config(); len(c1.Fixes()) != 0 {
		t.Error("after removing every fix the old lines came back")
	}

	// nothing to migrate: the file is not written
	empty := openStore(t, t.TempDir())
	if n, err := empty.MigrateMatchFixes(); n != 0 || err != nil || !empty.FirstRun() {
		t.Errorf("an install without old lines: %d %v, file written: %v", n, err, !empty.FirstRun())
	}

	// lines the window's rules refuse are made to pass, or left out, and that is logged once
	s, logs := legacyStore(t)
	if n, err := s.MigrateMatchFixes(); err != nil || n != 3 {
		t.Fatalf("migrated %d (%v), want 3 titles", n, err)
	}
	want = []core.MatchFix{
		{Match: "sample long", Search: strings.Repeat("a", maxLineLen)}, // clipped
		{Match: "sample show", Search: "Sample Series"},                 // its cover is not an http(s) address
		{Match: "sample movie", Cover: "https://img.example/movie.jpg"}, // trimmed
	}
	if got := s.Config().MatchFixes; !reflect.DeepEqual(got, want) {
		t.Errorf("matchFixes = %+v\nwant %+v", got, want)
	}
	if len(*logs) != 1 || !strings.Contains((*logs)[0], "2 of the old") || !strings.Contains((*logs)[0], "and 3 were left out") {
		t.Errorf("log %q: want one line, 2 changed and 3 left out", *logs)
	}

	// nothing usable left: an empty list, so the old lines are no longer read (the window could not save them)
	none := openStore(t, t.TempDir())
	os.WriteFile(none.File, []byte(`{"artworkOverrides": {"sample show": "C:/covers/show.jpg"}}`), 0o600)
	if n, err := none.MigrateMatchFixes(); n != 0 || err != nil {
		t.Errorf("migrated %d (%v)", n, err)
	}
	if c := none.Config(); c.MatchFixes == nil || len(c.Fixes()) != 0 {
		t.Errorf("fixes after the migration: %+v (matchFixes %v)", c.Fixes(), c.MatchFixes)
	}
}

// legacyStore is an install with old Fix a match lines that the window's rules refuse: a cover that is not an
// http(s) address, text over its limits, control characters, and a cover too long for the cover finder.
func legacyStore(t *testing.T) (*Store, *[]string) {
	t.Helper()
	old := map[string]any{
		"artworkAliases": map[string]string{
			"sample long":   strings.Repeat("a", maxLineLen+20),
			"sample show":   "Sample Series",
			"sample broken": "Sample\u0001Title", // control character, no cover: nothing left
		},
		"artworkOverrides": map[string]string{
			"sample show":   "file:///covers/show.jpg",
			"sample movie":  "  https://img.example/movie.jpg ",
			"sample huge":   "https://img.example/" + strings.Repeat("x", maxCoverLen), // too long: nothing left
			"sample\u0002x": "https://img.example/x.jpg",                               // the title cannot pass
		},
	}
	b, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	s := openStore(t, dir)
	var logs []string
	s.Log = func(level, msg string) { logs = append(logs, level+" "+msg) }
	return s, &logs
}

// After a migration of such lines, saving in the window works: it sends the whole list back as it shows it.
func TestSaveAfterMigratingRefusedLines(t *testing.T) {
	s, _ := legacyStore(t)
	if _, err := s.MigrateMatchFixes(); err != nil {
		t.Fatal(err)
	}
	fixes := s.Values()["matchFixes"]
	if err := s.SetSettings(map[string]any{"matchFixes": fixes, "titleAsName": false}); err != nil {
		t.Fatalf("the window's save after the migration: %v", err)
	}
	if c := s.Config(); len(c.MatchFixes) != 3 || c.TitleAsName {
		t.Errorf("saved %+v", c.MatchFixes)
	}
}

func sameFixes(a, b []core.MatchFix) bool {
	key := func(l []core.MatchFix) map[string]core.MatchFix {
		m := map[string]core.MatchFix{}
		for _, f := range l {
			m[f.Match] = f
		}
		return m
	}
	return reflect.DeepEqual(key(a), key(b))
}

func TestCleanFixes(t *testing.T) {
	s := openStore(t, t.TempDir())
	ok := []any{
		map[string]any{"match": "  Sample Show ", "entry": "https://www.imdb.com/title/tt0000001/", "search": "ignored with an entry", "label": "Sample Show (2019) · Show · IMDb", "episodeOffset": 12, "seasonOffset": 0},
		map[string]any{"match": "sample.movie.2020.mkv", "file": true, "cover": "https://img.example/a.jpg"},
		map[string]any{"match": "SAMPLE SHOW", "search": "Sample Series"}, // replaces the first: the same title
	}
	if err := s.SetSettings(map[string]any{"matchFixes": ok}); err != nil {
		t.Fatal(err)
	}
	got := s.Config().MatchFixes
	want := []core.MatchFix{{Match: "SAMPLE SHOW", Search: "Sample Series"}, {Match: "sample.movie.2020.mkv", File: true, Cover: "https://img.example/a.jpg"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	b, _ := os.ReadFile(s.File)
	if !strings.Contains(string(b), `"match": "sample.movie.2020.mkv",
      "file": true,
      "cover": "https://img.example/a.jpg"`) {
		t.Errorf("stored as:\n%s", b)
	}
	for _, bad := range []map[string]any{
		{"search": "no title"},
		{"match": "x", "entry": "https://example.com/not-a-catalog"},
		{"match": "x", "cover": "file:///C:/poster.jpg"},
		{"match": "x", "episodeOffset": 1.5},
		{"match": "x", "episodeOffset": 9999},
		{"match": "line\nbreak"},
	} {
		if err := s.SetSettings(map[string]any{"matchFixes": []any{bad}}); err == nil {
			t.Errorf("%v was taken", bad)
		}
	}
	if !reflect.DeepEqual(s.Config().MatchFixes, want) {
		t.Error("a refused list changed the saved one")
	}
}

func TestCleanTextLines(t *testing.T) {
	s := openStore(t, t.TempDir())
	if err := s.SetSettings(map[string]any{"textLines": "custom", "customLines": map[string]any{"episodeDetails": " {code} · {episodeTitle} ", "movieCover": ""}}); err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	if cfg.TextLines != "custom" || !reflect.DeepEqual(cfg.CustomLines, map[string]string{"episodeDetails": "{code} · {episodeTitle}"}) {
		t.Errorf("got %q %+v", cfg.TextLines, cfg.CustomLines)
	}
	if v := s.Values()["customLines"]; !reflect.DeepEqual(v, map[string]any{"episodeDetails": "{code} · {episodeTitle}"}) {
		t.Errorf("the window gets %#v", v)
	}
	for _, bad := range []map[string]any{
		{"textLines": "fancy"},
		{"customLines": map[string]any{"title": "x"}},
		{"customLines": map[string]any{"episodeState": 5}},
		{"customLines": map[string]any{"episodeState": strings.Repeat("x", 301)}},
		{"customLines": "{title}"},
	} {
		if err := s.SetSettings(bad); err == nil {
			t.Errorf("%v was taken", bad)
		}
	}
	if d := core.DefaultConfig(); d.TextLines != "standard" || len(d.CustomLines) != 0 || d.MatchFixes != nil {
		t.Errorf("the defaults must change nothing: %q %v %v", d.TextLines, d.CustomLines, d.MatchFixes)
	}
}

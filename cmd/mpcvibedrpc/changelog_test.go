package main

import (
	"encoding/json"
	"regexp"
	"testing"
	"unicode/utf8"

	"github.com/CptPakundo/MPCvibedRPC/internal/assets"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
)

// The changelog's newest entry is this version (a release cannot ship without one), the entries run newest first,
// and every line is short.
func TestChangelog(t *testing.T) {
	var log []struct {
		Version string   `json:"version"`
		Date    string   `json:"date"`
		Items   []string `json:"items"`
	}
	if err := json.Unmarshal([]byte(assets.Changelog), &log); err != nil {
		t.Fatal(err)
	}
	if len(log) == 0 || log[0].Version != version {
		t.Fatalf("the newest changelog entry must be %s (the version in main.go)", version)
	}
	date := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	for i, v := range log {
		if !date.MatchString(v.Date) || len(v.Items) == 0 {
			t.Errorf("%s: date %q, %d items", v.Version, v.Date, len(v.Items))
		}
		if i > 0 && (updater.Compare(v.Version, log[i-1].Version) >= 0 || v.Date > log[i-1].Date) {
			t.Errorf("%s comes after %s: newest first", v.Version, log[i-1].Version)
		}
		for _, it := range v.Items {
			if n := utf8.RuneCountInString(it); n < 10 || n > 130 {
				t.Errorf("%s: %d characters, keep it short: %q", v.Version, n, it)
			}
		}
	}
}

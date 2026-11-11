package core

import (
	"strings"
	"testing"
)

// A nameless file takes its show from the folder. Its marker is found on the name with runs of dots and underscores
// collapsed, so what follows the marker must be cut from the name at the right place, runs included.
func TestFolderNameWithDoubledSeparators(t *testing.T) {
	cfg := DefaultConfig()
	dirs := []string{"Season 2", "Sample Show"}
	want := ParseMedia("Ep_05 - Sample Title.mkv", &cfg, dirs)
	got := ParseMedia("Ep__05 - Sample Title.mkv", &cfg, dirs)
	if want.Season != 2 || want.Episode != 5 {
		t.Fatalf("the plain form: %+v", want)
	}
	if got.Display != want.Display || got.Season != want.Season || got.Episode != want.Episode || got.EpTitle != want.EpTitle {
		t.Errorf("doubled separators: %q S%dE%d %q, want %q S%dE%d %q", got.Display, got.Season, got.Episode, got.EpTitle, want.Display, want.Season, want.Episode, want.EpTitle)
	}
}

// A name far longer than a file name can be is cut, not parsed at the square of its length.
func TestVeryLongNameIsCut(t *testing.T) {
	cfg := DefaultConfig()
	long := "Sample Movie (2020) " + strings.Repeat("a-b c.d ", 20000)
	m := ParseMedia(long, &cfg, nil)
	if m == nil || !strings.HasPrefix(m.Title, "Sample Movie") {
		t.Errorf("got %+v", m)
	}
}

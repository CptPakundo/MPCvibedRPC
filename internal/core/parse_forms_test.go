package core

import "testing"

// File-name forms the recorded parse data does not cover: an episode code in square brackets, an unpadded S2E5, an
// episode range written E05-E06, and "Ep. 5" with a dot.
func TestParseEpisodeForms(t *testing.T) {
	cfg := DefaultConfig()
	for _, c := range []struct {
		file, title string
		season, ep  int
		display     string
	}{
		{"Sample Show [S01E02].mkv", "Sample Show", 1, 2, "Sample Show - S01E02"},
		{"Sample Show [1x02] [1080p].mkv", "Sample Show", 1, 2, "Sample Show - S01E02"},
		{"Sample Show S2E5.mkv", "Sample Show", 2, 5, "Sample Show - S02E05"},
		{"Sample.Show.s01e05-e06.mkv", "Sample Show", 1, 5, "Sample Show - S01E05E06"},
		{"Sample Show E05-E06.mkv", "Sample Show", NA, 5, "Sample Show - E05E06"},
		{"Sample Show Ep. 5.mkv", "Sample Show", NA, 5, "Sample Show - E05"},
		{"Sample Show S01E123.mkv", "Sample Show", 1, 123, "Sample Show - S01E123"},
		{"Sample Show - S01E05 - S01E06.mkv", "Sample Show", 1, 5, "Sample Show - S01E05E06"},
		{"Sample Show - S01E05 - S01E06 - Two Parts.mkv", "Sample Show", 1, 5, "Sample Show - S01E05E06 - Two Parts"},
		{"Sample Show - S01E05 - S02E01.mkv", "Sample Show", 1, 5, "Sample Show - S01E05 - S02E01"}, // not one double episode
	} {
		m := ParseMedia(c.file, &cfg, nil)
		if !m.IsEpisode || m.Title != c.title || m.Season != c.season || m.Episode != c.ep || m.Display != c.display {
			t.Errorf("%s: title %q, season %d, episode %d, display %q", c.file, m.Title, m.Season, m.Episode, m.Display)
		}
	}
	// A special's words after a year in brackets stay in the title (for the lookup's NamedSpecial); an edition note
	// does not.
	if m := ParseMedia("Sample Show (2010) - Special 01.mkv", &cfg, nil); m.Title != "Sample Show - Special 01" || m.Year != 2010 || NamedSpecial(m) == nil {
		t.Errorf("special after a year: title %q, year %d", m.Title, m.Year)
	}
	if m := ParseMedia("Sample Movie (1982) Final Cut.mkv", &cfg, nil); m.Title != "Sample Movie" || m.Year != 1982 {
		t.Errorf("edition after a year: title %q, year %d", m.Title, m.Year)
	}
	// Square brackets that are not an episode stay tags.
	if m := ParseMedia("[Group] Sample Movie (2020) [1080p].mkv", &cfg, nil); m.IsEpisode || m.Title != "Sample Movie" {
		t.Errorf("tags: %+v", m)
	}
}

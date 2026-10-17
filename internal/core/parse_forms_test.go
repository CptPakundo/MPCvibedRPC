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
	} {
		m := ParseMedia(c.file, &cfg, nil)
		if !m.IsEpisode || m.Title != c.title || m.Season != c.season || m.Episode != c.ep || m.Display != c.display {
			t.Errorf("%s: title %q, season %d, episode %d, display %q", c.file, m.Title, m.Season, m.Episode, m.Display)
		}
	}
	// Square brackets that are not an episode stay tags.
	if m := ParseMedia("[Group] Sample Movie (2020) [1080p].mkv", &cfg, nil); m.IsEpisode || m.Title != "Sample Movie" {
		t.Errorf("tags: %+v", m)
	}
}

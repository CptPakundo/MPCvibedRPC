package jellyfin

import (
	"fmt"
	"regexp"
	"strings"
)

// Item is what the server says about what a session plays (the parts used).
type Item struct {
	ID                string `json:"Id"`
	Name              string `json:"Name"`
	Type              string `json:"Type"`      // Movie, Episode, Video, MusicVideo, Audio, TvChannel, ...
	MediaType         string `json:"MediaType"` // Video, Audio, Photo, Book
	SeriesName        string `json:"SeriesName"`
	ParentIndexNumber *int   `json:"ParentIndexNumber"` // the season of an episode
	IndexNumber       *int   `json:"IndexNumber"`       // its number in the season
	ProductionYear    int    `json:"ProductionYear"`
	RunTimeTicks      int64  `json:"RunTimeTicks"` // 100 ns
}

// IsVideo tells video apart from music, audiobooks and photos, which are never shown as "Watching".
func (it *Item) IsVideo() bool {
	switch it.Type {
	case "Movie", "Episode", "Video", "MusicVideo", "Trailer", "TvChannel", "Recording":
		return true
	case "Audio", "AudioBook", "Photo", "Book":
		return false
	}
	return it.MediaType == "Video"
}

// Duration is the item's length in milliseconds (0 when unknown).
func (it *Item) Duration() int { return int(it.RunTimeTicks / 10000) }

var (
	// rePlaceholder matches a title that only repeats the numbering ("Episode 5", "S01E05").
	rePlaceholder = regexp.MustCompile(`(?i)^(?:episode\s+\d+|e\d+|s\d+\s*e\d+)$`)
	// reCode finds a season and episode code in a title made from a file name ("Sample Show - S01E02").
	reCode = regexp.MustCompile(`(?i)\bs\d{1,2}\s*e\d{1,4}\b`)
)

// clean keeps a title from looking like a folder path to the file-name parser.
func clean(s string) string {
	s = strings.NewReplacer("/", "-", `\`, "-", "\r", " ", "\n", " ").Replace(s)
	return strings.TrimSpace(s)
}

// FileName writes the item the way a well-named file would be called ("Sample Show - S01E02 - Title",
// "Sample Movie (2020)"), so the same parsing and catalog lookups apply as for a file.
func (it *Item) FileName() string {
	title := clean(it.Name)
	switch it.Type {
	case "Episode":
		show := clean(it.SeriesName)
		if show == "" {
			return title
		}
		ep := title
		low, lowShow := strings.ToLower(ep), strings.ToLower(show)
		if rePlaceholder.MatchString(ep) || reCode.MatchString(ep) || low == lowShow || strings.HasPrefix(low, lowShow+" -") {
			ep = "" // the server had no title and used the numbering or the file name: let the catalog find the real one
		}
		code := ""
		if it.ParentIndexNumber != nil && it.IndexNumber != nil {
			code = fmt.Sprintf("S%02dE%02d", *it.ParentIndexNumber, *it.IndexNumber)
		} else if it.IndexNumber != nil {
			code = fmt.Sprintf("E%02d", *it.IndexNumber)
		}
		parts := []string{show}
		for _, p := range []string{code, ep} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, " - ")
	case "Movie":
		if it.ProductionYear > 0 && title != "" && !strings.Contains(title, fmt.Sprintf("(%d)", it.ProductionYear)) {
			return fmt.Sprintf("%s (%d)", title, it.ProductionYear)
		}
	}
	return title
}

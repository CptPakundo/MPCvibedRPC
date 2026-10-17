package jellyfin

import (
	"regexp"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
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
	Path              string `json:"Path"`         // the file on the server, when it says (for the Never show list only)
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

// FileName writes the item the way a well-named file would be called ("Sample Show - S01E02 - Title",
// "Sample Movie (2020)"), so the same parsing and catalog lookups apply as for a file.
func (it *Item) FileName() string {
	title := core.CleanTitle(it.Name)
	switch it.Type {
	case "Episode":
		show := core.CleanTitle(it.SeriesName)
		if show == "" {
			return title
		}
		low, lowShow := strings.ToLower(title), strings.ToLower(show)
		if rePlaceholder.MatchString(title) || reCode.MatchString(title) || low == lowShow || strings.HasPrefix(low, lowShow+" -") {
			title = "" // the server had no title and used the numbering or the file name: let the catalog find the real one
		}
		return core.EpisodeName(show, title, it.ParentIndexNumber, it.IndexNumber)
	case "Movie":
		return core.MovieName(title, it.ProductionYear)
	}
	return title
}

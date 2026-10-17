package plex

import (
	"regexp"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// Item is what the server says about the video being played (library metadata; only the parts used).
type Item struct {
	Type             string `json:"type"` // movie, episode, clip, track, ...
	Title            string `json:"title"`
	GrandparentTitle string `json:"grandparentTitle"` // the show of an episode
	ParentIndex      *int   `json:"parentIndex"`      // its season
	Index            *int   `json:"index"`            // its number in the season
	Year             int    `json:"year"`
	Duration         int    `json:"duration"` // ms
	// where the server keeps it (for the Never show list only)
	LibrarySectionTitle string `json:"librarySectionTitle"`
	Media               []struct {
		Part []struct {
			File string `json:"file"`
		} `json:"Part"`
	} `json:"Media"`
}

// Where is where the server keeps the item: its library name and its file path, for the Never show list.
func (it *Item) Where() []string {
	var w []string
	if it.LibrarySectionTitle != "" {
		w = append(w, it.LibrarySectionTitle)
	}
	if len(it.Media) > 0 && len(it.Media[0].Part) > 0 && it.Media[0].Part[0].File != "" {
		w = append(w, it.Media[0].Part[0].File)
	}
	return w
}

// IsVideo tells video apart from music, photos and the like, which are never shown as "Watching".
func (it *Item) IsVideo() bool {
	switch it.Type {
	case "movie", "episode", "clip", "video":
		return true
	}
	return false
}

// rePlaceholder matches the title Plex gives an episode it knows nothing about ("Episode 5").
var rePlaceholder = regexp.MustCompile(`(?i)^episode\s+\d+$`)

// Name writes the item the way a well-named file would be called ("Sample Show - S01E02 - Title",
// "Sample Movie (2020)"), so the same parsing and catalog lookups apply as for a file.
func (it *Item) Name() string {
	title := core.CleanTitle(it.Title)
	switch it.Type {
	case "episode":
		show := core.CleanTitle(it.GrandparentTitle)
		if show == "" {
			return title
		}
		if rePlaceholder.MatchString(title) {
			title = "" // let the catalog find the real title
		}
		return core.EpisodeName(show, title, it.ParentIndex, it.Index)
	case "movie":
		return core.MovieName(title, it.Year)
	}
	return title
}

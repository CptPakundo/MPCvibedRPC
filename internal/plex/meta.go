package plex

import (
	"fmt"
	"regexp"
	"strings"
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

// clean keeps a title from looking like a folder path to the file-name parser.
func clean(s string) string {
	s = strings.NewReplacer("/", "-", `\`, "-", "\r", " ", "\n", " ").Replace(s)
	return strings.TrimSpace(s)
}

// Name writes the item the way a well-named file would be called ("Sample Show - S01E02 - Title",
// "Sample Movie (2020)"), so the same parsing and catalog lookups apply as for a file.
func (it *Item) Name() string {
	title := clean(it.Title)
	switch it.Type {
	case "episode":
		show := clean(it.GrandparentTitle)
		if show == "" {
			return title
		}
		ep := title
		if rePlaceholder.MatchString(ep) {
			ep = "" // let the catalog find the real title
		}
		code := ""
		if it.ParentIndex != nil && it.Index != nil {
			code = fmt.Sprintf("S%02dE%02d", *it.ParentIndex, *it.Index)
		} else if it.Index != nil {
			code = fmt.Sprintf("E%02d", *it.Index)
		}
		parts := []string{show}
		for _, p := range []string{code, ep} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, " - ")
	case "movie":
		if it.Year > 0 && title != "" {
			return fmt.Sprintf("%s (%d)", title, it.Year)
		}
	}
	return title
}

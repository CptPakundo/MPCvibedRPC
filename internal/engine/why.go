package engine

import (
	"strconv"
	"strings"

	"github.com/CptPakundo/MPRP/internal/artwork"
	"github.com/CptPakundo/MPRP/internal/core"
)

// readAs is the start of "Why this title?": the name the player gave and what was read from it. The folder path is
// never included (only "from its folder"), as in the log.
func readAs(info *core.Info, media *core.Media) []string {
	out := []string{info.Player + `: "` + info.File + `"`}
	parts := []string{`title "` + media.Title + `"`}
	if media.Year != core.NA && media.Year != 0 {
		parts = append(parts, "year "+strconv.Itoa(media.Year))
	}
	if media.Season != core.NA {
		parts = append(parts, "season "+strconv.Itoa(media.Season))
	}
	if media.Episode != core.NA {
		parts = append(parts, "episode "+strconv.Itoa(media.Episode))
	}
	if media.Code != "" {
		parts = append(parts, "production code "+media.Code)
	}
	if media.EpTitle != "" {
		parts = append(parts, `episode title "`+media.EpTitle+`"`)
	}
	if media.Anime {
		parts = append(parts, "named like an anime release")
	}
	if media.FromFolder {
		parts = append(parts, "show name from its folder")
	}
	out = append(out, "Read as: "+strings.Join(parts, ", ")+".")
	return out
}

// MatchInfo is what the window's Fix this match dialog starts from (Status.Match). File is the file's base name,
// never its folder; Season and Episode are the numbers read from the file (0: none); Now describes the current
// match ("" for none), NowEntry is its catalog page, and Fixed says the user's fix applies.
type MatchInfo struct {
	File     string `json:"file"`
	Title    string `json:"title"`
	Season   int    `json:"season,omitempty"`
	Episode  int    `json:"episode,omitempty"`
	Now      string `json:"now,omitempty"`
	NowEntry string `json:"nowEntry,omitempty"`
	Fixed    bool   `json:"fixed,omitempty"`
}

// matchOf is the dialog's starting point for a title read as media and found as found (nil: nothing found).
func matchOf(media *core.Media, found *core.Art, art *artwork.Artwork) *MatchInfo {
	m := &MatchInfo{File: media.File, Title: media.Title}
	if media.IsEpisode && media.Season != core.NA {
		m.Season = media.Season
	}
	if media.IsEpisode && media.Episode != core.NA {
		m.Episode = media.Episode
	}
	if found != nil {
		switch {
		case found.Source == "override":
			m.Now = "your image"
		default:
			m.Now = found.Name
			if found.Year != core.NA && found.Year != 0 {
				m.Now += " (" + strconv.Itoa(found.Year) + ")"
			}
			m.NowEntry = found.URL
		}
	}
	m.Fixed = art.FixFor(media) != nil
	return m
}

// baseName is a file name without its folder (a player may give a whole path or an address).
func baseName(file string) string {
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		return file[i+1:]
	}
	return file
}

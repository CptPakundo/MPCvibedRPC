package core

import (
	"fmt"
	"strings"
)

// CleanTitle keeps a title from looking like a folder path to the file-name parser.
func CleanTitle(s string) string {
	s = strings.NewReplacer("/", "-", `\`, "-", "\r", " ", "\n", " ").Replace(s)
	return strings.TrimSpace(s)
}

// EpisodeName writes an episode the way a well-named file would be called ("Sample Show - S01E02 - Title"). The show
// and the episode title are already cleaned; the title may be empty, and so may either number.
func EpisodeName(show, title string, season, episode *int) string {
	code := ""
	if season != nil && episode != nil {
		code = fmt.Sprintf("S%02dE%02d", *season, *episode)
	} else if episode != nil {
		code = fmt.Sprintf("E%02d", *episode)
	}
	parts := []string{show}
	for _, p := range []string{code, title} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " - ")
}

// MovieName writes a movie the way a well-named file would be called ("Sample Movie (2020)"): the year is added
// unless the title has it or it is unknown.
func MovieName(title string, year int) string {
	if year > 0 && title != "" && !strings.Contains(title, fmt.Sprintf("(%d)", year)) {
		return fmt.Sprintf("%s (%d)", title, year)
	}
	return title
}

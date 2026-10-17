package engine

import (
	"strconv"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
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

package artwork

import (
	"strconv"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// Explain says in a few lines how the catalogs answered for media, for "Why this title?" in Copy diagnostics: which
// one matched and as what, where the episode title came from, or why nothing matched. found is what Lookup returned.
func (a *Artwork) Explain(media *core.Media, found *core.Art) []string {
	if a == nil || media == nil {
		return nil
	}
	cfg := a.cfg
	if !cfg.ShowArtwork {
		return []string{"Cover lookups are off (Titles & art)."}
	}
	var out []string
	add := func(s string) { out = append(out, s) }
	nt := normTitle(media.Title)
	if alias := a.aliases[nt]; alias != "" {
		add(`Searched as "` + alias + `" (Fix a match > Search under another name).`)
	}
	if found == nil {
		j := a.newLookupJob(media)
		a.mu.Lock()
		missed, soft, closest := a.missed(j.key), a.soft[j.key], a.closest[nt]
		a.mu.Unlock()
		switch {
		case missed && soft:
			add("No match yet: a catalog was slow or offline, so it is asked again soon.")
		case missed:
			add("No catalog matched.")
			if len(closest) > 0 {
				add("Closest results: " + strings.Join(closest, "; ") + ".")
			}
		default:
			add("Still being looked up.")
		}
		return out
	}
	if found.Source == "override" {
		add("Cover pinned by you (Fix a match > Pin a cover image).")
	} else {
		m := "Matched by " + found.Source + `: "` + found.Name + `"`
		var bits []string
		if found.Year != core.NA && found.Year != 0 {
			bits = append(bits, strconv.Itoa(found.Year))
		}
		switch found.Type {
		case "movie":
			bits = append(bits, "a movie")
		case "series":
			bits = append(bits, "a show")
		}
		if len(bits) > 0 {
			m += " (" + strings.Join(bits, ", ") + ")"
		}
		if found.ViaRank {
			m += ", the best known of that name (the file has no year)"
		}
		if found.SeasonName != "" {
			m += `, season "` + found.SeasonName + `"`
		}
		add(m + ".")
		if found.Alt != nil && found.Source != "AniList" && cfg.AnimeTitles != "file" {
			add("Its anime names come from AniList.")
		}
	}
	if ep := found.Episode; ep != nil {
		m := "Episode title from " + ep.Source
		if ep.Via != "" && normTitle(ep.Via) != nt {
			m += ` (looked up as "` + ep.Via + `")`
		}
		add(m + ".")
	} else if media.IsEpisode && media.Episode != core.NA && media.EpTitle == "" && cfg.EpisodeTitles {
		add("No episode title found (yet).")
	}
	if hasInfoData(found.Info) {
		add("Genres and rating from Cinemeta.")
	}
	return out
}

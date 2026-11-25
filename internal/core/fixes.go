package core

import (
	"regexp"
	"sort"
	"strconv"
)

// MatchFix is one fix made in Fix a match: what the user said a title, or one file, is.
//
// A fix applies to every file read as Match (the read title, compared the way the cover finder compares titles), or,
// with File set, to the one file whose base name is Match (any case). A file fix wins over a title fix. Entry pins a
// catalog page (IMDb, TMDB, TVmaze, AniList or Kitsu); Search searches under another name instead; Cover is an image
// address shown in place of the catalog's cover. The offsets are added to the season and episode read from the file
// before the catalogs are asked (0 and 0: none). Label and Poster only describe the pinned entry in the window.
type MatchFix struct {
	Match         string `json:"match"`
	File          bool   `json:"file,omitempty"`
	Entry         string `json:"entry,omitempty"`
	Search        string `json:"search,omitempty"`
	Cover         string `json:"cover,omitempty"`
	SeasonOffset  int    `json:"seasonOffset,omitempty"`
	EpisodeOffset int    `json:"episodeOffset,omitempty"`
	Label         string `json:"label,omitempty"`
	Poster        string `json:"poster,omitempty"`
}

// Fixes is Fix a match as the program uses it: matchFixes, or, while config.json has none (an install from before
// matchFixes), the legacy artworkAliases and artworkOverrides lines read as fixes, so they act as before.
func (c *Config) Fixes() []MatchFix {
	if c.MatchFixes != nil {
		return c.MatchFixes
	}
	return LegacyFixes(c.ArtworkAliases, c.ArtworkOverrides)
}

// LegacyFixes turns the legacy maps into fixes: one per title (sorted), the alias as Search and the pinned cover as
// Cover; a title in both is one fix. nil when both are empty.
func LegacyFixes(aliases, overrides map[string]string) []MatchFix {
	var keys []string
	seen := map[string]bool{}
	for _, m := range []map[string]string{aliases, overrides} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	var out []MatchFix
	for _, k := range keys {
		f := MatchFix{Match: k, Search: aliases[k], Cover: overrides[k]}
		if f.Search != "" || f.Cover != "" {
			out = append(out, f)
		}
	}
	return out
}

var (
	reLabelSE = regexp.MustCompile(`^S(\d+)E(\d+)$`)
	reLabelE  = regexp.MustCompile(`^E(\d+)$`)

	reEntryImdb    = regexp.MustCompile(`^https?://(?:www\.|m\.)?imdb\.com/title/(tt\d{5,10})/?(?:[?#].*)?$`)
	reEntryTmdb    = regexp.MustCompile(`^https?://(?:www\.)?themoviedb\.org/(movie|tv)/(\d{1,10})(?:-[\w-]*)?/?(?:[?#].*)?$`)
	reEntryTvmaze  = regexp.MustCompile(`^https?://(?:www\.)?tvmaze\.com/shows/(\d{1,10})(?:/[\w-]*)?/?(?:[?#].*)?$`)
	reEntryAnilist = regexp.MustCompile(`^https?://anilist\.co/anime/(\d{1,10})(?:/[^/?#]*)?/?(?:[?#].*)?$`)
	reEntryKitsu   = regexp.MustCompile(`^https?://kitsu\.(?:io|app)/anime/([\w-]{1,100})/?(?:[?#].*)?$`)
)

// Entry is a catalog page a fix pins: which catalog (a core.Sources id), the kind when the address tells it ("movie",
// "series" or ""), and the catalog's own id (a Kitsu slug for Kitsu).
type Entry struct{ Catalog, Kind, ID string }

// EntryPage reads a catalog page address (IMDb, TMDB, TVmaze, AniList or Kitsu); ok is false for anything else.
func EntryPage(u string) (e Entry, ok bool) {
	if len(u) > 512 {
		return e, false
	}
	if m := reEntryImdb.FindStringSubmatch(u); m != nil {
		return Entry{Catalog: "imdb", ID: m[1]}, true
	}
	if m := reEntryTmdb.FindStringSubmatch(u); m != nil {
		kind := "movie"
		if m[1] == "tv" {
			kind = "series"
		}
		return Entry{Catalog: "tmdb", Kind: kind, ID: m[2]}, true
	}
	if m := reEntryTvmaze.FindStringSubmatch(u); m != nil {
		return Entry{Catalog: "tvmaze", Kind: "series", ID: m[1]}, true
	}
	if m := reEntryAnilist.FindStringSubmatch(u); m != nil {
		return Entry{Catalog: "anilist", ID: m[1]}, true
	}
	if m := reEntryKitsu.FindStringSubmatch(u); m != nil {
		return Entry{Catalog: "kitsu", ID: m[1]}, true
	}
	return e, false
}

// ShiftMedia adds a fix's season and episode offsets to what was read from the file (after folderEpisodeNumbers chose
// the file's number), so that the catalogs are asked, and the card shows, the catalog's numbers. ok is false, and
// media is returned as it is, when a number would fall below 1. A season offset needs a season read from the file.
func ShiftMedia(media *Media, season, episode int) (out *Media, ok bool) {
	if media == nil || !media.IsEpisode || (season == 0 && episode == 0) {
		return media, true
	}
	cp := *media
	if season != 0 && cp.Season != NA {
		cp.Season += season
		if cp.Season < 1 {
			return media, false
		}
	}
	if episode != 0 && cp.Episode != NA {
		cp.Episode += episode
		if cp.Episode < 1 {
			return media, false
		}
		if truthy(cp.BadgeEpisode) {
			cp.BadgeEpisode += episode
			if cp.BadgeEpisode < 1 {
				return media, false
			}
		}
	}
	relabel := func(p string) string {
		if m := reLabelSE.FindStringSubmatch(p); m != nil && cp.Season != NA && cp.Episode != NA {
			return "S" + pad2n(cp.Season) + "E" + pad2n(cp.Episode)
		}
		if m := reLabelE.FindStringSubmatch(p); m != nil && cp.Episode != NA {
			return "E" + pad2n(cp.Episode)
		}
		return p
	}
	cp.Parts = make([]string, len(media.Parts))
	for i, p := range media.Parts {
		cp.Parts[i] = relabel(p)
	}
	if len(cp.Parts) > 0 {
		cp.Label = joinParts(cp.Parts)
	} else {
		cp.Label = relabel(cp.Label)
	}
	return &cp, true
}

func joinParts(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += " · "
		}
		s += p
	}
	return s
}

// OffsetText says what a fix's offsets do, for the window and the reports: "seasons +1, episodes +12" ("" for none).
func OffsetText(season, episode int) string {
	sign := func(n int) string {
		if n > 0 {
			return "+" + strconv.Itoa(n)
		}
		return strconv.Itoa(n)
	}
	var parts []string
	if season != 0 {
		parts = append(parts, "seasons "+sign(season))
	}
	if episode != 0 {
		parts = append(parts, "episodes "+sign(episode))
	}
	return joinNonEmpty(", ", parts...)
}

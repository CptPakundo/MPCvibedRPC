package artwork

import (
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// blockedHostFilter returns a function that reports whether a host belongs to a service switched off in the
// settings (Config.DisabledSources), or nil when none is. It is the last line of defence: the lookups already skip
// disabled services, and this makes sure that no code path can reach one anyway.
func blockedHostFilter(cfg *core.Config) func(host string) bool {
	if len(cfg.DisabledSources) == 0 {
		return nil
	}
	var hosts []string
	add := func(id, base string) {
		if cfg.SourceOn(id) {
			return
		}
		if h := baseHost(base); h != "" {
			hosts = append(hosts, strings.ToLower(h))
		}
	}
	add("tmdb", "https://api.themoviedb.org")
	add("imdb", cfg.ImdbSuggestBase)
	add("cinemeta", cfg.CinemetaBase)
	add("tvmaze", cfg.TvmazeBase)
	add("anilist", cfg.AnilistURL)
	add("kitsu", cfg.KitsuBase)
	add("jikan", cfg.JikanBase)
	add("bulbapedia", cfg.BulbapediaBase)
	add("wikipedia", cfg.WikipediaBase)
	if len(hosts) == 0 {
		return nil
	}
	return func(host string) bool {
		host = strings.ToLower(host)
		for _, h := range hosts {
			if host == h || strings.HasSuffix(host, "."+h) {
				return true
			}
		}
		return false
	}
}

// baseHost is the host part of a base URL ("{lang}." placeholders, as in the Wikipedia address, are dropped so that
// every language edition matches).
func baseHost(base string) string {
	s := base
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "{lang}.", "")
	s = strings.ReplaceAll(s, "{lang}", "")
	return strings.Trim(s, ".")
}
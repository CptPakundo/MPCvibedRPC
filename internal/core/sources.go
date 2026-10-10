package core

import "strings"

// SourceInfo describes an online service the program can ask about a title (to find cover art, episode names,
// genres and ratings). The settings window lists them with a switch each.
type SourceInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Used  string `json:"used"` // what it is used for
	Host  string `json:"host"` // where requests go (for display)
}

// Sources is every online service the program can contact about titles, in the order the window shows them.
var Sources = []SourceInfo{
	{"tmdb", "TMDB", "Cover art and episode names. Only used if you add your own API key.", "api.themoviedb.org"},
	{"imdb", "IMDb", "Title search.", "v3.sg.media-imdb.com"},
	{"cinemeta", "Cinemeta", "An IMDb-based catalog: titles, genres, ratings and runtimes.", "v3-cinemeta.strem.io"},
	{"tvmaze", "TVmaze", "TV shows, seasons and episode names.", "api.tvmaze.com"},
	{"anilist", "AniList", "Anime titles and covers.", "graphql.anilist.co"},
	{"kitsu", "Kitsu", "Anime titles, covers and episode names.", "kitsu.io"},
	{"jikan", "MyAnimeList (via Jikan)", "Anime titles, covers and episode names.", "api.jikan.moe"},
	{"wikipedia", "Wikipedia", "A last-resort cover image from the article.", "wikipedia.org"},
	{"bulbapedia", "Bulbapedia", "Episode names for files named with a production code.", "bulbapedia.bulbagarden.net"},
}

// SourceOn reports whether a service may be contacted: it is not listed in DisabledSources.
func (c *Config) SourceOn(id string) bool {
	for _, d := range c.DisabledSources {
		if strings.EqualFold(strings.TrimSpace(d), id) {
			return false
		}
	}
	return true
}

// IsSourceID reports whether id names one of Sources.
func IsSourceID(id string) bool {
	for _, s := range Sources {
		if s.ID == id {
			return true
		}
	}
	return false
}

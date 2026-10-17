package core

import "strings"

// How a host outside this computer is reached.
const (
	HostAsked  = "asked"  // the program sends it requests
	HostLoaded = "loaded" // the settings window and Discord load an image from it (a cover, a player's logo)
	HostOpened = "opened" // only a link: a page opened in your browser, or a button on your status
)

// HostInfo is one fixed address outside this computer that the program, its window or Discord may reach because of
// it. Not listed: addresses you type in yourself (a Plex, Jellyfin, Emby or Kodi server, a Jikan server), and the
// image hosts a catalog's answer names for its covers.
type HostInfo struct {
	Host    string `json:"host"` // also covers its subdomains ("wikipedia.org": "en.wikipedia.org")
	Service string `json:"service"`
	How     string `json:"how"` // HostAsked, HostLoaded or HostOpened
	When    string `json:"when"`
}

// Hosts is every fixed outside address besides the catalogs in Sources. SECURITY.md lists them all, and a test
// fails when the code names an address that is in neither list.
var Hosts = []HostInfo{
	{"api.github.com", "GitHub", HostAsked, "Check for updates is on: asks for the newest release."},
	{"github.com", "GitHub", HostAsked, "Check for updates is on: the release page when the API's hourly limit is used up, and the download."},
	{"release-assets.githubusercontent.com", "GitHub", HostAsked, "An update is installed: GitHub sends the download from here."},
	{"plex.tv", "Plex", HostAsked, "Signing in to Plex, and while signed in (your servers)."},
	{"clients.plex.tv", "Plex", HostAsked, "While signed in to Plex: your servers and their addresses."},
	{"link.stremio.com", "Stremio", HostAsked, "Signing in to Stremio (the link code)."},
	{"api.strem.io", "Stremio", HostAsked, "While signed in to Stremio: your library every 20 seconds, and signing out."},
	{"raw.githubusercontent.com", "GitHub", HostLoaded, "The player's logo on your status and in the preview."},
	{"image.tmdb.org", "TMDB", HostLoaded, "A cover found by TMDB (with your own TMDB key)."},
	{"app.plex.tv", "Plex", HostOpened, "The page where you approve the Plex sign-in."},
	{"stremio.com", "Stremio", HostOpened, "The page where you enter the Stremio link code."},
	{"www.imdb.com", "IMDb", HostOpened, "The title's page, linked from your status while Link to the title's page is on."},
	{"www.themoviedb.org", "TMDB", HostOpened, "The title's page, linked from your status; TMDB's credit on the Help tab."},
	{"www.tvmaze.com", "TVmaze", HostOpened, "The title's page, linked from your status; TVmaze's credit on the Help tab."},
	{"kitsu.io", "Kitsu", HostOpened, "The title's page, linked from your status (Kitsu is also a catalog)."},
}

// KnownHost reports whether host is in Hosts or Sources (or a subdomain of one), and how it is reached.
func KnownHost(host string) (how string, ok bool) {
	host = strings.ToLower(host)
	under := func(h string) bool { return host == h || strings.HasSuffix(host, "."+h) }
	for _, s := range Sources {
		if under(s.Host) {
			return HostAsked, true
		}
	}
	for _, h := range Hosts {
		if under(h.Host) {
			return h.How, true
		}
	}
	return "", false
}

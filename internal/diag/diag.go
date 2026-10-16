// Package diag builds the "Copy diagnostics" text: version, system, the settings that differ from the defaults,
// the current state and the recent log, for pasting into a bug report. Titles and file names stay in: a wrong title can
// only be traced back to the name it came from. Left out are folder paths (they can hold a user name), the paths and
// queries of web addresses, sign-ins, keys and the Never show list.
package diag

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
)

// Input is everything the report is made from.
type Input struct {
	Version string
	System  string   // e.g. "Windows 11 (10.0.26300)"
	Players []string // running players by name, e.g. "MPC-BE"
	Config  core.Config
	App     store.App
	Status  engine.Status
	Log     []string
}

var (
	// A web address keeps only its server: a user:password@ part, the path and the query go.
	reURL    = regexp.MustCompile(`(https?://)(?:[^/\s"'@]*@)?([^/\s"']+)[^\s"']*`)
	reServer = regexp.MustCompile(`(server) "[^"]*"`) // a server's name is often a person's or a computer's name
	// A path may contain spaces, so it runs to the ": " that ends it in an error message, to a closing quote, or to the
	// end of the line.
	reWinPath = regexp.MustCompile(`(^|[^A-Za-z0-9])(?:[A-Za-z]:[\\/]|\\\\)[^"']*?(:\s|["']|$)`)
	reUnix    = regexp.MustCompile(`(^|[\s("'])/(?:home|Users|Volumes|tmp|var|private|mnt|media|run|opt|srv|storage)/[^"']*?(:\s|["']|$)`)
)

// Redact shortens a log line for a bug report: web addresses keep only their server (a path or query can carry a key),
// and folder paths and server names are replaced (they can hold a user name). Titles and file names stay: they are
// what a report about a wrong title needs.
func Redact(line string) string {
	line = reURL.ReplaceAllString(line, "${1}${2}/…")
	line = reServer.ReplaceAllString(line, `$1 "…"`)
	line = reWinPath.ReplaceAllString(line, "$1<path>$2")
	return reUnix.ReplaceAllString(line, "$1<path>$2")
}

// changed lists the settings that differ from the defaults. Values that could identify you or what you watch are
// summarised instead of shown.
func changed(cfg core.Config) []string {
	cur, def := asMap(cfg), asMap(core.DefaultConfig())
	var keys []string
	for k := range cur {
		if !reflect.DeepEqual(cur[k], def[k]) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		out = append(out, k+" = "+summarise(k, cur[k]))
	}
	return out
}

func asMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

// privateKeys are the settings whose value is never shown, only that it is set: keys, sign-ins, accounts and servers.
var privateKeys = map[string]bool{
	"tmdbApiKey": true, "vlcPassword": true, "stremioKey": true,
	"plexToken": true, "plexAccount": true, "plexUser": true, "plexServer": true, "plexServerName": true, "plexClient": true, "plexAddress": true,
	"jellyfinAddress": true, "jellyfinToken": true, "jellyfinUser": true, "jellyfinUserName": true, "jellyfinServerName": true, "jellyfinDevice": true,
}

func summarise(key string, v any) string {
	if privateKeys[key] {
		return "(set)"
	}
	switch key {
	case "clientId":
		return "(custom)"
	case "mpvPipe", "iinaPipe":
		return pathSafe(v)
	}
	if strings.HasSuffix(key, "Base") || key == "anilistUrl" {
		return baseSafe(v)
	}
	switch key {
	case "hideFiles":
		return fmt.Sprintf("(%d entries)", count(v)) // the user's private list stays private
	case "artworkOverrides":
		b, _ := json.Marshal(v)
		return reURL.ReplaceAllString(string(b), "${1}${2}/…") // which titles have a pinned cover, and from which server
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// pathSafe shows a bare name (an mpv pipe name) as it is and anything with a path in it as "(custom path)": a path
// can hold a user's home folder.
func pathSafe(v any) string {
	s, _ := v.(string)
	if strings.ContainsAny(s, `/\~`) {
		return `"(custom path)"`
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// baseSafe shows only the server of a custom web address; anything else with a path in it is "(custom path)".
func baseSafe(v any) string {
	s, _ := v.(string)
	for _, p := range []string{"http://", "https://"} {
		if strings.HasPrefix(strings.ToLower(s), p) {
			rest := s[len(p):]
			if i := strings.IndexAny(rest, "/?#"); i >= 0 {
				return `"` + s[:len(p)+i] + `/…"`
			}
			return `"` + s + `"`
		}
	}
	return pathSafe(s)
}

func count(v any) int {
	switch x := v.(type) {
	case []any:
		return len(x)
	case map[string]any:
		return len(x)
	}
	return 0
}

func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Report builds the text.
func Report(in Input) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("MPCvibedRPC diagnostics")
	w("Version: %s", in.Version)
	w("System: %s", in.System)
	if len(in.Players) > 0 {
		w("Players running: %s", strings.Join(in.Players, ", "))
	} else {
		w("Players running: none found")
	}
	vlc := "off (no password)"
	if in.Config.VlcPassword != "" {
		vlc = fmt.Sprintf("port %d", in.Config.VlcPort)
	}
	w("Web interface port: %d, mpv connection: %s, VLC web interface: %s", in.Config.Port, orNone(strings.Trim(pathSafe(in.Config.MpvPipe), `"`)), vlc)
	plex := "off (not signed in)"
	if in.Config.PlexToken != "" {
		plex = "signed in"
		if in.Config.PlexServer == "" {
			plex += ", no server chosen"
		}
		if in.Config.PlexAddress != "" {
			plex += ", fixed server address"
		}
	}
	w("Plex: %s", plex)
	stremio := "off (not signed in)"
	if in.Config.StremioKey != "" {
		stremio = "signed in"
	}
	w("Stremio: %s", stremio)
	jellyfin := "off (not signed in)"
	if in.Config.JellyfinToken != "" {
		jellyfin = "signed in"
		if in.Config.JellyfinProduct != "" {
			jellyfin += " to a " + in.Config.JellyfinProduct + " server"
		}
	}
	w("Jellyfin / Emby: %s", jellyfin)
	if in.Status.Hint != "" {
		w("Player hint: %s", Redact(in.Status.Hint))
	}
	w("")
	w("State")
	w("  Presence on: %s", yn(in.Status.Running))
	w("  Discord: %s", in.Status.Discord)
	if in.Status.MPC && in.Status.Player != "" {
		w("  Player reachable: yes (%s)", in.Status.Player)
	} else {
		w("  Player reachable: %s", yn(in.Status.MPC))
	}
	w("  Something playing: %s (paused: %s, hidden: %s, cleared while paused: %s)", yn(in.Status.NowPlaying != nil), yn(in.Status.Paused), yn(in.Status.Hidden), yn(in.Status.PauseCleared))
	if in.Status.NowPlaying != nil {
		w("  Now playing: %s", *in.Status.NowPlaying)
	}
	if in.Status.LastError != nil && *in.Status.LastError != "" {
		w("  Last error: %s", Redact(*in.Status.LastError))
	}
	w("")
	w("App settings")
	w("  start with Windows: %s, start presence at launch: %s, show window at launch: %s, check for updates: %s", yn(in.App.AutoStart), yn(in.App.StartPresence), yn(in.App.OpenWindow), yn(in.App.CheckUpdates))
	if in.App.UpdateRepo != store.DefaultUpdateRepo {
		w("  update source: (custom)")
	}
	w("")
	w("Settings that differ from the defaults")
	if c := changed(in.Config); len(c) == 0 {
		w("  (none)")
	} else {
		for _, l := range c {
			w("  %s", l)
		}
	}
	w("")
	w("Recent log (titles and file names kept; folder paths and web address details left out)")
	if len(in.Log) == 0 {
		w("  (empty)")
	}
	for _, l := range in.Log {
		w("  %s", Redact(l))
	}
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "(off)"
	}
	return s
}

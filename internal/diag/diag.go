// Package diag builds the "Copy diagnostics" text: version, system, the settings that differ from the defaults,
// the current state and the recent log, with everything private left out (titles, file names, paths, URL queries,
// keys and the like). It is meant to be pasted into a bug report.
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
	rePlaying = regexp.MustCompile(`^(\[[^\]]*\] \w+ )(Playing|Paused): .*$`)
	reQuote   = regexp.MustCompile(`^([^"]*)".*$`)
	reVia     = regexp.MustCompile(`\bvia ([A-Za-z]+)`)
	reURL     = regexp.MustCompile(`(https?://[^/\s"']+)[^\s"']*`)
	// A path may contain spaces, so it runs to the ": " that ends it in an error message (or the end of the line).
	reWinPath = regexp.MustCompile(`(^|[^A-Za-z0-9])(?:[A-Za-z]:[\\/]|\\\\)[^"']*?(:\s|$)`)
	reUnix    = regexp.MustCompile(`(^|[\s(])/(?:home|Users|tmp|var|mnt|media)/[^"']*?(:\s|$)`)
)

// Redact removes what a log line says about what you watch: the title after "Playing:"/"Paused:", anything from the
// first quote on (titles are always quoted in the log; the service that answered is kept), URL paths and queries,
// and file paths.
func Redact(line string) string {
	if m := rePlaying.FindStringSubmatch(line); m != nil {
		return m[1] + m[2] + ": (title omitted)"
	}
	if strings.Contains(line, `"`) {
		tail := line[strings.Index(line, `"`):]
		out := reQuote.ReplaceAllString(line, `$1"…"`)
		if v := reVia.FindStringSubmatch(tail); v != nil {
			out += " (via " + v[1] + ")"
		}
		line = out + " (details omitted)"
	}
	line = reURL.ReplaceAllString(line, "$1/…")
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

func summarise(key string, v any) string {
	switch key {
	case "tmdbApiKey", "vlcPassword", "plexToken", "plexAccount", "plexUser", "plexServer", "plexServerName", "plexClient", "plexAddress":
		return "(set)"
	case "clientId":
		return "(custom)"
	case "artworkAliases", "artworkOverrides", "hideFiles":
		return fmt.Sprintf("(%d entries)", count(v))
	}
	b, _ := json.Marshal(v)
	return string(b)
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
	w("Web interface port: %d, mpv connection: %s, VLC web interface: %s", in.Config.Port, orNone(in.Config.MpvPipe), vlc)
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
	if in.Status.Hint != "" {
		w("Player hint: %s", in.Status.Hint)
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
	w("Recent log (titles, file names and paths left out)")
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

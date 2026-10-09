// Package store keeps the settings file and describes the settings shown in the window.
package store

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

// Field is one setting shown in the window. type: bool | int | text | secret | choice | map ("name = value" per line) |
// sources (a switch per online service; the value is the list of services that are OFF) | lines (one entry per line,
// stored as a list) | heading (a group title in the window; it has no key and no value)
type Field struct {
	Key     string            `json:"key"`
	Type    string            `json:"type"`
	Label   string            `json:"label"`
	Help    string            `json:"help,omitempty"`
	Options [][2]string       `json:"options,omitempty"`
	Min     *int              `json:"min,omitempty"`
	Max     *int              `json:"max,omitempty"`
	Sources []core.SourceInfo `json:"sources,omitempty"`
}

// Section groups fields in the window.
type Section struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Fields []Field `json:"fields"`
}

func ip(n int) *int { return &n }

// Sections is the settings window's layout. Adding a field here adds a control (and its validation).
var Sections = []Section{
	{ID: "look", Title: "How it looks", Fields: []Field{
		{Key: "activityType", Type: "choice", Label: "Style", Options: [][2]string{{"watching", "Watching (with progress bar)"}, {"playing", "Playing (classic timer)"}}},
		{Key: "titleAsName", Type: "bool", Label: "Show the title as the activity name", Help: `Shows "Watching <title>" instead of "Watching Media Player Classic".`},
		{Key: "textProgressBar", Type: "bool", Label: "Text progress bar while paused", Help: "Discord cannot freeze its own bar, so a ▰▰▱▱ bar is shown in text."},
		{Key: "richInfo", Type: "bool", Label: "Genres, rating and director", Help: "Extra info line from the catalog match."},
		{Key: "linkButton", Type: "bool", Label: `Add a "View on IMDb" style button`, Help: "Visible to others on your profile."},
		{Key: "episodeInDetails", Type: "bool", Label: "Repeat the season and episode number in the title line", Help: "Discord already shows a season and episode badge, so this is off by default."},
	}},
	{ID: "titles", Title: "Titles and artwork", Fields: []Field{
		{Key: "episodeTitles", Type: "bool", Label: "Look up episode titles"},
		{Key: "catalogTitle", Type: "bool", Label: "Use the catalog's spelling of titles", Help: "Restores punctuation that file names cannot contain, such as a colon in a title."},
		{Key: "animeTitles", Type: "choice", Label: "Anime titles", Options: [][2]string{{"auto", "Closest AniList name (auto)"}, {"english", "Always English"}, {"romaji", "Always romaji"}, {"file", "Keep the file's spelling"}}},
		{Key: "useFolderName", Type: "bool", Label: "Use the folder name when the file is just an episode number"},
		{Key: "folderEpisodeNumbers", Type: "bool", Label: "Number code episodes by folder position", Help: "For files named with a running production code, show the episode as its position within the season folder."},
		{Key: "tmdbApiKey", Type: "secret", Label: "TMDB API key (optional)", Help: "Free at themoviedb.org/settings/api. Improves matches for some titles."},
	}},
	{ID: "privacy", Title: "Privacy", Fields: []Field{
		{Type: "heading", Label: "What other people see on Discord"},
		{Key: "hideTitle", Type: "bool", Label: "Hide what I'm watching", Help: "Shows only \"Watching a video\" with the progress bar: no title, cover, episode or buttons. Nothing is looked up online either."},
		{Key: "hideFiles", Type: "lines", Label: "Never show these files", Help: "One entry per line. A file is hidden when its path contains the entry (any case), for example a folder name such as Private. Use * and ? as wildcards, for example *.xyz. A hidden file shows no status at all and is not looked up."},
		{Key: "pauseClearMinutes", Type: "int", Label: "Clear my status when paused for (minutes)", Help: "0 keeps it until you resume (the default is 30). The status comes back when you play again or move around in the video.", Min: ip(0), Max: ip(1440)},
		{Type: "heading", Label: "What online services see"},
		{Key: "showArtwork", Type: "bool", Label: "Look up cover art and titles online", Help: "Turn off to stay completely offline: nothing about what you watch is sent anywhere."},
		{Key: "disabledSources", Type: "sources", Label: "Services that may see your titles", Help: "The title from the file name is sent to these services to find cover art and details. Switch off any you don't want to use. They are never contacted while off.", Sources: core.Sources},
	}},
	{ID: "advanced", Title: "Advanced", Fields: []Field{
		{Key: "port", Type: "int", Label: "MPC-HC web interface port", Help: "MPC-BE and MPC-QT's web interfaces use it too. MPC-QT also works without one.", Min: ip(1), Max: ip(65535)},
		{Key: "mpvPipe", Type: "text", Label: "mpv connection name", Help: "The input-ipc-server name in mpv.conf (Set up the player connection adds it). Leave empty to not look for mpv."},
		{Key: "vlcPassword", Type: "secret", Label: "VLC web interface password", Help: "The password of VLC's web interface (Preferences > All > Interface > Main interfaces > Lua). Set up the player connection switches the interface on and fills this in. Leave empty to not look for VLC."},
		{Key: "vlcPort", Type: "int", Label: "VLC web interface port", Help: "VLC uses 8080 unless you changed it.", Min: ip(1), Max: ip(65535)},
		{Key: "pollInterval", Type: "int", Label: "Check every (ms)", Min: ip(1000), Max: ip(60000)},
		{Key: "clientId", Type: "text", Label: "Discord application ID", Help: "Make your own at discord.com/developers/applications to change the name and artwork Discord shows."},
		{Key: "artworkAliases", Type: "map", Label: "Search under another name", Help: "One per line:  filename title = catalog title"},
		{Key: "artworkOverrides", Type: "map", Label: "Pin a cover image", Help: "One per line:  title = https://example.com/poster.jpg"},
	}},
}

// AllFields lists every field in window order.
func AllFields() []Field {
	var out []Field
	for _, s := range Sections {
		out = append(out, s.Fields...)
	}
	return out
}

// ByKey finds a setting by its key (headings are labels, not settings, and are never found).
func ByKey(key string) (Field, bool) {
	for _, f := range AllFields() {
		if f.Key == key && f.Type != "heading" {
			return f, true
		}
	}
	return Field{}, false
}

// ValidationError is a problem with what the user typed (shown in the window).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// IsValidation lets the HTTP layer answer 400.
func (e *ValidationError) IsValidation() bool { return true }

// jsNumber is Number(v).
func jsNumber(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case float64:
		return x
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		low := strings.ToLower(s)
		switch {
		case strings.HasPrefix(low, "0x"), strings.HasPrefix(low, "0b"), strings.HasPrefix(low, "0o"):
			n, err := strconv.ParseInt(low, 0, 64)
			if err != nil {
				return math.NaN()
			}
			return float64(n)
		case s == "Infinity" || s == "+Infinity":
			return math.Inf(1)
		case s == "-Infinity":
			return math.Inf(-1)
		}
		if strings.ContainsAny(low, "nxp_") || strings.HasPrefix(low, "inf") || strings.HasPrefix(low, "+inf") || strings.HasPrefix(low, "-inf") {
			return math.NaN()
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	}
	return math.NaN()
}

func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return jsonx.NumStr(x)
	}
	return fmt.Sprint(v)
}

// limits for list settings (one entry per line)
const (
	maxLines   = 100
	maxLineLen = 300
)

var (
	reClientID = jsre.MustCompile(`^\d{15,22}$`, "")
	reNewline  = jsre.MustCompile(`\r?\n`, "")
)

// jsTrim is String.prototype.trim.
func jsTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r', 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
			return true
		}
		return r >= 0x2000 && r <= 0x200a
	})
}

// Clean validates one value from the window and returns what is stored (bool, int, string or an ordered map).
func Clean(f Field, v any) (any, error) {
	switch f.Type {
	case "bool":
		if b, ok := v.(bool); ok {
			return b, nil
		}
		if s, ok := v.(string); ok {
			return s == "true", nil
		}
		if n, ok := v.(float64); ok {
			return n == 1, nil
		}
		return false, nil
	case "int":
		n := math.Floor(jsNumber(v) + 0.5)
		if math.IsNaN(n) || math.IsInf(n, 0) || n < float64(*f.Min) || n > float64(*f.Max) {
			return nil, &ValidationError{fmt.Sprintf("%s: enter a number from %d to %d", f.Label, *f.Min, *f.Max)}
		}
		return int(n), nil
	case "choice":
		if s, ok := v.(string); ok {
			for _, o := range f.Options {
				if o[0] == s {
					return s, nil
				}
			}
		}
		return nil, &ValidationError{f.Label + ": unknown choice"}
	case "text", "secret":
		s := jsTrim(jsString(v))
		if f.Key == "clientId" && !reClientID.Test(s) {
			return nil, &ValidationError{"Discord application ID must be a long number"}
		}
		return s, nil
	case "lines":
		// one entry per line, from the window (a string or an array of strings): trimmed, without blanks or duplicates
		var items []string
		switch x := v.(type) {
		case nil:
		case string:
			items = reNewline.Split(x)
		case []any:
			for _, e := range x {
				s, ok := e.(string)
				if !ok {
					return nil, &ValidationError{f.Label + ": every entry must be text"}
				}
				items = append(items, s)
			}
		default:
			return nil, &ValidationError{f.Label + ": expected a list of lines"}
		}
		out := []any{}
		seen := map[string]bool{}
		for _, s := range items {
			s = jsTrim(s)
			if s == "" || seen[s] {
				continue
			}
			if len([]rune(s)) > maxLineLen {
				return nil, &ValidationError{fmt.Sprintf("%s: an entry is longer than %d characters", f.Label, maxLineLen)}
			}
			seen[s] = true
			out = append(out, s)
		}
		if len(out) > maxLines {
			return nil, &ValidationError{fmt.Sprintf("%s: at most %d entries", f.Label, maxLines)}
		}
		return out, nil
	case "sources":
		// the list of services that are OFF, from the window (an array of ids); unknown ids are refused
		out := []any{}
		seen := map[string]bool{}
		if arr, ok := v.([]any); ok {
			for _, x := range arr {
				id, _ := x.(string)
				if !core.IsSourceID(id) {
					return nil, &ValidationError{f.Label + ": unknown service " + jsString(x)}
				}
				seen[id] = true
			}
		} else if v != nil {
			return nil, &ValidationError{f.Label + ": expected a list"}
		}
		for _, s := range core.Sources { // keep the catalog's order, without duplicates
			if seen[s.ID] {
				out = append(out, s.ID)
			}
		}
		return out, nil
	case "map":
		out := jsonx.NewObj()
		var lines []string
		if s, ok := v.(string); ok {
			lines = reNewline.Split(s)
		}
		for _, line := range lines {
			i := strings.Index(line, "=")
			if i < 0 {
				if jsTrim(line) != "" {
					return nil, &ValidationError{fmt.Sprintf(`%s: "%s" needs an "=" in the middle`, f.Label, jsTrim(line))}
				}
				continue
			}
			k := jsre.ToLower(jsTrim(line[:i]))
			val := jsTrim(line[i+1:])
			if k != "" && val != "" {
				out.Set(k, val)
			}
		}
		return out, nil
	}
	return nil, &ValidationError{"bad field"}
}

// MapToText shows a map setting one "name = value" per line.
func MapToText(o *jsonx.Obj) string {
	if o == nil {
		return ""
	}
	parts := make([]string, 0, len(o.Keys))
	for _, k := range o.Keys {
		parts = append(parts, k+" = "+jsString(o.M[k]))
	}
	return strings.Join(parts, "\n")
}

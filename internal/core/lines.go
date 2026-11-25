package core

import (
	"regexp"
	"strings"
)

// Text lines (Look > Text lines): the two text lines of the status, and a film's cover text, written as a line with
// placeholders. "standard" is not a template: it is BuildActivity's own code path, which the recorded golden data
// pins. The page draws its sample with the same rules (MPRP_LINES in ui.js; testdata/lines.json holds the cases both
// must agree on).

// LineSep separates the parts of a line; a part that ends up empty is dropped with its separator.
const LineSep = " · "

// lineSet is one layout: an episode's two lines, or a film's two lines and its cover text ("" = today's).
type lineSet struct{ details, state, cover string }

// DefaultCoverLine is a film's cover text in every layout, and what an empty custom cover text falls back to.
const DefaultCoverLine = "{title}[ ({year})]"

// linePresets are Short and Detailed: episode first and second line, film first and second line.
var linePresets = map[string][4]string{
	"short":    {"{code}" + LineSep + "{episodeTitle}", "", "{year}", "{director}"},
	"detailed": {"{code}" + LineSep + "{episodeTitle}", "{genres}" + LineSep + "{rating}" + LineSep + "[on {player}]", "{year}" + LineSep + "{genres}" + LineSep + "{length}", "{rating}" + LineSep + "{director}" + LineSep + "[on {player}]"},
}

// CustomLineKeys are the keys of customLines.
var CustomLineKeys = []string{"episodeDetails", "episodeState", "movieDetails", "movieState", "movieCover"}

// linesFor is the layout textLines asks for, or nil for Standard (and for a value this version does not know).
func linesFor(cfg *Config, episode bool) *lineSet {
	var l [4]string
	cover := DefaultCoverLine
	switch cfg.TextLines {
	case "short", "detailed":
		l = linePresets[cfg.TextLines]
	case "custom":
		c := cfg.CustomLines
		l = [4]string{c["episodeDetails"], c["episodeState"], c["movieDetails"], c["movieState"]}
		if strings.TrimSpace(c["movieCover"]) != "" {
			cover = c["movieCover"]
		}
	default:
		return nil
	}
	if episode {
		return &lineSet{details: l[0], state: l[1]}
	}
	return &lineSet{details: l[2], state: l[3], cover: cover}
}

var (
	rePlaceholder = regexp.MustCompile(`\{(\w+)\}`)
	reSection     = regexp.MustCompile(`\[([^\[\]]*)\]`)
)

// Expand fills a line: each known {name} becomes its value (an unknown name stays as typed); a part in [ ] drops out
// whole when a placeholder in it is empty (its brackets go either way); the line is split at " · " and empty parts are
// dropped; then it is trimmed and cut to Discord's 128 characters (125 and "...").
func Expand(tpl string, vals map[string]string) string {
	s := reSection.ReplaceAllStringFunc(tpl, func(m string) string {
		inner := m[1 : len(m)-1]
		for _, p := range rePlaceholder.FindAllStringSubmatch(inner, -1) {
			if v, ok := vals[p[1]]; ok && v == "" {
				return ""
			}
		}
		return inner
	})
	s = rePlaceholder.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := vals[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
	var keep []string
	for _, p := range strings.Split(s, LineSep) {
		if p = trim(p); p != "" {
			keep = append(keep, p)
		}
	}
	s = trim(strings.Join(keep, LineSep))
	if u16len(s) > 128 {
		s = u16slice(s, 0, 125) + "..."
	}
	return s
}

// lineValues are the placeholders' values for one status, from what BuildActivity already worked out. An episode
// knows every name (empty where it has no value); a film has no episode names, so {code} stays as typed there.
func lineValues(info *Info, media *Media, cfg *Config, art *Art, title string, effSeason int) map[string]string {
	genres, rating, director := richParts(media, cfg, art)
	length := ""
	if info.Duration > 0 {
		length = fmtTime(info.Duration)
	}
	v := map[string]string{"title": title, "genres": genres, "rating": rating, "director": director, "player": cfg.AppName, "length": length}
	if !media.IsEpisode {
		v["year"] = filmYear(media, art)
		return v
	}
	v["year"] = ""
	if truthy(media.Year) {
		v["year"] = itoa(media.Year)
	}
	p := episodeParts(media, art, title, effSeason)
	v["code"], v["episodeTitle"], v["seasonName"] = p.label, p.epTitle, p.seasonName
	v["season"], v["episode"] = "", ""
	if effSeason != NA {
		v["season"] = itoa(effSeason)
	}
	if n := badgeEpisode(media, art); n != NA {
		v["episode"] = itoa(n)
	}
	return v
}

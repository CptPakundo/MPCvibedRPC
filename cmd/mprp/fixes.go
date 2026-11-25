package main

import (
	"strconv"
	"strings"

	"github.com/CptPakundo/MPRP/internal/core"
	"github.com/CptPakundo/MPRP/internal/jsonx"
	"github.com/CptPakundo/MPRP/internal/server"
)

// The window's Fix this match dialog: its search as you type, the other catalogs' covers, and the text of "Tell the
// project about this". Each answer carries the request number n the window sent, so the window can drop an answer
// to an older keystroke.

func (a *app) matchHandlers(h map[string]server.Handler) {
	h["match-search"] = server.Handler{Fn: func(body *jsonx.Obj) (any, error) {
		q, _ := body.M["q"].(string)
		list, catalogs, err := a.eng.MatchSearch(q)
		out := map[string]any{"n": body.M["n"], "results": list, "catalogs": catalogs}
		if list == nil {
			out["results"] = []any{}
		}
		if catalogs == nil {
			out["catalogs"] = []string{}
		}
		if err != nil {
			out["error"] = "A catalog did not answer (" + err.Error() + ")."
		}
		return out, nil
	}}
	h["match-covers"] = server.Handler{Fn: func(body *jsonx.Obj) (any, error) {
		name, _ := body.M["name"].(string)
		kind, _ := body.M["kind"].(string)
		except, _ := body.M["catalog"].(string)
		year := 0
		if f, ok := body.M["year"].(float64); ok {
			year = int(f)
		}
		covers := a.eng.MatchCovers(name, year, kind, except)
		out := map[string]any{"n": body.M["n"], "covers": covers}
		if covers == nil {
			out["covers"] = []any{}
		}
		return out, nil
	}}
	h["match-report"] = server.Handler{Fn: func(body *jsonx.Obj) (any, error) {
		return map[string]any{"text": fixReport(body, a.eng.Config(), version)}, nil
	}}
}

// fixReport is the text of "Tell the project about this": the file's name (never its folder), what it was read as,
// what it matched by itself, the fix, the catalogs that are on and the version. The window shows it to be looked over
// and posted by the user; the program sends nothing.
func fixReport(body *jsonx.Obj, cfg core.Config, version string) string {
	text := func(o *jsonx.Obj, k string) string {
		if o == nil {
			return ""
		}
		s, _ := o.M[k].(string)
		return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }), " ")
	}
	num := func(k string) int {
		f, _ := body.M[k].(float64)
		return int(f)
	}
	fx, _ := body.M["fix"].(*jsonx.Obj)
	var lines []string
	file := text(body, "file")
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		file = file[i+1:] // a name only, never a folder
	}
	if file == "" {
		file = "(none: the fix was made for a title that was not playing)"
	}
	lines = append(lines, "File name: "+file)
	read := text(body, "title")
	if read == "" {
		read = text(fx, "match")
	}
	if s := num("season"); s > 0 {
		read += ", season " + strconv.Itoa(s)
	}
	if e := num("episode"); e > 0 {
		read += ", episode " + strconv.Itoa(e)
	}
	lines = append(lines, "Read as: "+read)
	before := text(body, "before")
	if before == "" {
		before = "nothing"
	}
	lines = append(lines, "Matched by itself: "+before)
	var what []string
	switch {
	case text(fx, "entry") != "":
		what = append(what, strings.TrimSpace(text(fx, "label")+" ("+text(fx, "entry")+")"))
	case text(fx, "search") != "":
		what = append(what, `searched as "`+text(fx, "search")+`"`)
	}
	if text(fx, "cover") != "" {
		what = append(what, "their own cover image")
	}
	so, eo := 0, 0
	if fx != nil {
		f1, _ := fx.M["seasonOffset"].(float64)
		f2, _ := fx.M["episodeOffset"].(float64)
		so, eo = int(f1), int(f2)
	}
	if o := core.OffsetText(so, eo); o != "" {
		what = append(what, o)
	}
	if onlyFile := fx != nil && fx.M["file"] == true; onlyFile {
		what = append(what, "for this file only")
	} else {
		what = append(what, `for every file read as "`+text(fx, "match")+`"`)
	}
	lines = append(lines, "The fix: "+strings.Join(what, "; "))
	var on []string
	if cfg.ShowArtwork {
		for _, s := range core.Sources {
			if !cfg.SourceOn(s.ID) || s.ID == "jikan" || s.ID == "bulbapedia" {
				continue
			}
			l := s.Label
			if s.ID == "tmdb" && cfg.TmdbAPIKey == "" {
				l += " (no key)"
			}
			on = append(on, l)
		}
	}
	if len(on) == 0 {
		on = []string{"none"}
	}
	lines = append(lines, "Catalogs on: "+strings.Join(on, ", "))
	lines = append(lines, "Version: "+version)
	return strings.Join(lines, "\n")
}

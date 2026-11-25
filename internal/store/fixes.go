package store

import (
	"fmt"
	"math"
	"strings"

	"github.com/CptPakundo/MPRP/internal/core"
	"github.com/CptPakundo/MPRP/internal/jsonx"
)

// limits for Fix a match and the custom text lines
const (
	maxFixes     = 500
	maxOffset    = 500
	maxCoverLen  = 256 // the cover finder takes image addresses up to this length
	maxLineChars = 300
)

// cleanFixes validates Fix a match's list from the window (see core.MatchFix): each fix needs the title (or file name)
// it is for, and at most one of a catalog page or another name to search; the cover is an http(s) image address. A
// second fix for the same title (or file) replaces the first. Stored in a fixed key order, without empty fields.
func cleanFixes(f Field, v any) (any, error) {
	var arr []any
	switch x := v.(type) {
	case nil:
	case []any:
		arr = x
	default:
		return nil, &ValidationError{f.Label + ": expected a list of fixes"}
	}
	bad := func(msg string) error { return &ValidationError{f.Label + ": " + msg} }
	var out []*jsonx.Obj
	at := map[string]int{}
	for _, e := range arr {
		o, ok := e.(*jsonx.Obj)
		if !ok {
			return nil, bad("every fix must be an object")
		}
		text := func(key string, max int) (string, error) {
			raw, present := o.M[key]
			if !present || raw == nil {
				return "", nil
			}
			s, ok := raw.(string)
			if !ok {
				return "", bad(key + " must be text")
			}
			s = jsTrim(s)
			if hasControl(s) {
				return "", bad(key + " must not contain control characters")
			}
			if len([]rune(s)) > max {
				return "", bad(fmt.Sprintf("%s is longer than %d characters", key, max))
			}
			return s, nil
		}
		address := func(key string, max int) (string, error) {
			s, err := text(key, max)
			if err == nil && s != "" && !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "http://") {
				err = bad(key + " must be an http(s) address")
			}
			return s, err
		}
		offset := func(key string) (int, error) {
			raw, present := o.M[key]
			if !present || raw == nil || isBlank(raw) {
				return 0, nil
			}
			n := jsNumber(raw)
			if math.IsNaN(n) || n != math.Trunc(n) || math.Abs(n) > maxOffset {
				return 0, bad(fmt.Sprintf("an offset must be a whole number from -%d to %d", maxOffset, maxOffset))
			}
			return int(n), nil
		}
		var fx core.MatchFix
		var err error
		if fx.Match, err = text("match", maxLineLen); err != nil {
			return nil, err
		}
		if fx.Match == "" {
			return nil, bad("every fix needs the title or file name it is for")
		}
		fx.File, _ = o.M["file"].(bool)
		if fx.Entry, err = address("entry", 512); err != nil {
			return nil, err
		}
		if _, ok := core.EntryPage(fx.Entry); fx.Entry != "" && !ok {
			return nil, bad("the pinned entry must be an IMDb, TMDB, TVmaze, AniList or Kitsu page")
		}
		if fx.Search, err = text("search", maxLineLen); err != nil {
			return nil, err
		}
		if fx.Entry != "" {
			fx.Search = "" // a pinned entry needs no other name
		}
		if fx.Cover, err = address("cover", maxCoverLen); err != nil {
			return nil, err
		}
		if fx.SeasonOffset, err = offset("seasonOffset"); err != nil {
			return nil, err
		}
		if fx.EpisodeOffset, err = offset("episodeOffset"); err != nil {
			return nil, err
		}
		if fx.Label, err = text("label", maxLineLen); err != nil {
			return nil, err
		}
		if fx.Poster, err = address("poster", 512); err != nil {
			return nil, err
		}
		key := fmt.Sprint(fx.File, "|", strings.ToLower(fx.Match))
		if i, dup := at[key]; dup {
			out[i] = fixObj(fx)
			continue
		}
		at[key] = len(out)
		out = append(out, fixObj(fx))
	}
	if len(out) > maxFixes {
		return nil, bad(fmt.Sprintf("at most %d fixes", maxFixes))
	}
	list := make([]any, len(out))
	for i, o := range out {
		list[i] = o
	}
	return list, nil
}

// fixObj is a fix as config.json stores it.
func fixObj(fx core.MatchFix) *jsonx.Obj {
	o := jsonx.NewObj()
	o.Set("match", fx.Match)
	if fx.File {
		o.Set("file", true)
	}
	for _, kv := range [][2]string{{"entry", fx.Entry}, {"search", fx.Search}, {"cover", fx.Cover}} {
		if kv[1] != "" {
			o.Set(kv[0], kv[1])
		}
	}
	if fx.SeasonOffset != 0 {
		o.Set("seasonOffset", float64(fx.SeasonOffset))
	}
	if fx.EpisodeOffset != 0 {
		o.Set("episodeOffset", float64(fx.EpisodeOffset))
	}
	if fx.Label != "" {
		o.Set("label", fx.Label)
	}
	if fx.Poster != "" {
		o.Set("poster", fx.Poster)
	}
	return o
}

// cleanTextLines validates the custom text lines: an object with the keys of core.CustomLineKeys (others are refused),
// each a line of text. Empty lines are left out (a missing line is an empty one).
func cleanTextLines(f Field, v any) (any, error) {
	out := jsonx.NewObj()
	if v == nil {
		return out, nil
	}
	o, ok := v.(*jsonx.Obj)
	if !ok {
		return nil, &ValidationError{f.Label + ": expected an object"}
	}
	known := map[string]bool{}
	for _, k := range core.CustomLineKeys {
		known[k] = true
	}
	for _, k := range o.Keys {
		if !known[k] {
			return nil, &ValidationError{f.Label + ": unknown line " + k}
		}
	}
	for _, k := range core.CustomLineKeys {
		raw, present := o.M[k]
		if !present || raw == nil {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			return nil, &ValidationError{f.Label + ": every line must be text"}
		}
		s = jsTrim(s)
		if hasControl(s) {
			return nil, &ValidationError{f.Label + ": a line must not contain line breaks or other control characters"}
		}
		if len([]rune(s)) > maxLineChars {
			return nil, &ValidationError{fmt.Sprintf("%s: a line is longer than %d characters", f.Label, maxLineChars)}
		}
		if s != "" {
			out.Set(k, s)
		}
	}
	return out, nil
}

// MigrateMatchFixes writes the legacy Fix a match lines (artworkAliases, artworkOverrides) as matchFixes, once: only
// while config.json has no matchFixes key. Each title becomes one fix (the alias as its search, the pinned cover as
// its cover), in the file's order. The old keys stay in the file, for a downgrade, and are not read again. Each fix
// is made to pass the window's rules first (fitFix), since the window saves the whole list: a fix it refuses would make
// every later save fail. It returns how many fixes were written, and whether the matchFixes key was written (also
// with no fix in it, when every old line was left out): then the program's fixes have changed.
func (s *Store) MigrateMatchFixes() (n int, wrote bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.read()
	if err != nil {
		return 0, false, err
	}
	if _, has := cur.M["matchFixes"]; has {
		return 0, false, nil
	}
	var order []string
	fixes := map[string]*core.MatchFix{}
	for _, src := range []string{"artworkAliases", "artworkOverrides"} {
		m, ok := cur.M[src].(*jsonx.Obj)
		if !ok {
			continue
		}
		for _, k := range m.Keys {
			val, _ := m.M[k].(string)
			if jsTrim(k) == "" || val == "" {
				continue
			}
			fx := fixes[k]
			if fx == nil {
				fx = &core.MatchFix{Match: k}
				fixes[k] = fx
				order = append(order, k)
			}
			if src == "artworkAliases" {
				fx.Search = val
			} else {
				fx.Cover = val
			}
		}
	}
	if len(order) == 0 {
		return 0, false, nil
	}
	var list []any
	changed, skipped := 0, 0
	for _, k := range order {
		fx, ok, ch := fitFix(*fixes[k])
		switch {
		case !ok || len(list) == maxFixes:
			skipped++
			continue
		case ch:
			changed++
		}
		list = append(list, fixObj(fx))
	}
	if changed+skipped > 0 {
		s.logf("WARN", fmt.Sprintf("Fix a match: %d of the old artworkAliases and artworkOverrides titles were changed to fit the rules "+
			"(a cover that is not an http(s) address left out, text over its limit shortened) and %d were left out.", changed, skipped))
	}
	// an empty list is written too: the old lines, which the window could not save, are then no longer read
	if list == nil {
		list = []any{}
	}
	cur.Set("matchFixes", list)
	if err := s.write(cur); err != nil {
		return 0, false, err
	}
	return len(list), true, nil
}

// fitFix makes a fix carried over from the old Fix a match lines one that cleanFixes takes: the text trimmed, text
// over its limit shortened, a field that still cannot pass (control characters; a cover that is not an http(s)
// address or is too long, which could never show on Discord anyway) left out. ok is false when nothing is left to
// act on (no title, or neither another name nor a cover); changed says that a field had to be changed beyond the
// trimming.
func fitFix(in core.MatchFix) (fx core.MatchFix, ok, changed bool) {
	text := func(s string, max int) string {
		s = jsTrim(s)
		if hasControl(s) {
			changed = true
			return ""
		}
		if r := []rune(s); len(r) > max {
			changed = true
			s = jsTrim(string(r[:max]))
		}
		return s
	}
	fx.Match = text(in.Match, maxLineLen)
	fx.Search = text(in.Search, maxLineLen)
	if c := jsTrim(in.Cover); c != "" {
		if hasControl(c) || len([]rune(c)) > maxCoverLen || !(strings.HasPrefix(c, "https://") || strings.HasPrefix(c, "http://")) {
			changed = true
		} else {
			fx.Cover = c
		}
	}
	return fx, fx.Match != "" && (fx.Search != "" || fx.Cover != ""), changed
}

package core

import "strings"

// ---- privacy mode: files that are never shown, and hiding the title ----------------------------------------

// MatchesHideList reports whether a playing file is on the user's "don't show" list. Each entry is matched without
// regard to case against the whole path (folders and file name, with / and \ treated alike) and means "the path
// contains this text". An entry with * or ? is a wildcard instead: it has to match the whole file name or the whole
// path, where * stands for any run of characters and ? for any one character.
func MatchesHideList(file, fullDir string, entries []string) bool {
	if file == "" || len(entries) == 0 {
		return false
	}
	name := normHidePath(file)
	full := name
	if dir := normHidePath(fullDir); dir != "" {
		full = strings.TrimRight(dir, "/") + "/" + name
	}
	for _, e := range entries {
		p := normHidePath(strings.TrimSpace(e))
		if p == "" {
			continue
		}
		if strings.ContainsAny(p, "*?") {
			if wildMatch(p, name) || wildMatch(p, full) {
				return true
			}
			continue
		}
		if strings.Contains(full, p) {
			return true
		}
	}
	return false
}

func normHidePath(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, `\`, "/"))
}

// wildMatch matches pattern against s where * is any run of characters (including none) and ? any single character.
func wildMatch(pattern, s string) bool {
	p, t := []rune(pattern), []rune(s)
	pi, ti, star, mark := 0, 0, -1, 0
	for ti < len(t) {
		switch {
		case pi < len(p) && (p[pi] == '?' || p[pi] == t[ti]):
			pi++
			ti++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, ti
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			ti = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// HiddenActivity is the status shown when the title is hidden: no name, folder, cover, details or buttons, only that
// something is being watched, and where it is (the progress bar, or the pause text).
func HiddenActivity(info *Info, cfg *Config, now int64) *Activity {
	if info == nil {
		return nil
	}
	c := *cfg
	c.TitleAsName, c.RichInfo, c.LinkButton, c.EpisodeInDetails, c.CatalogTitle, c.ShowArtwork = false, false, false, false, false, false
	plain := &Info{File: "video", State: info.State, Position: info.Position, Duration: info.Duration, Rate: info.Rate}
	a := BuildActivity(plain, &c, now, nil, nil)
	if a == nil {
		return nil
	}
	a.Details = "Watching a video"
	a.Buttons = nil
	a.Assets = nil
	if cfg.LargeImageKey != "" {
		a.Assets = &Assets{LargeImage: cfg.LargeImageKey, LargeText: cfg.AppName}
	}
	return a
}
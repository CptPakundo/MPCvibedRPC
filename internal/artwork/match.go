// Package artwork finds cover art, catalog names and episode titles for the file being watched.
package artwork

import (
	"math"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

func re(p, flags string) *jsre.Regexp { return jsre.MustCompile(p, flags) }

func u16len(s string) int { return core.U16Len(s) }

// ---- title matching ------------------------------------------------------------------

var (
	reMarks      = re(`\p{M}`, "gu")
	reAmpPlus    = re(`[&+]`, "g")
	reArticle    = re(`^\s*(?:the|a|an)\s+`, "")
	reOrdNum     = re(`\b(\d{1,2})(?:st|nd|rd|th)\b`, "g")
	reOrdWord    = re(`\b(first|second|third|fourth|fifth|sixth|seventh|eighth|ninth|tenth)\b`, "g")
	reNonAN      = re(`[^\p{L}\p{N}]+`, "gu")
	reDiacr      = re(`[\u0300-\u036f]`, "g")
	reDigits     = re(`\d+`, "g")
	reColonSplit = re(`:|\s[-–]\s`, "")
	reHasSub     = re(`^[^:]*:|\s[-–]\s`, "")
	reArt1       = re(`^(?:the|a|an)$`, "")
)

var ordinals = map[string]int{"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10}

func stripMarks(s string) string { return reMarks.ReplaceStr(jsre.NFD(s), "") }

// normTitle: lowercase, accent-insensitive, punctuation-free, keeps any script. Ordinals are unified so
// "Initial D Fourth Stage" == "Initial D 4th Stage".
func normTitle(s string) string {
	t := jsre.ToLower(stripMarks(s))
	t = reAmpPlus.ReplaceStr(t, " and ")
	t = reArticle.ReplaceStr(t, "")
	t = reOrdNum.ReplaceStr(t, "$1")
	t = reOrdWord.Replace(t, func(m *jsre.Match) string { return itoa(ordinals[m.Str(0)]) })
	return reNonAN.ReplaceStr(t, "")
}

// plain: Latin diacritics only (é -> e). Leaves Hebrew points and Japanese dakuten alone.
func plain(s string) string { return jsre.NFC(reDiacr.ReplaceStr(jsre.NFD(s), "")) }

func words(s string) []string {
	var out []string
	for _, w := range reNonAN.Split(jsre.ToLower(stripMarks(s))) {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func digitKey(s string) string {
	var parts []string
	for _, m := range reDigits.FindAll(s) {
		parts = append(parts, m.Str(0))
	}
	return strings.Join(parts, ",")
}

func units(s string) []uint16 { return utf16.Encode([]rune(s)) }

func dice(a, b string) float64 {
	if a == b {
		return 1
	}
	ua, ub := units(a), units(b)
	if len(ua) < 2 || len(ub) < 2 {
		return 0
	}
	grams := func(u []uint16) map[[2]uint16]int {
		m := map[[2]uint16]int{}
		for i := 0; i < len(u)-1; i++ {
			m[[2]uint16{u[i], u[i+1]}]++
		}
		return m
	}
	A, B := grams(ua), grams(ub)
	overlap := 0
	for g, n := range A {
		overlap += min(n, B[g])
	}
	return float64(2*overlap) / float64((len(ua)-1)+(len(ub)-1))
}

// Cand is one search result from a catalog.
type Cand struct {
	Titles   []string
	Year     int // 0 = unknown
	Poster   string
	URL      string
	URLLabel string
	Ref      string
	Kind     string
	ID       string // episode lookups: the catalog's own id
	Runtime  int
	Eps      int
	Alt      *core.ArtAlt
	AniInfo  *core.ArtInfo
	ViaAlias bool
	ViaRank  bool
	l0       string // titles[0] as the catalog gave it (IMDb alias check)
}

func (c *Cand) names() []string { return c.Titles }

func yearClose(c *Cand, year int) bool {
	return c.Year != 0 && math.Abs(float64(c.Year-year)) <= 1
}

// oneEdit: one inserted, dropped or changed letter in a longish word ("Toitsu" / "Touitsu").
func oneEdit(xs, ys string) bool {
	x, y := units(xs), units(ys)
	if len(x) < 6 || len(y) < 6 || abs(len(x)-len(y)) > 1 {
		return false
	}
	i := 0
	for i < len(x) && i < len(y) && x[i] == y[i] {
		i++
	}
	eq := func(a, b []uint16) bool {
		if len(a) != len(b) {
			return false
		}
		for k := range a {
			if a[k] != b[k] {
				return false
			}
		}
		return true
	}
	if len(x) == len(y) {
		return eq(x[min(i+1, len(x)):], y[min(i+1, len(y)):])
	}
	s, l := x, y
	if len(x) >= len(y) {
		s, l = y, x
	}
	return eq(s[min(i, len(s)):], l[min(i+1, len(l)):])
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// PickOpts are the matching options of one source.
type PickOpts struct {
	Earliest, ExactOnly, TrustAlias, Ranked bool
	Runtime                                 int
	Contain                                 []string // nil = none
}

func yr(c *Cand) int {
	if c.Year != 0 {
		return c.Year
	}
	return 9999
}

func firstSeg(n string) string { return reColonSplit.Split(n)[0] }

func anyName(c *Cand, f func(n string) bool) bool {
	for _, n := range c.Titles {
		if f(n) {
			return true
		}
	}
	return false
}

// pickMatch picks the candidate that really is `title`. Wrong posters are worse than none.
func pickMatch(cands []*Cand, title string, year int, strictYear bool, o PickOpts) *Cand {
	want := normTitle(title)
	if want == "" {
		return nil
	}
	var wantWords []string
	for i, w := range words(title) {
		if !(i == 0 && reArt1.Test(w)) {
			wantWords = append(wantWords, w)
		}
	}
	byYear := func(list []*Cand) []*Cand {
		if o.Earliest {
			cp := append([]*Cand(nil), list...)
			sort.SliceStable(cp, func(i, j int) bool { return yr(cp[i]) < yr(cp[j]) })
			return cp
		}
		return list
	}
	type timedC struct {
		c *Cand
		d float64
	}
	timedOf := func(list []*Cand) []timedC {
		var t []timedC
		for _, c := range list {
			if c.Runtime > 0 {
				d := math.Abs(float64(c.Runtime-o.Runtime)) / float64(c.Runtime)
				if d <= 0.3 {
					t = append(t, timedC{c, d})
				}
			}
		}
		sort.SliceStable(t, func(i, j int) bool { return t[i].d < t[j].d })
		return t
	}
	fit := func(list []*Cand) *Cand {
		if o.Runtime != 0 && len(list) > 1 {
			if t := timedOf(list); len(t) > 0 {
				return t[0].c
			}
		}
		return list[0]
	}
	hasAll := func(n string) bool {
		nn := normTitle(n)
		for _, x := range o.Contain {
			if !strings.Contains(nn, x) {
				return false
			}
		}
		return true
	}
	if o.Ranked && year == 0 && o.Runtime != 0 && o.Contain == nil {
		var group []*Cand
		for _, c := range cands {
			if c.Runtime > 0 && anyName(c, func(n string) bool {
				nn := normTitle(n)
				return nn == want || (strings.HasPrefix(nn, want) && normTitle(firstSeg(n)) == want)
			}) {
				group = append(group, c)
			}
		}
		near := timedOf(group)
		if len(group) > 1 && len(near) > 0 {
			cp := *near[0].c
			cp.ViaRank = true
			return &cp
		}
	}
	if o.Ranked && year == 0 && len(cands) > 0 && o.Contain == nil {
		top := cands[0]
		isExact := anyName(top, func(n string) bool { return normTitle(n) == want })
		ext := anyName(top, func(n string) bool {
			nn := normTitle(n)
			return strings.HasPrefix(nn, want) && nn != want && reHasSub.Test(n) && normTitle(firstSeg(n)) == want
		})
		if !isExact && ext {
			for _, c := range cands {
				if anyName(c, func(n string) bool { return normTitle(n) == want }) {
					cp := *top
					cp.ViaRank = true
					return &cp
				}
			}
		}
	}
	full := func(x string) string {
		t := jsre.ToLower(stripMarks(x))
		t = reAmpPlus.ReplaceStr(t, " and ")
		return reNonAN.ReplaceStr(t, "")
	}
	wantFull := full(title)
	tailRe := re(`[^\p{L}\p{N}\s)\]]+$`, "u")
	tail := func(x string) string {
		if m := tailRe.Exec(core.Trim(x)); m != nil {
			return m.Str(0)
		}
		return ""
	}
	wantTail := tail(title)
	withArticle := func(list []*Cand) []*Cand {
		out := list
		var same []*Cand
		for _, c := range out {
			if anyName(c, func(n string) bool { return full(n) == wantFull }) {
				same = append(same, c)
			}
		}
		if len(same) > 0 && len(same) < len(out) {
			out = same
		}
		var tl []*Cand
		for _, c := range out {
			if anyName(c, func(n string) bool { return normTitle(n) == want && tail(n) == wantTail }) {
				tl = append(tl, c)
			}
		}
		if len(tl) > 0 && len(tl) < len(out) {
			out = tl
		}
		return out
	}
	var exact0 []*Cand
	for _, c := range cands {
		if anyName(c, func(n string) bool { return normTitle(n) == want && (o.Contain == nil || hasAll(n)) }) {
			exact0 = append(exact0, c)
		}
	}
	exact := withArticle(exact0)
	if len(exact) > 0 {
		if year != 0 {
			var dated []*Cand
			for _, c := range exact {
				if yearClose(c, year) {
					dated = append(dated, c)
				}
			}
			if len(dated) > 0 {
				return fit(byYear(dated))
			}
			var undated []*Cand
			for _, c := range exact {
				if c.Year == 0 {
					undated = append(undated, c)
				}
			}
			if len(undated) > 0 {
				return fit(undated)
			}
			if strictYear {
				return nil
			}
		}
		return fit(byYear(exact))
	}
	if len(o.Contain) > 0 {
		var hits []*Cand
		for _, c := range cands {
			if (year == 0 || c.Year == 0 || yearClose(c, year)) && anyName(c, func(n string) bool { return hasAll(n) }) {
				hits = append(hits, c)
			}
		}
		if len(hits) > 0 {
			ln := func(c *Cand) int {
				best := math.MaxInt32
				for _, n := range c.Titles {
					nn := normTitle(n)
					ok := true
					for _, x := range o.Contain {
						if !strings.Contains(nn, x) {
							ok = false
						}
					}
					if ok && u16len(nn) < best {
						best = u16len(nn)
					}
				}
				return best
			}
			cp := append([]*Cand(nil), hits...)
			sort.SliceStable(cp, func(i, j int) bool {
				a, b := ln(cp[i]), ln(cp[j])
				if a != b {
					return a < b
				}
				return yr(cp[i]) < yr(cp[j])
			})
			return cp[0]
		}
	}
	if o.ExactOnly || o.Contain != nil {
		return nil
	}

	yearOk := func(c *Cand) bool { return year == 0 || c.Year == 0 || yearClose(c, year) }

	if u16len(want) >= 5 {
		var best *Cand
		bestScore := 0.0
		for _, c := range cands {
			if !yearOk(c) {
				continue
			}
			for _, n := range c.Titles {
				nn := normTitle(n)
				if u16len(nn) < 5 || digitKey(nn) != digitKey(want) {
					continue
				}
				score := dice(nn, want)
				a, b := words(n), words(title)
				var wordsOk bool
				if len(a) != len(b) {
					wordsOk = score >= 0.93
				} else {
					wordsOk = true
					for i, w := range a {
						if !(w == b[i] || (u16len(w) >= 4 && dice(w, b[i]) >= 0.75) || oneEdit(w, b[i])) {
							wordsOk = false
							break
						}
					}
				}
				if wordsOk && score >= 0.85 && score > bestScore {
					best, bestScore = c, score
				}
			}
		}
		if best != nil {
			return best
		}
	}

	if u16len(want) >= 7 && len(wantWords) > 0 {
		var pf []*Cand
		for _, c := range cands {
			var ok bool
			if strictYear {
				ok = year != 0 && yearClose(c, year)
			} else {
				ok = yearOk(c)
			}
			if !ok {
				continue
			}
			if anyName(c, func(n string) bool {
				var w []string
				for i, x := range words(n) {
					if !(i == 0 && reArt1.Test(x)) {
						w = append(w, x)
					}
				}
				if len(w) <= len(wantWords) {
					return false
				}
				for i, x := range wantWords {
					if w[i] != x {
						return false
					}
				}
				return true
			}) {
				pf = append(pf, c)
			}
		}
		if len(pf) > 0 {
			cp := append([]*Cand(nil), pf...)
			sort.SliceStable(cp, func(i, j int) bool { return yr(cp[i]) < yr(cp[j]) })
			return cp[0]
		}
	}

	if u16len(want) >= 8 {
		var subs []*Cand
		for _, c := range cands {
			var ok bool
			if strictYear {
				ok = year == 0 || yearClose(c, year)
			} else {
				ok = yearOk(c)
			}
			if !ok {
				continue
			}
			if anyName(c, func(n string) bool {
				i := strings.Index(n, ":")
				return i > 0 && normTitle(n[i+1:]) == want
			}) {
				subs = append(subs, c)
			}
		}
		if len(subs) > 0 {
			cp := append([]*Cand(nil), subs...)
			sort.SliceStable(cp, func(i, j int) bool { return yr(cp[i]) < yr(cp[j]) })
			return cp[0]
		}
	}

	if len(wantWords) >= 3 {
		type sup struct {
			c     *Cand
			extra int
		}
		var sups []sup
		for _, c := range cands {
			if strictYear {
				if year != 0 && !yearClose(c, year) {
					continue
				}
			} else if !yearOk(c) {
				continue
			}
			for _, n := range c.Titles {
				have := words(n)
				set := map[string]bool{}
				for _, w := range have {
					set[w] = true
				}
				extra := len(have) - len(wantWords)
				if extra >= 1 && extra <= 3 {
					all := true
					for _, x := range wantWords {
						if !set[x] {
							all = false
							break
						}
					}
					if all && digitKey(normTitle(n)) == digitKey(want) {
						sups = append(sups, sup{c, extra})
						break
					}
				}
			}
		}
		if len(sups) > 0 {
			sort.SliceStable(sups, func(i, j int) bool { return sups[i].extra < sups[j].extra })
			return sups[0].c
		}
	}

	if o.TrustAlias && u16len(want) >= 5 {
		for _, c := range cands {
			if c.ViaAlias && yearOk(c) {
				return c
			}
		}
	}
	return nil
}

func itoa(n int) string { return core.Itoa(n) }

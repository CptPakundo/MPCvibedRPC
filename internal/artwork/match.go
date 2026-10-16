// Package artwork finds cover art, catalog names and episode titles for the file being watched.
package artwork

import (
	"math"
	"sort"
	"strconv"
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
	reDigits     = re(`\d+`, "g")
	reColonSplit = re(`:|\s[-–]\s`, "")
	reHasSub     = re(`^[^:]*:|\s[-–]\s`, "")
	reArt1       = re(`^(?:the|a|an)$`, "")
	reTrailPunct = re(`[^\p{L}\p{N}\s)\]]+$`, "u")
)

var ordinals = map[string]int{"first": 1, "second": 2, "third": 3, "fourth": 4, "fifth": 5, "sixth": 6, "seventh": 7, "eighth": 8, "ninth": 9, "tenth": 10}

func stripMarks(s string) string { return reMarks.ReplaceStr(jsre.NFD(s), "") }

// normTitle: lowercase, accent-insensitive, punctuation-free, keeps any script. Ordinals are unified so
// "Show Fourth Stage" == "Show 4th Stage".
func normTitle(s string) string {
	t := jsre.ToLower(stripMarks(s))
	t = reAmpPlus.ReplaceStr(t, " and ")
	t = reArticle.ReplaceStr(t, "")
	t = reOrdNum.ReplaceStr(t, "$1")
	t = reOrdWord.Replace(t, func(m *jsre.Match) string { return strconv.Itoa(ordinals[m.Str(0)]) })
	return reNonAN.ReplaceStr(t, "")
}

// plain: Latin diacritics only (é -> e). Leaves Hebrew points and Japanese dakuten alone.
func plain(s string) string { return jsre.NFC(jsre.StripAccents(s)) }

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

func yearClose(c *Cand, year int) bool {
	return c.Year != 0 && math.Abs(float64(c.Year-year)) <= 1
}

// oneEdit: one inserted, dropped or changed letter in a longish word ("Colour" / "Color").
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

// dropArticle removes a leading "the", "a" or "an" from a list of words.
func dropArticle(ws []string) []string {
	var out []string
	for i, w := range ws {
		if !(i == 0 && reArt1.Test(w)) {
			out = append(out, w)
		}
	}
	return out
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

// filter keeps the candidates that satisfy keep, in order.
func filter(cands []*Cand, keep func(*Cand) bool) []*Cand {
	var out []*Cand
	for _, c := range cands {
		if keep(c) {
			out = append(out, c)
		}
	}
	return out
}

// earliest is the candidate with the lowest year (the first of equals); nil for an empty list.
func earliest(list []*Cand) *Cand {
	var best *Cand
	for _, c := range list {
		if best == nil || yr(c) < yr(best) {
			best = c
		}
	}
	return best
}

// normFull is normTitle without the ordinal and article rules.
func normFull(x string) string {
	t := jsre.ToLower(stripMarks(x))
	t = reAmpPlus.ReplaceStr(t, " and ")
	return reNonAN.ReplaceStr(t, "")
}

// trailOf is the punctuation a title ends with ("!" in "Show!").
func trailOf(x string) string {
	if m := reTrailPunct.Exec(core.Trim(x)); m != nil {
		return m.Str(0)
	}
	return ""
}

// matcher is one pickMatch call: the wanted title and the tiers that try to find it among the candidates.
type matcher struct {
	cands      []*Cand
	title      string
	want       string // normTitle(title)
	wantWords  []string
	wantFull   string
	wantTail   string
	year       int
	strictYear bool
	o          PickOpts
}

// pickMatch picks the candidate that really is `title`. Wrong posters are worse than none. The tiers run from the
// most to the least certain; the first that finds a candidate decides.
func pickMatch(cands []*Cand, title string, year int, strictYear bool, o PickOpts) *Cand {
	want := normTitle(title)
	if want == "" {
		return nil
	}
	m := &matcher{cands: cands, title: title, want: want, wantWords: dropArticle(words(title)), wantFull: normFull(title),
		wantTail: trailOf(title), year: year, strictYear: strictYear, o: o}
	if o.Ranked && year == 0 && o.Runtime != 0 && o.Contain == nil {
		if c := m.rankedByRuntime(); c != nil {
			return c
		}
	}
	if o.Ranked && year == 0 && len(cands) > 0 && o.Contain == nil {
		if c := m.rankedTop(); c != nil {
			return c
		}
	}
	if c, found := m.exact(); found {
		return c
	}
	if len(o.Contain) > 0 {
		if c := m.contain(); c != nil {
			return c
		}
	}
	if o.ExactOnly || o.Contain != nil {
		return nil
	}
	if u16len(want) >= 5 {
		if c := m.spelling(); c != nil {
			return c
		}
	}
	if u16len(want) >= 7 && len(m.wantWords) > 0 {
		if c := m.prefix(); c != nil {
			return c
		}
	}
	if u16len(want) >= 8 {
		if c := m.subtitle(); c != nil {
			return c
		}
	}
	if len(m.wantWords) >= 3 {
		if c := m.superset(); c != nil {
			return c
		}
	}
	if o.TrustAlias && u16len(want) >= 5 {
		return m.alias()
	}
	return nil
}

// yearOK: the candidate's year is unknown or within a year of the wanted one.
func (m *matcher) yearOK(c *Cand) bool { return m.year == 0 || c.Year == 0 || yearClose(c, m.year) }

// yearGate is yearOK, but with strictYear a candidate must be dated close to the wanted year (needYear), or any
// year is fine when none is wanted.
func (m *matcher) yearGate(c *Cand, needYear bool) bool {
	if !m.strictYear {
		return m.yearOK(c)
	}
	if needYear {
		return m.year != 0 && yearClose(c, m.year)
	}
	return m.year == 0 || yearClose(c, m.year)
}

// hasAll: the name contains every word of o.Contain.
func (m *matcher) hasAll(n string) bool {
	nn := normTitle(n)
	for _, x := range m.o.Contain {
		if !strings.Contains(nn, x) {
			return false
		}
	}
	return true
}

// byYear is list ordered by year when o.Earliest is set (the first of equals stays first), else list itself.
func (m *matcher) byYear(list []*Cand) []*Cand {
	if m.o.Earliest {
		cp := append([]*Cand(nil), list...)
		sort.SliceStable(cp, func(i, j int) bool { return yr(cp[i]) < yr(cp[j]) })
		return cp
	}
	return list
}

type timedCand struct {
	c *Cand
	d float64
}

// timedOf lists the candidates whose runtime is within 30% of o.Runtime, closest first.
func (m *matcher) timedOf(list []*Cand) []timedCand {
	var t []timedCand
	for _, c := range list {
		if c.Runtime > 0 {
			d := math.Abs(float64(c.Runtime-m.o.Runtime)) / float64(c.Runtime)
			if d <= 0.3 {
				t = append(t, timedCand{c, d})
			}
		}
	}
	sort.SliceStable(t, func(i, j int) bool { return t[i].d < t[j].d })
	return t
}

// fit is the candidate whose runtime fits best, or the first.
func (m *matcher) fit(list []*Cand) *Cand {
	if m.o.Runtime != 0 && len(list) > 1 {
		if t := m.timedOf(list); len(t) > 0 {
			return t[0].c
		}
	}
	return list[0]
}

// rankedByRuntime: several results carry the title itself (or the title before a subtitle) and one of them has the
// wanted runtime.
func (m *matcher) rankedByRuntime() *Cand {
	group := filter(m.cands, func(c *Cand) bool {
		return c.Runtime > 0 && anyName(c, func(n string) bool {
			nn := normTitle(n)
			return nn == m.want || (strings.HasPrefix(nn, m.want) && normTitle(firstSeg(n)) == m.want)
		})
	})
	near := m.timedOf(group)
	if len(group) > 1 && len(near) > 0 {
		cp := *near[0].c
		cp.ViaRank = true
		return &cp
	}
	return nil
}

// rankedTop: the best-ranked result is the title with a subtitle while another result is the title itself, so the
// ranking is trusted.
func (m *matcher) rankedTop() *Cand {
	top := m.cands[0]
	isExact := anyName(top, func(n string) bool { return normTitle(n) == m.want })
	ext := anyName(top, func(n string) bool {
		nn := normTitle(n)
		return strings.HasPrefix(nn, m.want) && nn != m.want && reHasSub.Test(n) && normTitle(firstSeg(n)) == m.want
	})
	if !isExact && ext {
		for _, c := range m.cands {
			if anyName(c, func(n string) bool { return normTitle(n) == m.want }) {
				cp := *top
				cp.ViaRank = true
				return &cp
			}
		}
	}
	return nil
}

// withArticle narrows candidates that all match the normalised title to those that also match with its "the",
// ampersands and trailing punctuation.
func (m *matcher) withArticle(list []*Cand) []*Cand {
	out := list
	same := filter(out, func(c *Cand) bool { return anyName(c, func(n string) bool { return normFull(n) == m.wantFull }) })
	if len(same) > 0 && len(same) < len(out) {
		out = same
	}
	tl := filter(out, func(c *Cand) bool {
		return anyName(c, func(n string) bool { return normTitle(n) == m.want && trailOf(n) == m.wantTail })
	})
	if len(tl) > 0 && len(tl) < len(out) {
		out = tl
	}
	return out
}

// exact: candidates named exactly like the title. found says there were some; the pick may still be nil (a year
// that rules them all out).
func (m *matcher) exact() (c *Cand, found bool) {
	exact := m.withArticle(filter(m.cands, func(c *Cand) bool {
		return anyName(c, func(n string) bool { return normTitle(n) == m.want && (m.o.Contain == nil || m.hasAll(n)) })
	}))
	if len(exact) == 0 {
		return nil, false
	}
	if m.year != 0 {
		if dated := filter(exact, func(c *Cand) bool { return yearClose(c, m.year) }); len(dated) > 0 {
			return m.fit(m.byYear(dated)), true
		}
		if undated := filter(exact, func(c *Cand) bool { return c.Year == 0 }); len(undated) > 0 {
			return m.fit(undated), true
		}
		if m.strictYear {
			return nil, true
		}
	}
	return m.fit(m.byYear(exact)), true
}

// contain: candidates whose name holds every word of o.Contain; the shortest name wins, then the earliest year.
func (m *matcher) contain() *Cand {
	hits := filter(m.cands, func(c *Cand) bool { return m.yearOK(c) && anyName(c, m.hasAll) })
	if len(hits) == 0 {
		return nil
	}
	ln := func(c *Cand) int {
		best := math.MaxInt32
		for _, n := range c.Titles {
			if !m.hasAll(n) {
				continue
			}
			if l := u16len(normTitle(n)); l < best {
				best = l
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

// wordsClose: every word of a is the word of b, a close spelling of it or one edit away; with a different number of
// words only the overall score counts.
func wordsClose(a, b []string, score float64) bool {
	if len(a) != len(b) {
		return score >= 0.93
	}
	for i, w := range a {
		if !(w == b[i] || (u16len(w) >= 4 && dice(w, b[i]) >= 0.75) || oneEdit(w, b[i])) {
			return false
		}
	}
	return true
}

// spelling: the name is a close spelling of the title (typos, "Colour" / "Color"); the best score wins.
func (m *matcher) spelling() *Cand {
	var best *Cand
	bestScore := 0.0
	for _, c := range m.cands {
		if !m.yearOK(c) {
			continue
		}
		for _, n := range c.Titles {
			nn := normTitle(n)
			if u16len(nn) < 5 || digitKey(nn) != digitKey(m.want) {
				continue
			}
			score := dice(nn, m.want)
			if wordsClose(words(n), words(m.title), score) && score >= 0.85 && score > bestScore {
				best, bestScore = c, score
			}
		}
	}
	return best
}

// prefix: the name starts with the title's words and goes on ("Show" -> "Show Returns"); the earliest wins.
func (m *matcher) prefix() *Cand {
	return earliest(filter(m.cands, func(c *Cand) bool {
		return m.yearGate(c, true) && anyName(c, func(n string) bool {
			w := dropArticle(words(n))
			if len(w) <= len(m.wantWords) {
				return false
			}
			for i, x := range m.wantWords {
				if w[i] != x {
					return false
				}
			}
			return true
		})
	}))
}

// subtitle: the title is the part after the colon ("Franchise: Show"); the earliest wins.
func (m *matcher) subtitle() *Cand {
	return earliest(filter(m.cands, func(c *Cand) bool {
		return m.yearGate(c, false) && anyName(c, func(n string) bool {
			i := strings.Index(n, ":")
			return i > 0 && normTitle(n[i+1:]) == m.want
		})
	}))
}

// superset: the name holds all the title's words plus one to three more; the fewest extra words win.
func (m *matcher) superset() *Cand {
	var best *Cand
	bestExtra := 0
	for _, c := range m.cands {
		if !m.yearGate(c, false) {
			continue
		}
		for _, n := range c.Titles {
			have := words(n)
			set := map[string]bool{}
			for _, w := range have {
				set[w] = true
			}
			extra := len(have) - len(m.wantWords)
			if extra < 1 || extra > 3 {
				continue
			}
			all := true
			for _, x := range m.wantWords {
				if !set[x] {
					all = false
					break
				}
			}
			if all && digitKey(normTitle(n)) == digitKey(m.want) {
				if best == nil || extra < bestExtra {
					best, bestExtra = c, extra
				}
				break
			}
		}
	}
	return best
}

// alias: a candidate that a configured alias led to.
func (m *matcher) alias() *Cand {
	for _, c := range m.cands {
		if c.ViaAlias && m.yearOK(c) {
			return c
		}
	}
	return nil
}

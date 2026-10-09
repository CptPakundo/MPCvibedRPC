package artwork

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

// ---- seasons that are really separate titles -----------------------------------------
// Anime (and plenty of live-action) catalogs list each season as its own title:
//   "Show: Fourth Stage", "Show Season 2", "Show II".

var seasonNouns = map[string]bool{"season": true, "stage": true, "series": true, "part": true, "cour": true, "chapter": true, "act": true, "round": true}
var roman = map[string]int{"ii": 2, "iii": 3, "iv": 4, "vi": 6, "vii": 7, "viii": 8, "ix": 9}
var stopWords = map[string]bool{"and": true, "the": true, "of": true, "a": true, "an": true, "in": true, "to": true}

var (
	reTokens  = re(`[\p{L}\p{N}]+`, "gu")
	reNumOrd  = re(`^(\d{1,2})(?:st|nd|rd|th)?$`, "")
	reOneDig  = re(`^\d$`, "")
	reGeneric = re(`^(?:season|series|saison|staffel|temporada|stagione)?\s*\d+$`, "i")
)

type token struct {
	w          string
	start, end int // rune offsets into the original text
}

func tokenize(s string) []token {
	var out []token
	for _, m := range reTokens.FindAll(s) {
		out = append(out, token{jsre.ToLower(stripMarks(m.Str(0))), m.Index(), m.End()})
	}
	return out
}

func tokWords(t []token) []string {
	w := make([]string, len(t))
	for i, x := range t {
		w[i] = x.w
	}
	return w
}

// numOf: the season number a word stands for, or -1.
func numOf(w string) int {
	if w == "" {
		return -1
	}
	if n, ok := ordinals[w]; ok {
		return n
	}
	if m := reNumOrd.Exec(w); m != nil {
		n, _ := strconv.Atoi(m.Str(1))
		return n
	}
	return -1
}

// SeasonMark is what a season name denotes: a number, or "final".
type SeasonMark struct {
	N     int // core.NA when none
	Final bool
}

func parseSeasonMark(rem []string) SeasonMark {
	at := func(i int) string {
		if i < len(rem) {
			return rem[i]
		}
		return ""
	}
	for i := range rem {
		w, next := rem[i], at(i+1)
		if w == "final" && seasonNouns[next] {
			return SeasonMark{N: core.NA, Final: true}
		}
		if seasonNouns[w] && numOf(next) >= 0 {
			return SeasonMark{N: numOf(next)}
		}
		if numOf(w) >= 0 && seasonNouns[next] {
			return SeasonMark{N: numOf(w)}
		}
		if seasonNouns[w] && roman[next] != 0 {
			return SeasonMark{N: roman[next]}
		}
	}
	if len(rem) == 1 && roman[rem[0]] != 0 {
		return SeasonMark{N: roman[rem[0]]}
	}
	if len(rem) == 1 && reOneDig.Test(rem[0]) {
		if n, _ := strconv.Atoi(rem[0]); n >= 2 {
			return SeasonMark{N: n}
		}
	}
	return SeasonMark{N: core.NA}
}

type initials struct{ initials, joined, all string }

func firstUnit(w string) string {
	for _, r := range w {
		return string(r)
	}
	return ""
}

func upperJS(s string) string { return strings.ToUpper(s) }

func initialsOf(rem []string) initials {
	var ws, all []string
	for _, w := range rem {
		if !stopWords[w] {
			all = append(all, w)
			if !seasonNouns[w] {
				ws = append(ws, w)
			}
		}
	}
	first := func(l []string) string {
		var b strings.Builder
		for _, w := range l {
			b.WriteString(firstUnit(w))
		}
		return upperJS(b.String())
	}
	return initials{first(ws), upperJS(strings.Join(ws, "")), first(all)}
}

func codeMatches(rem []string, prefix string) bool {
	in := initialsOf(rem)
	return u16len(prefix) >= 2 && (in.initials == prefix || in.joined == prefix || in.all == prefix)
}

type member struct {
	c       *Cand
	rem     []string
	remText string
	mark    SeasonMark
}

// franchiseMembers: candidates that are "<title> + something" (the other seasons of the same franchise).
func franchiseMembers(cands []*Cand, title string) []member {
	tw := tokWords(tokenize(title))
	for len(tw) > 0 && reArt1.Test(tw[0]) && len(tw) > 1 {
		tw = tw[1:]
	}
	if len(tw) == 0 {
		return nil
	}
	var out []member
	for _, c := range cands {
		for _, name := range c.Titles {
			toks := tokenize(name)
			k := 0
			for k < len(toks)-1 && reArt1.Test(toks[k].w) && len(toks)-k > len(tw) {
				k++
			}
			body := toks[k:]
			if len(body) <= len(tw) {
				continue
			}
			ok := true
			for i, x := range tw {
				if body[i].w != x {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			rem := body[len(tw):]
			rn := []rune(name)
			remText := core.Trim(string(rn[rem[0].start:]))
			out = append(out, member{c: c, rem: tokWords(rem), remText: remText})
			break
		}
	}
	return out
}

func earliestOf(list []member) *member {
	if len(list) == 0 {
		return nil
	}
	cp := append([]member(nil), list...)
	sort.SliceStable(cp, func(i, j int) bool { return yr(cp[i].c) < yr(cp[j].c) })
	return &cp[0]
}

// SeasonPick is a catalog entry chosen for a season.
type SeasonPick struct {
	Cand       *Cand
	SeasonName string
	Shift      int
}

// pickSeasonEntry: which listed title is season `season` (or the series whose production code is `codePrefix`)?
func pickSeasonEntry(cands []*Cand, title string, season int, codePrefix string) *SeasonPick {
	members := franchiseMembers(cands, title)
	if len(members) == 0 {
		return nil
	}
	for i := range members {
		members[i].mark = parseSeasonMark(members[i].rem)
	}
	var pick *member
	filter := func(f func(m member) bool) []member {
		var out []member
		for _, m := range members {
			if f(m) {
				out = append(out, m)
			}
		}
		return out
	}
	if season != core.NA {
		pick = earliestOf(filter(func(m member) bool { return m.mark.N == season }))
		if pick == nil {
			numbered := filter(func(m member) bool { return m.mark.N != core.NA })
			max := 0
			for _, m := range numbered {
				if m.mark.N > max {
					max = m.mark.N
				}
			}
			fin := filter(func(m member) bool { return m.mark.Final })
			if len(fin) > 0 && len(numbered) >= 2 && season == max+1 {
				pick = earliestOf(fin)
			}
		}
	}
	if pick == nil && codePrefix != "" {
		pick = earliestOf(filter(func(m member) bool { return codeMatches(m.rem, codePrefix) }))
	}
	if pick == nil {
		return nil
	}
	return &SeasonPick{Cand: pick.c, SeasonName: pick.remText}
}

// chainEntry: production codes that run on across catalog entries ("AB" has 93 episodes, and "AB Z" follows it).
func chainEntry(cands []*Cand, title, prefix string, first *Cand, n int) *SeasonPick {
	if first == nil || first.Eps == 0 || n == core.NA || n <= first.Eps {
		return nil
	}
	type nx struct {
		member
		initials
	}
	var nexts []nx
	for _, m := range franchiseMembers(cands, title) {
		if m.c != first && m.c.Eps >= 10 && m.c.Year >= first.Year {
			in := initialsOf(m.rem)
			if strings.HasPrefix(in.joined, prefix) && u16len(in.joined) <= u16len(prefix)+2 {
				nexts = append(nexts, nx{m, in})
			}
		}
	}
	sort.SliceStable(nexts, func(i, j int) bool { return yr(nexts[i].c) < yr(nexts[j].c) })
	cur, shift, name := first, 0, ""
	for _, m := range nexts {
		if n-shift <= cur.Eps {
			break
		}
		shift += cur.Eps
		cur = m.c
		name = m.remText
	}
	if n-shift >= 1 && n-shift <= cur.Eps && cur != first {
		return &SeasonPick{Cand: cur, SeasonName: name, Shift: shift}
	}
	return nil
}

// matchSeasonByCode: season list of ONE show (TVmaze/TMDB style): the season a production code like "AB" names.
func matchSeasonByCode(seasons []any, prefix string) any {
	for _, x := range seasons {
		if truthy(x) && truthy(get(x, "name")) && codeMatches(tokWords(tokenize(str(get(x, "name")))), prefix) {
			return x
		}
	}
	return nil
}

func genericSeasonName(n string) bool {
	return n == "" || reGeneric.Test(core.Trim(n))
}

func jsRound(x float64) float64 { return math.Floor(x + 0.5) }

var reDirector = re(`^director$`, "i")

// aniInfoOf: genres / score / director straight from AniList.
func aniInfoOf(m any) *core.ArtInfo {
	var dir any
	for _, e := range asArr(get(m, "staff", "edges")) {
		if truthy(e) && reDirector.Test(core.Trim(orStr(get(e, "role"), ""))) {
			dir = e
			break
		}
	}
	score := num(get(m, "averageScore"))
	info := &core.ArtInfo{Genres: []string{}}
	for _, g := range asArr(get(m, "genres")) {
		if s, ok := g.(string); ok {
			info.Genres = append(info.Genres, s)
		}
	}
	if len(info.Genres) > 3 {
		info.Genres = info.Genres[:3]
	}
	if score > 0 {
		info.Rating = jsRound(score) / 10
	}
	if dir != nil {
		info.Director = orStr(get(dir, "node", "name", "full"), "")
	}
	return info
}

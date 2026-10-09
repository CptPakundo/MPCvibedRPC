package core

import (
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

// ---- MPC-HC variables.html ---------------------------------------------------------

// Info is what MPC-HC's web interface reports about the current file.
type Info struct {
	File     string
	Dir      []string // folder names, nearest first
	FullDir  string
	State    int // -1 none, 0 stopped, 1 paused, 2 playing
	Position int // ms
	Duration int // ms
	Rate     float64
}

var (
	reEntDecRun = re(`(?:&#\d+;)+`, "g")
	reEntHexRun = re(`(?:&#x[0-9a-f]+;)+`, "gi")
	reSepAny   = re(`[\\/]`, "")
	reLastSeg  = re(`[\\/][^\\/]*$`, "")
	reSepRun   = re(`[\\/]+`, "")
	reDriveTop = re(`^[A-Za-z]:$`, "")
	reVarP     = re(`<p\s+id="([A-Za-z]+)"\s*>([\s\S]*?)<\/p>`, "g")
)

// decodeNumeric replaces runs of numeric entities. A run is decoded as a unit so that UTF-16 surrogate halves
// written as separate entities (&#55357;&#56832;) join into one character, like they do in a JavaScript string.
// bad is set when an entity is beyond U+10FFFF (JavaScript's String.fromCodePoint throws there).
func decodeNumeric(s string, runRe *jsre.Regexp, base int, bad *bool) string {
	return runRe.Replace(s, func(m *jsre.Match) string {
		var units []int
		for _, e := range strings.Split(m.Str(0), ";") {
			if e == "" {
				continue
			}
			digits := e[2:]
			if base == 16 {
				digits = digits[1:]
			}
			n := 0
			for _, c := range digits {
				if base == 16 {
					n = n*16 + hexv(c)
				} else {
					n = n*10 + int(c-'0')
				}
				if n > 0x10FFFF {
					*bad = true
					return ""
				}
			}
			units = append(units, n)
		}
		var b strings.Builder
		for i := 0; i < len(units); i++ {
			u := units[i]
			if u >= 0xD800 && u < 0xDC00 && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] < 0xE000 {
				b.WriteRune(0x10000 + (rune(u)-0xD800)<<10 + (rune(units[i+1]) - 0xDC00))
				i++
				continue
			}
			b.WriteRune(rune(u)) // a lone surrogate becomes U+FFFD
		}
		return b.String()
	})
}

func decodeEntities(s string) (string, bool) {
	bad := false
	s = decodeNumeric(s, reEntDecRun, 10, &bad)
	s = decodeNumeric(s, reEntHexRun, 16, &bad)
	for _, p := range [][2]string{{"&quot;", `"`}, {"&apos;", "'"}, {"&lt;", "<"}, {"&gt;", ">"}, {"&amp;", "&"}} {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	return s, !bad
}

func hexv(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	}
	return int(c-'A') + 10
}

func fullDirOf(filepath, filedir string) string {
	if filepath != "" && reSepAny.Test(filepath) {
		return reLastSeg.ReplaceStr(filepath, "")
	}
	return filedir
}

// dirOf lists the folder names of the playing file, nearest first (["Season 1", "Show Name", "TV"]).
func dirOf(filepath, filedir string) []string {
	var out []string
	for _, x := range reSepRun.Split(fullDirOf(filepath, filedir)) {
		if x != "" && !reDriveTop.Test(x) {
			out = append(out, x)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

// ParseVariables reads MPC-HC's /variables.html. It returns nil when the page is not an MPC variables page.
func ParseVariables(html string) *Info {
	raw := map[string]string{}
	for _, m := range reVarP.FindAll(html) {
		v, ok := decodeEntities(trim(m.Str(2)))
		if !ok {
			return nil // an impossible character makes the page unreadable
		}
		raw[Lower(m.Str(1))] = v
	}
	if _, ok := raw["state"]; !ok {
		return nil
	}
	rate := parseFloat(raw["playbackrate"])
	if !(rate > 0) {
		rate = 1
	}
	st, okState := parseIntJS(raw["state"])
	if !okState {
		st = NA
	}
	pos, _ := parseIntJS(raw["position"])
	dur, _ := parseIntJS(raw["duration"])
	return &Info{
		File:     raw["file"],
		Dir:      dirOf(raw["filepath"], raw["filedir"]),
		FullDir:  fullDirOf(raw["filepath"], raw["filedir"]),
		State:    st,
		Position: pos,
		Duration: dur,
		Rate:     rate,
	}
}

// ---- release-name parsing ------------------------------------------------------------

var strongRe = re(`^(?:`+strings.Join([]string{
	`(?:360|480|576|720|1080|1440|2160|4320)[pi]`, `[248]k`, `uhd`, `\d{3,4}x\d{3,4}`,
	`bluray|bdrip|brrip|bdremux|remux|web-dl|webdl|webrip|web-rip|hdtv|pdtv|dvdrip|dvdscr|dvd5|dvd9|dvdr|bdmv|hddvd|r5|hdrip|hdcam|hdts|telesync|camrip|vhsrip|tvrip|satrip|uhdrip|webhd`,
	`x26[45]|h26[45]|hevc|avc|xvid|divx|av1|vp9|10bits?|8bits?|hi10p?|hi444p?`,
	`aac\d?(?:\.\d)?|mp3|opus|ac3|eac3|e-ac-3|dts(?:-hd|-x|-es)?|truehd|flac|atmos|lpcm|ddp?\+?\d?(?:\.\d)?`,
	`hdr(?:10(?:\+|plus)?)?|dovi|hlg|sdr`,
	`stabili[sz]ed|upscaled|deinterlaced|uncensored|dcp|pcm(?:\d(?:\.\d)?)?|readnfo|repack|proper|extended|unrated|uncut|remastered|dubbed|subbed|multi|multisub|engsub|hardsub|softsub|dualaudio|imax|criterion|theatrical|rerip`,
}, "|")+`)$`, "i")

// Ambiguous words are only junk when they sit right before a strong marker ("AMZN WEB-DL").
var ambiguousRe = re(`^(?:web|dvd|bd|hd|dv|amzn|nf|dsnp|hulu|hmax|atvp|pcok|cr|[1-9]\.[01])$`, "i")

// Scene tags that are real words, so only junk right after the year or right before a technical marker.
var sceneRe = re(`^(?:internal|limited|retail|festival|final\.?cut)$`, "i")

var mediaExtRe = re(`\.(?=[A-Za-z0-9]*[A-Za-z])[A-Za-z0-9]{1,5}$`, "")

var smallWords = map[string]bool{"a": true, "an": true, "and": true, "as": true, "at": true, "but": true, "by": true, "for": true, "in": true, "nor": true, "of": true, "on": true, "or": true, "the": true, "to": true, "vs": true}

var (
	reBareLead  = re(`^[(\[{]+|[)\]}]+$`, "g")
	reSOH       = re("\u0001", "g")
	reHyphenEnd = re(`-[A-Za-z0-9]+$`, "")
	reYear      = re(`^(?:19|20)\d{2}$`, "")
)

func bare(t string) string { return reSOH.ReplaceStr(reBareLead.ReplaceStr(t, ""), ".") }

func isStrong(t string) bool {
	b := bare(t)
	if strongRe.Test(b) {
		return true
	}
	base := reHyphenEnd.ReplaceStr(b, "") // "x264-GROUP"
	return base != b && strongRe.Test(base)
}

func isYear(t string) bool { return reYear.Test(bare(t)) }

var (
	rpUnderscore = re(`_`, "g")
	rpBluray     = re(`\bblu[ .-]?ray\b`, "gi")
	rpWebdl      = re(`\bweb[ .-]?dl\b`, "gi")
	rpWebrip     = re(`\bweb[ .-]?rip\b`, "gi")
	rpBdrip      = re(`\bbd[ .-]?rip\b`, "gi")
	rpBrrip      = re(`\bbr[ .-]?rip\b`, "gi")
	rpDaily      = re(`(?<!\d)((?:19|20)\d\d)[.-](0[1-9]|1[0-2])[.-](0[1-9]|[12]\d|3[01])(?!\d)`, "g")
	rpDts        = re(`\bdts[ .-]?(hd|x|es)\b`, "gi")
	rpDolby      = re(`\bdolby[ .-]?vision\b`, "gi")
	rpDual       = re(`\bdual[ .-]?audio\b`, "gi")
	rpH26x       = re(`\b([hx])[ .]?(26[45])\b`, "gi")
	rpDigitDot   = re(`(?<!\d)(\d)\.(\d)(?!\d)`, "g")
	rpAcronym    = re(`(?<![A-Za-z])((?:[A-Za-z]\.)+[A-Za-z])(\.|(?![A-Za-z0-9]))`, "g")
	rpDot        = re(`\.(?!\s)`, "g")
	rpDotAny     = re(`\.`, "g")
)

func protect(s string) string {
	s = rpUnderscore.ReplaceStr(s, " ")
	s = rpBluray.ReplaceStr(s, "BluRay")
	s = rpWebdl.ReplaceStr(s, "WEB-DL")
	s = rpWebrip.ReplaceStr(s, "WEBRip")
	s = rpBdrip.ReplaceStr(s, "BDRip")
	s = rpBrrip.ReplaceStr(s, "BRRip")
	s = rpDaily.ReplaceStr(s, "$1-$2-$3") // daily shows: 2024.05.01
	s = rpDts.ReplaceStr(s, "DTS-$1")
	s = rpDolby.ReplaceStr(s, "DoVi")
	s = rpDual.ReplaceStr(s, "DualAudio")
	s = rpH26x.ReplaceStr(s, "$1$2")
	s = rpDigitDot.ReplaceStr(s, "$1\u0001$2")            // keep 5.1 / 7.1 intact
	s = rpAcronym.Replace(s, func(m *jsre.Match) string { // S.W.A.T. / S.H.I.E.L.D. stay acronyms
		out := rpDotAny.ReplaceStr(m.Str(1), "\u0001")
		if m.Str(2) != "" {
			out += "\u0001 "
		}
		return out
	})
	return rpDot.ReplaceStr(s, " ")
}

var (
	reHasLower = re(`[a-z]`, "")
)

func titleCase(str string) string {
	if str != Lower(str) || !reHasLower.Test(str) {
		return str // only fix all-lowercase names
	}
	words := strings.Split(str, " ")
	for i, w := range words {
		if i > 0 && smallWords[w] {
			continue
		}
		words[i] = firstRuneUpper(w)
	}
	return strings.Join(words, " ")
}

var (
	tdSOH    = re("\u0001", "g")
	tdWS     = re(`\s+`, "g")
	tdHyphen = re(`(?<=\p{L})-\s+(?=\p{Lu})`, "gu")
	tdSpaceQ = re(`\s+([?!])`, "g")
	tdEdges  = re(`^[\s\-–_.]+|[\s\-–_.]+$`, "g")
)

func tidy(s string) string {
	s = tdSOH.ReplaceStr(s, ".")
	s = tdWS.ReplaceStr(s, " ")
	s = tdHyphen.ReplaceStr(s, ": ") // "Star Trek- Section 31": a ":" the file system wouldn't allow
	s = tdSpaceQ.ReplaceStr(s, "$1") // "What If ?"
	return tdEdges.ReplaceStr(s, "")
}

// Production-style episode codes such as AB001, CD012, EF150, OVA01.
// (Not season markers like S14, E03/EP03, or things like CD2 / VOL12 / DVD5.)
var codeRe = re(`^(?!S\d{1,2}$|EP?\d|(?:CD|DVD|VOL|PT|HD|UHD|NO|ID|TV)\d)[A-Z]{1,4}\d{2,4}$`, "")

type epMatch struct {
	idx, length int
	season      int
	episode     int
	parts       []string
	code        bool
	codeText    string
	anime       bool
	date        bool
}

var (
	feSE      = re(`^s(\d{1,2})((?:-?e\d{1,4})+)(?:v\d)?$`, "i")
	feNxM     = re(`^(\d{1,2})x(\d{2,3})$`, "i")
	feDate    = re(`^((?:19|20)\d\d)-(\d\d)-(\d\d)$`, "")
	feESx     = re(`^([A-Za-z]{2,4})x(\d{1,3})$`, "")
	feESxBad1 = re(`^(?:x|h)$`, "i")
	feESxBad2 = re(`^(?:web|dvd|hd|uhd|bd|ac|dd|dts)$`, "i")
	feSeason  = re(`^s(\d{1,2})$`, "i")
	feDigits4 = re(`^\d{1,4}$`, "")
	feEp      = re(`^e(?:p(?:isode)?)?(\d{1,4})$`, "i")
	feE       = re(`^e(?:p(?:isode)?)?(\d{1,4})(?:v\d)?$`, "i")
	feEpWord  = re(`^(?:ep|episode)$`, "i")
	feJp      = re(`^第?(\d{1,4})[話话集]$`, "")
	feDashEp  = re(`^(\d{1,3}|(?!(?:19|20)\d\d|1080|1440|2160|4320)\d{4})(?:v\d)?$`, "i")
)

// findEpisode finds an episode marker among tokens[1..cut).
func findEpisode(tokens []string, cut int) *epMatch {
	for i := 1; i < cut; i++ {
		b := bare(tokens[i])
		next := ""
		if i+1 < cut {
			next = bare(tokens[i+1])
		}
		if m := feSE.Exec(b); m != nil {
			return &epMatch{idx: i, length: 1, season: atoi(m.Str(1)), episode: digitsOf(m.Str(2)),
				parts: []string{"S" + pad2s(m.Str(1)) + strings.ToUpper(strings.ReplaceAll(m.Str(2), "-", ""))}}
		}
		if m := feNxM.Exec(b); m != nil {
			return &epMatch{idx: i, length: 1, season: atoi(m.Str(1)), episode: atoi(m.Str(2)), parts: []string{"S" + pad2s(m.Str(1)) + "E" + m.Str(2)}}
		}
		if m := feDate.Exec(b); m != nil && i > 0 { // "Show 2024-05-01 Guest": a dated episode
			return &epMatch{idx: i, length: 1, season: NA, episode: NA, parts: []string{b}, date: true}
		}
		// Production-code episode: "ESx02" (Extra Stage, episode 2). The letters name the season entry, like "AB001".
		if m := feESx.Exec(b); m != nil && !feESxBad1.Test(m.Str(1)) && !feESxBad2.Test(m.Str(1)) {
			code := strings.ToUpper(m.Str(1)) + padStart3(m.Str(2))
			return &epMatch{idx: i, length: 1, season: NA, episode: atoi(m.Str(2)), parts: []string{"E" + pad2s(m.Str(2))}, code: true, codeText: code, anime: true}
		}
		// Standalone season marker followed by an episode: "S14 - AB001", "S02 - 05", "S02 E05"
		if m := feSeason.Exec(b); m != nil {
			j := i + 1
			for j < cut && bare(tokens[j]) == "-" {
				j++
			}
			t := ""
			if j < cut {
				t = bare(tokens[j])
			}
			season := atoi(m.Str(1))
			if feDigits4.Test(t) {
				return &epMatch{idx: i, length: j - i + 1, season: season, episode: atoi(t), parts: []string{"S" + pad2n(season) + "E" + pad2s(t)}}
			}
			if m2 := feEp.Exec(t); m2 != nil {
				return &epMatch{idx: i, length: j - i + 1, season: season, episode: atoi(m2.Str(1)), parts: []string{"S" + pad2n(season) + "E" + pad2s(m2.Str(1))}}
			}
			if codeRe.Test(t) {
				return &epMatch{idx: i, length: j - i + 1, season: season, episode: digitsOf(t), parts: []string{"S" + pad2n(season), t}, code: true, codeText: t, anime: true}
			}
			continue // "S02" with no episode (e.g. a season pack) is just part of the title
		}
		if m := feE.Exec(b); m != nil { // E03, Ep03, Episode03 (v2 = a re-release)
			return &epMatch{idx: i, length: 1, season: NA, episode: atoi(m.Str(1)), parts: []string{"E" + pad2s(m.Str(1))}}
		}
		if feEpWord.Test(b) && feDigits4.Test(next) { // "Episode 3", "Ep 03"
			return &epMatch{idx: i, length: 2, season: NA, episode: atoi(next), parts: []string{"E" + pad2s(next)}}
		}
		if m := feJp.Exec(b); m != nil && i > 0 { // 第28話 / 1000話
			return &epMatch{idx: i, length: 1, season: NA, episode: atoi(m.Str(1)), parts: []string{"E" + pad2s(m.Str(1))}, anime: true}
		}
		if b == "-" {
			if m := feDashEp.Exec(next); m != nil { // anime: "Title - 05", "One Piece - 1087"
				return &epMatch{idx: i, length: 2, season: NA, episode: atoi(m.Str(1)), parts: []string{"E" + pad2s(m.Str(1))}, anime: true}
			}
			if codeRe.Test(next) { // "Title - AB001"
				return &epMatch{idx: i, length: 2, season: NA, episode: digitsOf(next), parts: []string{next}, code: true, codeText: next, anime: true}
			}
		}
	}
	return nil
}

func padStart3(s string) string {
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

// Scene names without spaces or dots ("h-mongol1080-x264", "title-1080p-x264-grp"): hyphens separate words, and a
// resolution glued to the title ("mongol1080") is split off. A 1-2 letter first word is then a group tag.
var (
	glued       = re(`^(.*[A-Za-z]{3,})(480|576|720|1080|1440|2160)(?=-|$)`, "")
	sceneSpace  = re(`[\s._]`, "")
	sceneShort  = re(`^[A-Za-z]{1,2}$`, "")
	reVersion   = re(`^v\d{1,2}$`, "i")
	reHyphenAll = re(`-`, "g")
)

func sceneNormalize(base string) string {
	hy := len(reHyphenAll.FindAll(base))
	if sceneSpace.Test(base) || hy < 1 {
		return base
	}
	var parts []string
	didGlue := false
	for _, p := range strings.Split(base, "-") {
		if g := glued.Exec(p); g != nil && g.Str(0) == p {
			didGlue = true
			parts = append(parts, g.Str(1), g.Str(2)+"p")
		} else {
			parts = append(parts, p)
		}
	}
	if !didGlue && hy < 3 {
		return base
	}
	if didGlue && len(parts) > 2 && sceneShort.Test(parts[0]) {
		parts = parts[1:]
	}
	return strings.Join(parts, " ")
}

// "Show Title v3": a release revision (v2, v3...) trailing the title is not part of the name.
func stripVersion(tokens []string) []string {
	t := append([]string(nil), tokens...)
	for len(t) > 1 && reVersion.Test(bare(t[len(t)-1])) {
		t = t[:len(t)-1]
	}
	return t
}

// Media is what is known about the file that is playing.
type Media struct {
	Title       string
	Year        int // NA when unknown
	Season      int // NA
	Episode     int // NA
	Parts       []string
	EpTitle     string
	EpTitleBase string
	IsEpisode   bool
	AnimeHint   bool
	Code        string // "AB001"
	CodePrefix  string // "AB"
	Label       string
	Display     string
	Ext         string
	Index       int // NA: a leading list number ("02. Show Name")
	IndexTitle  string
	FromFolder  bool
	Anime       bool
	Junk        bool

	// filled in by later stages
	DurationMin  int // 0 = unknown
	CodeEpisode  int // NA
	BadgeEpisode int // NA
}

func newMedia() *Media {
	return &Media{Year: NA, Season: NA, Episode: NA, Index: NA, CodeEpisode: NA, BadgeEpisode: NA}
}

func (o *Media) finish() *Media {
	if o.Code != "" {
		o.CodePrefix = strings.ToUpper(reLetters.Exec(o.Code).Str(0)) // "AB001" -> "AB"
	} else {
		o.CodePrefix = ""
	}
	o.Label = strings.Join(o.Parts, " · ") // for the "details" line
	first := o.Title
	if has(o.Year) {
		first += " (" + itoa(o.Year) + ")"
	}
	var disp []string
	if first != "" {
		disp = append(disp, first)
	}
	disp = append(disp, o.Parts...)
	if o.EpTitle != "" {
		disp = append(disp, o.EpTitle)
	}
	o.Display = strings.Join(disp, " - ")
	return o
}

var reLetters = re(`^[A-Za-z]+`, "")

var (
	psBracket     = re(`\[([^\]]*)\]|\(([^)]*)\)`, "g")
	psYearOnly    = re(`^\s*(?:19|20)\d{2}\s*$`, "")
	psHex8        = re(`^[0-9A-Fa-f]{8}$`, "")
	psOpenBracket = re(`\[[^\]]*$`, "")
	psSeasonWord  = re(`^season$`, "i")
	psSeasonNum   = re(`^\d{1,2}$`, "")
	psAfterJunk   = re(`^\(?v\d\)?$|^(?:end|final|fin)$|^\d$`, "i")
	psSiteTitle   = re(`^(?:ign|gamespot|polygon|youtube|yts|rarbg|ettv|eztv|vtv|publichd)$`, "i")
	psParenYear   = re(`^\(\d{4}\)$`, "")
	psTailDigit   = re(`^[1-9]$`, "")
	psTailWord    = re(`^\p{L}{2,}$`, "u")
)

// tailDigit: "Storm.Front.2": a lone 1-9 after the title words may be a part number or part of the name ("Deep
// Space 9"); the file's own title keeps it, and a catalog title that continues the name replaces it.
func tailDigit(after []string) string {
	last := ""
	if len(after) > 0 {
		last = bare(after[len(after)-1])
	}
	if len(after) >= 2 && psTailDigit.Test(last) && psTailWord.Test(bare(after[len(after)-2])) {
		return last
	}
	return ""
}

func parseSmart(base string, cfg *Config) *Media {
	s := protect(sceneNormalize(base))
	s = psBracket.Replace(s, func(m *jsre.Match) string {
		inner := m.Str(2)
		if m.Has(1) {
			inner = m.Str(1)
		}
		if psYearOnly.Test(inner) {
			return " (" + trim(inner) + ") " // "[1984]" is a year
		}
		whole := m.Str(0)
		if strings.HasPrefix(whole, "[") && cfg.IgnoreBrackets {
			return " "
		}
		toks := fields(inner)
		if len(toks) == 1 && (ambiguousRe.Test(toks[0]) || psHex8.Test(toks[0])) {
			return " " // [AMZN], [92FB308F]
		}
		for _, t := range toks {
			if isStrong(t) {
				return " "
			}
		}
		return whole
	})
	s = psOpenBracket.ReplaceStr(s, " ") // a tag cut off before its closing bracket: "[DarkDream"
	s = trim(tdWS.ReplaceStr(s, " "))

	tokens := splitFilter(s, " ")
	if len(tokens) == 0 {
		return nil
	}

	cut := len(tokens)
	for i := 1; i < len(tokens); i++ {
		if isStrong(tokens[i]) {
			cut = i
			break
		}
	}
	// "Title 2026 iNTERNAL 2160p" / "Title iNTERNAL 1080p": drop the scene tag so it isn't left in the title.
	for j := cut - 1; j >= 1; j-- {
		if cut < len(tokens) && sceneRe.Test(bare(tokens[j])) && (isYear(tokens[j-1]) || j+1 == cut || (j+1 < len(tokens) && isYear(tokens[j+1]))) {
			tokens = append(tokens[:j], tokens[j+1:]...)
			cut--
		}
	}
	if cut < len(tokens) {
		for cut > 1 && ambiguousRe.Test(bare(tokens[cut-1])) {
			cut--
		}
	}

	if ep := findEpisode(tokens, cut); ep != nil {
		titleTokens := append([]string(nil), tokens[:ep.idx]...)
		after := append([]string(nil), tokens[ep.idx+ep.length:cut]...)
		season := ep.season
		year := NA

		for len(titleTokens) > 0 && titleTokens[len(titleTokens)-1] == "-" {
			titleTokens = titleTokens[:len(titleTokens)-1]
		}
		if len(titleTokens) > 1 && isYear(titleTokens[len(titleTokens)-1]) {
			year = atoi(bare(titleTokens[len(titleTokens)-1]))
			titleTokens = titleTokens[:len(titleTokens)-1]
		}
		if !has(season) && len(titleTokens) >= 2 {
			sw := titleTokens[len(titleTokens)-2:]
			if psSeasonWord.Test(bare(sw[0])) && psSeasonNum.Test(bare(sw[1])) {
				season = atoi(bare(sw[1]))
				titleTokens = titleTokens[:len(titleTokens)-2]
			}
		}
		yi := -1
		for k, t := range after {
			if isYear(t) {
				yi = k
				break
			}
		}
		if yi >= 0 {
			if !has(year) {
				year = atoi(bare(after[yi]))
			}
			after = append(after[:yi:yi], after[yi+1:]...)
		}

		parts := ep.parts
		if has(season) && !has(ep.season) { // "Season 2" words found before an E05-style marker
			if ep.code {
				parts = append([]string{"S" + pad2n(season)}, parts...)
			} else {
				parts = []string{"S" + pad2n(season) + "E" + pad2n(ep.episode)}
			}
		}
		td := tailDigit(after)
		var kept []string
		for k, t := range after {
			if !psAfterJunk.Test(bare(t)) || (k == len(after)-1 && td != "") {
				kept = append(kept, t)
			}
		}
		o := newMedia()
		o.Title = titleCase(tidy(strings.Join(stripVersion(titleTokens), " ")))
		o.Year = year
		o.Season = season
		o.Episode = ep.episode
		o.Parts = parts
		o.EpTitle = psSiteTitle.ReplaceStr(titleCase(tidy(strings.Join(kept, " "))), "")
		o.IsEpisode = true
		o.AnimeHint = ep.anime
		o.Code = ep.codeText
		if td != "" {
			o.EpTitleBase = titleCase(tidy(strings.Join(after[:len(after)-1], " ")))
		}
		return o.finish()
	}

	titleTokens := append([]string(nil), tokens[:cut]...)
	year := NA
	if cut < len(tokens) {
		for i := cut - 1; i >= 1; i-- {
			if isYear(titleTokens[i]) {
				year = atoi(bare(titleTokens[i]))
				// "Title 1994 DC 1080p", "Title 2019 KOREAN 1080p": what sits between the year and the technical tags is
				// an edition or language note, never part of the name.
				titleTokens = titleTokens[:i]
				break
			}
		}
	} else {
		last := titleTokens[len(titleTokens)-1]
		if len(titleTokens) > 1 && psParenYear.Test(last) && isYear(last) {
			year = atoi(bare(last))
			titleTokens = titleTokens[:len(titleTokens)-1]
		} else {
			// "Movie Title (1982) Final Cut": a parenthesised year ends the title; the words after it are an edition note.
			for i := len(titleTokens) - 2; i >= 1; i-- {
				if psParenYear.Test(titleTokens[i]) && isYear(titleTokens[i]) && len(titleTokens)-i <= 4 {
					year = atoi(bare(titleTokens[i]))
					titleTokens = titleTokens[:i]
					break
				}
			}
		}
	}
	o := newMedia()
	o.Title = titleCase(tidy(strings.Join(stripVersion(titleTokens), " ")))
	o.Year = year
	return o.finish()
}

var (
	bnBracket = re(`\[[^\]]*\]`, "g")
)

func basicName(base string, cfg *Config) string {
	name := base
	if cfg.IgnoreBrackets {
		name = bnBracket.ReplaceStr(name, "")
	}
	if cfg.ReplaceUnderscore {
		name = rpUnderscore.ReplaceStr(name, " ")
	}
	if cfg.ReplaceDots {
		name = rpDotAny.ReplaceStr(name, " ")
	}
	return trim(tdWS.ReplaceStr(name, " "))
}

// A filename that starts with an episode marker ("S01E02", "1x02", "Ep03", "Episode 3", "03") has no show name.
var (
	namelessRe  = re(`^\s*(?:S(\d{1,2})\s*E(\d{1,4})|(\d{1,2})x(\d{2,3})|E(?:p|pisode)?[ .]?(\d{1,4})|Episode[ .]?(\d{1,4})|Season[ ._]?(\d{1,2})[ ._,-]*Episode[ ._]?(\d{1,4})|(0\d{1,2}|\d{1,2})(?=\s*$|\s+[-–]\s|\s+(?:\d{3,4}p|\d{4}|[248]k|bluray|web|hdtv|x26[45]|h26[45]|hevc)\b))(?![A-Za-z\d])`, "i")
	seasonDirRe = re(`^(?:season|series|saison|staffel|temporada|stagione|s)[ ._-]*(\d{1,2})(?:\b.*)?$`, "i")
	genericDir  = re(`^(?:tv|tv ?shows?|series|shows?|videos?|movies?|films?|anime|downloads?|torrents?|media|new folder|desktop|documents|specials?|extras?|subs?|season ?\d+)$`, "i")
	reDotUnder  = re(`[._]+`, "g")
)

// withFolder prepends the folder's show name to a nameless filename. Returns the new base name and true, or "", false.
func withFolder(base string, dirs []string) (string, bool) {
	m := namelessRe.Exec(reDotUnder.ReplaceStr(base, " "))
	if m == nil || len(dirs) == 0 {
		return "", false
	}
	season := NA
	show := ""
	found := false
	for _, d := range dirs {
		if sd := seasonDirRe.Exec(d); sd != nil {
			if !has(season) {
				season = atoi(sd.Str(1))
			}
			continue
		}
		if genericDir.Test(trim(d)) {
			if has(season) || !found {
				continue
			}
			break
		}
		show = d
		found = true
		break
	}
	if !found || show == "" {
		return "", false
	}
	def := DefaultConfig()
	sm := parseSmart(reDotUnder.ReplaceStr(show, " "), &def) // "Show.Name.2008.1080p.BluRay"
	name := show
	if sm != nil && sm.Title != "" {
		name = sm.Title
	}
	year := ""
	if sm != nil && has(sm.Year) {
		year = " (" + itoa(sm.Year) + ")"
	}
	if name == "" || namelessRe.Test(name) {
		return "", false
	}
	rest := base
	ep := ""
	for _, g := range []int{2, 4, 5, 6, 8, 9} {
		if m.Has(g) && m.Str(g) != "" {
			ep = m.Str(g)
			break
		}
	}
	epSet := false
	for _, g := range []int{2, 4, 5, 6, 8, 9} {
		if m.Has(g) {
			epSet = true
			break
		}
	}
	_ = ep
	if !m.Has(1) && !m.Has(3) && !m.Has(7) && has(season) && epSet {
		var epv string
		for _, g := range []int{2, 4, 5, 6, 8, 9} {
			if m.Has(g) {
				epv = m.Str(g)
				break
			}
		}
		rest = "S" + pad2n(season) + "E" + pad2n(atoi(epv)) + u16slice(base, u16len(m.Str(0)), 1<<30) // season comes from "Season 2/"
	}
	dash := ""
	if m.Has(9) && rest == base {
		dash = " -"
	}
	return name + dash + " " + rest + year, true // "Lost - 03" reads as an episode
}

var (
	pmBannerA   = re(`^\s*\[\s*(?:https?:\/\/)?(?:www\.)?[\w-]+(?:\.[\w-]+)*\.(?:com|net|org|io|me|to|cc|tv|info|mx|ag|lol|cz|ru)\s*\]\s*[-–—:]*\s*(?=\S)`, "i")
	pmBannerB   = re(`^\s*(?:https?:\/\/)?www\.[\w-]+(?:\.[\w-]+)*\.[a-z]{2,4}\s*[-–—:]+\s*(?=\S)`, "i")
	pmLead1     = re(`^\s*(\d{1,3})\s+[-–]\s+(?=\p{L})`, "u")
	pmLead2     = re(`^\s*(\d{1,3})(?:\.\s+|\)\s*)(?=\p{L})`, "u")
	pmLead3     = re(`^\s*(0\d{1,2})[._](?=\p{L})`, "u")
	pmTwoLetter = re(`\p{L}{2}`, "u")
	pmGroup     = re(`^\s*\[([^\]]+)\]`, "")
	pmMovieGrp  = re(`\.(?:com|net|org|mx|to|cc|io|me|tv)\s*$|^\s*www\.|^(?:yts|yify|rarbg|etrg|ettv|eztv|fgt|publichd|tgx|1337x|ion10|galaxyrg|psa|nitro|rartv)\b`, "i")
	pmHexTag    = re(`\[[0-9A-Fa-f]{8}\]`, "")
	pmJunkA     = re(`^(?:img|vid|dsc|dscn|pxl|mvi|mov|vts|video|untitled|sample|new video|clip|screen ?(?:recording|shot)|wa\d|signal|capture)\b[\s_-]*[\d\s_-]*(?:at\b.*)?$`, "i")
	pmJunkB     = re(`^\d{4}[-_.]\d{2}[-_.]\d{2}\b`, "")
	pmJunkC     = re(`^[\d\s._-]+$`, "")
	pmLeadZero  = re(`^0`, "")
)

// ParseMedia works out what is playing from a filename (and, when useFolderName is on, its folders).
func ParseMedia(file string, cfg *Config, dirs []string) *Media {
	base := file
	ext := ""
	if m := mediaExtRe.Exec(base); m != nil {
		ext = m.Str(0)
		base = u16slice(base, 0, u16len(base)-u16len(ext))
	}
	fromFolder := false
	if cfg.UseFolderName && dirs != nil {
		if w, ok := withFolder(base, dirs); ok {
			base = w
			fromFolder = true
		}
	}

	// A release-site banner in front of the name: "www.Site.com - Title", "[ www.Site.com ] - Title".
	base = pmBannerB.ReplaceStr(pmBannerA.ReplaceStr(base, ""), "")

	// A leading list number ("02. Show Name", "03 - Title"): not part of the name. Kept as `index`.
	index := NA
	// "14. Show Name" could also be a title that starts with a number ("12. Title"): both are searched
	// (see `indexTitle`), the number-less name first.
	lead := pmLead1.Exec(base)
	if lead == nil {
		lead = pmLead2.Exec(base)
	}
	if lead == nil {
		lead = pmLead3.Exec(base)
	}
	if lead != nil && pmTwoLetter.Test(u16slice(base, u16len(lead.Str(0)), 1<<30)) {
		index = atoi(lead.Str(1))
		base = u16slice(base, u16len(lead.Str(0)), 1<<30)
	}

	var media *Media
	if cfg.SmartFormat {
		media = parseSmart(base, cfg)
	}
	if media == nil || media.Title == "" {
		name := basicName(base, cfg)
		if name == "" {
			name = file
		}
		media = newMedia()
		media.Title = name
		media.Display = name
	}
	media.Ext = ext
	if media.IsEpisode {
		media.Index = NA
	} else {
		media.Index = index
	}
	if has(media.Index) && media.Title != "" && !pmLeadZero.Test(itoa(index)) {
		media.IndexTitle = itoa(index) + " " + media.Title
	}
	media.FromFolder = fromFolder
	// Fansub-style names ([Group] ..., [CRC32], "Title - 05", AB001 codes) are almost always anime.
	grp := ""
	if g := pmGroup.Exec(base); g != nil {
		grp = g.Str(1)
	}
	movieGroup := grp != "" && pmMovieGrp.Test(grp)
	media.Anime = (media.AnimeHint && !fromFolder) || (grp != "" && !movieGroup) || pmHexTag.Test(base)
	// Camera/phone/screen-recording names and bare dates: nothing to look up.
	media.Junk = pmJunkA.Test(media.Title) || pmJunkB.Test(media.Title) || (pmJunkC.Test(media.Title) && !has(media.Year))
	return media
}

// FolderEpisode: production-code episodes ("S18 - AB092") count on across seasons, while the season folder holds only
// its own. The position inside the folder is what a season/episode badge should say: AB050 is the 1st file, AB092 the
// 43rd. Needs a reasonably full folder (5+ files with the same code prefix and season) whose lowest code is not 1.
// Returns NA when it does not apply.
func FolderEpisode(media *Media, files []string) int {
	if media == nil || !media.IsEpisode || media.Code == "" || media.CodePrefix == "" || !has(media.Season) || !has(media.Episode) {
		return NA
	}
	def := DefaultConfig()
	def.UseFolderName = false
	var nums []int
	for _, f := range files {
		m := ParseMedia(f, &def, nil)
		if m.IsEpisode && m.CodePrefix == media.CodePrefix && m.Season == media.Season && m.Code != "" && has(m.Episode) {
			nums = append(nums, m.Episode)
		}
	}
	if len(nums) < 5 {
		return NA
	}
	min := nums[0]
	for _, n := range nums {
		if n < min {
			min = n
		}
	}
	if min > 1 && media.Episode >= min {
		return media.Episode - min + 1
	}
	return NA
}

// CleanName is the cleaned-up display name of a file.
func CleanName(file string, cfg *Config) string {
	media := ParseMedia(file, cfg, nil)
	if cfg.IgnoreFiletype {
		return media.Display
	}
	return media.Display + media.Ext
}

// Year0 is the year, or 0 when unknown (the form the lookup code uses).
func (o *Media) Year0() int {
	if o.Year == NA {
		return 0
	}
	return o.Year
}

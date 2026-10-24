package core

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

// ---- Discord wire format -------------------------------------------------------------

type Timestamps struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

type Assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	LargeURL   string `json:"large_url,omitempty"` // opened when the large image is clicked
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

type Button struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Activity is Discord's activity payload (snake_case).
type Activity struct {
	Instance          bool        `json:"instance"`
	Type              int         `json:"type,omitempty"`
	Name              string      `json:"name,omitempty"`
	StatusDisplayType *int        `json:"status_display_type,omitempty"`
	Details           string      `json:"details,omitempty"`
	DetailsURL        string      `json:"details_url,omitempty"` // opened when the details line is clicked
	State             string      `json:"state,omitempty"`
	StateURL          string      `json:"state_url,omitempty"` // opened when the state line is clicked
	Timestamps        *Timestamps `json:"timestamps,omitempty"`
	Assets            *Assets     `json:"assets,omitempty"`
	Buttons           []Button    `json:"buttons,omitempty"`
}

// ---- display helpers -----------------------------------------------------------------

func jsRound(x float64) float64 { return math.Floor(x + 0.5) }

func fmtTime(ms int) string {
	total := int(math.Max(0, math.Floor(float64(ms)/1000)))
	h := total / 3600
	mm := (total % 3600) / 60
	ss := total % 60
	if h > 0 {
		return itoa(h) + ":" + pad2n(mm) + ":" + pad2n(ss)
	}
	return itoa(mm) + ":" + pad2n(ss)
}

func ProgressBar(position, duration, width int) string {
	width = max(0, min(width, 40)) // a hand-edited width must not panic or overflow Discord's 128 characters
	if !(duration > 0) {
		return ""
	}
	filled := int(math.Max(0, math.Min(float64(width), jsRound(float64(position)/float64(duration)*float64(width)))))
	return strings.Repeat("▰", filled) + strings.Repeat("▱", width-filled)
}

// clipText is clip for an optional text: empty stays empty.
func clipText(s string) string {
	if s == "" {
		return ""
	}
	return clip(s)
}

func clip(s string) string {
	if u16len(s) > 128 {
		s = u16slice(s, 0, 125) + "..."
	}
	if n := u16len(s); n < 2 {
		s += strings.Repeat(string(rune(0x200B)), 2-n) // zero-width spaces: Discord wants at least two characters
	}
	return s
}

// toFixed1 is Number.prototype.toFixed(1): exact value, ties round up.
func toFixed1(x float64) string {
	if x < 0 {
		return strconv.FormatFloat(x, 'f', 1, 64)
	}
	r := new(big.Rat)
	r.SetFloat64(x)
	r.Mul(r, big.NewRat(10, 1))
	r.Add(r, big.NewRat(1, 2))
	n := new(big.Int).Div(r.Num(), r.Denom()) // floor for positive numbers
	s := n.String()
	for len(s) < 2 {
		s = "0" + s
	}
	return s[:len(s)-1] + "." + s[len(s)-1:]
}

func editDistance(a, b string) int {
	ua := utf16.Encode([]rune(a))
	ub := utf16.Encode([]rune(b))
	prev := make([]int, len(ub)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ua); i++ {
		cur := make([]int, len(ub)+1)
		cur[0] = i
		for j := 1; j <= len(ub); j++ {
			c := 1
			if ua[i-1] == ub[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev = cur
	}
	return prev[len(ub)]
}

func bigrams(s string) map[[2]uint16]int {
	u := utf16.Encode([]rune(s))
	m := map[[2]uint16]int{}
	for n := 0; n < len(u)-1; n++ {
		m[[2]uint16{u[n], u[n+1]}]++
	}
	return m
}

func bigramSim(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if u16len(a) > u16len(b) && strings.Contains(a, b) && u16len(b) >= 6 {
		return 0.99
	}
	x, y := bigrams(a), bigrams(b)
	hit := 0
	for g, n := range x {
		hit += min(n, y[g])
	}
	return float64(2*hit) / math.Max(1, float64(u16len(a)-1+u16len(b)-1))
}

var (
	reYearTail   = re(`^(.*\S)\s+((?:19|20)\d{2})$`, "")
	reSeasonPunc = re(`[:–]|\s-\s`, "")
	rePokemon    = re(`^pok.mon\b`, "i")
	reEpLabel    = re(`^E\d+$`, "i")
	reGenre      = re(`^(BW|DP|SM|HGSS|Black & White|Diamond & Pearl|Sun & Moon|HeartGold & SoulSilver)(?:\s*[:\-–]\s*(.+))?$`, "")
	rePartSuffix = re(`^(?:part|pt)(?:i|ii|iii|iv|v|vi|vii|viii|ix|[1-9]|one|two|three|four|five)$|^(?:ii|iii|iv|v|vi|vii|viii|ix|[2-9])$`, "")
	genNames     = map[string]string{"BW": "Black & White", "DP": "Diamond & Pearl", "SM": "Sun & Moon", "HGSS": "HeartGold & SoulSilver"}
)

// truthy is JavaScript truthiness for the optional numbers.
func truthy(n int) bool { return n != NA && n != 0 }

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// BuildActivity builds the activity in Discord's wire format, or nil if there is nothing to show. media may be nil
// (it is then parsed from the file name); art may be nil. It only orchestrates: the title, the info line, the two
// text lines, the state and the images are each built by a function of their own below.
func BuildActivity(info *Info, cfg *Config, now int64, media *Media, art *Art) *Activity {
	if info == nil || info.File == "" || (info.State != 1 && info.State != 2) {
		return nil
	}
	if media == nil {
		media = ParseMedia(info.File, cfg, info.Dir)
	}
	// "02. Show Name": a list number. When the catalog says it's a series, that number is the episode.
	if art != nil && art.Type == "series" && !media.IsEpisode && media.Index != NA {
		cp := *media
		cp.IsEpisode = true
		cp.Episode = media.Index
		cp.Parts = []string{"E" + pad2n(media.Index)}
		cp.Label = "E" + pad2n(media.Index)
		media = &cp
	}
	// "Sample Show - Special 03" found as Sample Show's special: shown as S00E03 of the show.
	specialLabel := "" // the file's own words ("OVA 3"), shown instead of "Season 0, Episode 3"
	if art != nil && art.Special && art.Type == "series" {
		if sp := NamedSpecial(media); sp != nil {
			media, specialLabel = sp, sp.Label
		}
	}

	watching := cfg.ActivityType != "playing"
	titleInName := watching && cfg.TitleAsName
	title := chooseTitle(media, cfg, art)

	a := &Activity{Instance: false}

	if watching {
		a.Type = 3
		zero, two := 0, 2
		if titleInName {
			a.Name = clip(title)
			a.StatusDisplayType = &zero
		} else {
			a.StatusDisplayType = &two
		}
	}

	genres, infoLine := infoParts(media, cfg, art)
	effSeason := effectiveSeason(media, art)

	details := ""
	seasonLine := ""
	if titleInName {
		if media.IsEpisode {
			var eraName string
			details, seasonLine, eraName = episodeDetails(media, cfg, art, title, effSeason, specialLabel, watching)
			if eraName != "" {
				a.Name = clip(eraName)
			}
		} else {
			details = movieDetails(media, art, genres)
		}
	} else {
		details = media.Display
	}
	if details != "" {
		a.Details = clip(details)
	}

	a.State, a.Timestamps = stateAndTimes(info, cfg, now, watching, titleInName, seasonLine, infoLine)
	a.Assets = buildAssets(media, cfg, art, title, watching, effSeason, specialLabel)

	if cfg.LinkButton && art != nil && buttonURL(art.URL) {
		lbl := "View on " + firstNonEmpty(art.URLLabel, "IMDb")
		if u16len(lbl) > 32 {
			lbl = u16slice(lbl, 0, 32)
		}
		a.Buttons = []Button{{Label: lbl, URL: art.URL}}
	}
	return a
}

// chooseTitle is the title the activity is built around: the file's title, spelled the way the catalog spells it
// (CatalogTitle), then for anime the English or romaji title the user prefers (AnimeTitles).
func chooseTitle(media *Media, cfg *Config, art *Art) string {
	title := media.Title
	if title == "" {
		title = media.Display
	}
	title = catalogSpelling(title, media, cfg, art)
	return animeTitle(title, cfg, art)
}

// catalogSpelling shows a recognised title the way the catalog spells it, and a split season as "Show: Season name".
func catalogSpelling(title string, media *Media, cfg *Config, art *Art) string {
	if !(cfg.CatalogTitle && art != nil && art.Name != "" && art.Source != "Wikipedia" && !strings.ContainsAny(art.Name, "()")) {
		return title
	}
	var ty *jsre.Match
	if !media.IsEpisode && !truthy(media.Year) {
		ty = reYearTail.Exec(title)
	}
	kn, kt := normKey(art.Name), normKey(title)
	same := kn == kt || (ty != nil && kn == normKey(ty.Str(1)))
	near := !same && !art.Split && u16len(kn) >= 12 && u16len(kt) >= 12 && abs(u16len(kn)-u16len(kt)) <= 1 && editDistance(kn, kt) <= 1
	longer := !media.IsEpisode && art.Type == "movie" && u16len(kt) >= 4 && strings.HasPrefix(kn, kt)
	season := ""
	if media.IsEpisode && art.Split && !same && strings.HasPrefix(kn, kt) {
		if reSeasonPunc.Test(art.Name) || art.SeasonName == "" || kn != normKey(title+art.SeasonName) {
			season = art.Name
		} else {
			season = title + ": " + art.SeasonName
		}
	}
	numbered := media.IndexTitle != "" && kn == normKey(media.IndexTitle)
	wrapped := !media.IsEpisode && art.Type == "movie" && u16len(kn) >= 8 && u16len(kt) > u16len(kn) && strings.HasSuffix(kt, kn)
	if same || near || longer || wrapped || numbered {
		return art.Name
	}
	if season != "" {
		return season
	}
	return title
}

// animeTitle swaps in the English or romaji title of a recognised anime, as AnimeTitles asks ("file" keeps the
// file's own).
func animeTitle(title string, cfg *Config, art *Art) string {
	if art == nil || art.Alt == nil || cfg.AnimeTitles == "file" {
		return title
	}
	english, romaji := art.Alt.English, art.Alt.Romaji
	pick := ""
	switch {
	case cfg.AnimeTitles == "english":
		pick = firstNonEmpty(english, romaji)
	case cfg.AnimeTitles == "romaji":
		pick = firstNonEmpty(romaji, english)
	case english != "" && romaji != "":
		f := normKey(title)
		if bigramSim(f, normKey(romaji)) > bigramSim(f, normKey(english)) {
			pick = romaji
		} else {
			pick = english
		}
	default:
		pick = firstNonEmpty(english, romaji)
	}
	if pick != "" && normText(pick) != normText(title) {
		return pick
	}
	return title
}

// infoParts reads the catalog's extra facts (RichInfo): the genres (at most two), which a film shows in its first
// line, and the info line after the season line: genres and rating for an episode, rating and director for a film.
func infoParts(media *Media, cfg *Config, art *Art) (genres, infoLine string) {
	var inf *ArtInfo
	if cfg.RichInfo && art != nil && art.Info != nil {
		inf = art.Info
	}
	rating, director := "", ""
	if inf != nil {
		if len(inf.Genres) > 0 {
			g := inf.Genres
			if len(g) > 2 {
				g = g[:2]
			}
			genres = strings.Join(g, ", ")
		}
		if inf.Rating != 0 {
			rating = "★ " + toFixed1(inf.Rating)
		}
		if inf.Director != "" && !media.IsEpisode {
			director = "Dir. " + inf.Director
		}
	}
	if media.IsEpisode {
		infoLine = joinNonEmpty(" · ", genres, rating)
	} else {
		infoLine = joinNonEmpty(" · ", rating, director)
	}
	return genres, infoLine
}

// effectiveSeason is the season an episode is shown with: the file's own, else the catalog's, else 1 for a show the
// catalog splits into seasons; NA when it has none (or the file has no episode number).
func effectiveSeason(media *Media, art *Art) int {
	effSeason := NA
	if media.IsEpisode && media.Episode != NA {
		switch {
		case media.Season != NA:
			effSeason = media.Season
		case art != nil && art.Episode != nil && art.Episode.Season != NA && !art.Episode.Absolute:
			effSeason = art.Episode.Season
		case art != nil && art.Split:
			effSeason = 1
		}
	}
	return effSeason
}

// episodeDetails is the first text line of an episode (label and episode title), the season line that goes in front
// of the info line, and eraName, which replaces the status name when a Pokémon season or era is recognised ("" if not).
func episodeDetails(media *Media, cfg *Config, art *Art, title string, effSeason int, specialLabel string, watching bool) (details, seasonLine, eraName string) {
	var em *jsre.Match
	if art != nil && art.SeasonName != "" && rePokemon.Test(title) {
		em = reGenre.Exec(art.SeasonName)
	}
	seasonLabel := ""
	if art != nil {
		seasonLabel = art.SeasonName
	}
	if em != nil {
		era := em.Str(1)
		if g, ok := genNames[era]; ok {
			era = g
		}
		seasonLabel = ""
		if em.Has(2) {
			seasonLabel = em.Str(2)
		}
		if !strings.Contains(normText(title), normText(era)) {
			eraName = title + ": " + era
		}
	}
	if em == nil && media.CodePrefix == "XY" && media.Episode >= 1 && media.Episode <= 140 && rePokemon.Test(media.Title) {
		xy := "XYZ"
		if media.Episode <= 49 {
			xy = "XY"
		} else if media.Episode <= 93 {
			xy = "XY Kalos Quest"
		}
		eraName = media.Title + ": " + xy
		seasonLabel = ""
	}
	sn := ""
	if seasonLabel != "" && !strings.Contains(normText(title), normText(seasonLabel)) {
		sn = seasonLabel
	}
	var ep *ArtEpisode
	if art != nil {
		ep = art.Episode
	}
	label := media.Label
	if media.Season == NA && effSeason != NA && reEpLabel.Test(label) {
		label = "S" + pad2n(effSeason) + "E" + pad2n(media.Episode)
	}
	continued := false
	if media.EpTitleBase != "" && ep != nil && ep.Title != "" {
		nt, nb := normKey(ep.Title), normKey(media.EpTitleBase)
		suffix := ""
		if strings.HasPrefix(nt, nb) {
			suffix = nt[len(nb):]
		}
		continued = rePartSuffix.Test(suffix)
	}
	epTitle := ""
	switch {
	case continued:
		epTitle = ep.Title
	case media.EpTitle != "":
		epTitle = media.EpTitle
	case ep != nil:
		epTitle = ep.Title
	}
	badge := watching && media.Episode != NA && effSeason != NA
	seasonLine = sn
	if badge && !cfg.EpisodeInDetails {
		details = joinNonEmpty(" · ", epTitle)
	} else {
		details = joinNonEmpty(" · ", label, epTitle)
	}
	if details == "" && sn != "" {
		details = sn
		seasonLine = ""
	}
	if details == "" {
		details = specialLabel // a special whose name no catalog has
	}
	return details, seasonLine, eraName
}

// movieDetails is the first text line of a film: its year (the file's, else the catalog's) and the genres.
func movieDetails(media *Media, art *Art, genres string) string {
	year := ""
	switch {
	case truthy(media.Year):
		year = itoa(media.Year)
	case art != nil && art.Type == "movie" && truthy(art.Year):
		year = itoa(art.Year)
	}
	return joinNonEmpty(" · ", year, genres)
}

// stateAndTimes is the second text line and the timestamps: while playing the season and info lines (or "Playing")
// with the progress bar's start and end, while paused the position (and no timestamps).
func stateAndTimes(info *Info, cfg *Config, now int64, watching, titleInName bool, seasonLine, infoLine string) (state string, ts *Timestamps) {
	if info.State == 2 {
		rate := info.Rate
		if !(rate > 0) {
			rate = 1
		}
		if !titleInName {
			state = "Playing"
		} else if seasonLine != "" || infoLine != "" {
			state = clip(joinNonEmpty(" · ", seasonLine, infoLine))
		}
		start := int64(jsRound(float64(now) - float64(info.Position)/rate))
		end := int64(jsRound(float64(now) + math.Max(0, float64(info.Duration-info.Position))/rate))
		switch {
		case watching && info.Duration > 0:
			ts = &Timestamps{Start: start, End: end}
		case !watching && cfg.ShowRemainingTime && info.Duration > 0:
			ts = &Timestamps{End: end}
		default:
			ts = &Timestamps{Start: start}
		}
		return state, ts
	}
	clock := fmtTime(info.Position)
	if info.Duration > 0 {
		clock = fmtTime(info.Position) + " / " + fmtTime(info.Duration)
	}
	bar := ""
	if cfg.TextProgressBar {
		bar = ProgressBar(info.Position, info.Duration, cfg.ProgressWidth)
	}
	if bar != "" {
		return "⏸ " + bar + " " + clock, nil
	}
	return "Paused at " + clock, nil
}

// buildAssets is the cover and the small corner icon with their hover texts; nil when there is no image to show.
func buildAssets(media *Media, cfg *Config, art *Art, title string, watching bool, effSeason int, specialLabel string) *Assets {
	poster := ""
	if cfg.ShowArtwork && art != nil && art.Poster != "" {
		poster = art.Poster
	}
	assets := &Assets{}
	if poster != "" {
		assets.LargeImage = poster
		if cfg.SmallImageKey != "" {
			assets.SmallImage = cfg.SmallImageKey
			assets.SmallText = clipText(cfg.AppName)
		}
	} else if cfg.LargeImageKey != "" {
		assets.LargeImage = cfg.LargeImageKey
	}
	if assets.LargeImage == "" {
		return nil
	}
	if watching && effSeason != NA && media.Episode != NA {
		ep := media.Episode
		switch {
		case truthy(media.BadgeEpisode):
			ep = media.BadgeEpisode
		case art != nil && art.Split && truthy(art.EpInEntry):
			ep = art.EpInEntry
		}
		assets.LargeText = "Season " + itoa(effSeason) + ", Episode " + itoa(ep)
		if specialLabel != "" {
			assets.LargeText = specialLabel
		}
	} else if poster != "" {
		yr := NA
		if !media.IsEpisode {
			if truthy(media.Year) {
				yr = media.Year
			} else if art != nil && art.Type == "movie" && truthy(art.Year) {
				yr = art.Year
			}
		}
		if yr != NA {
			assets.LargeText = clip(title + " (" + itoa(yr) + ")")
		} else {
			assets.LargeText = clip(title)
		}
	} else {
		assets.LargeText = clip(cfg.AppName)
	}
	return assets
}

// AddLinks makes the two text lines and the cover open the title's catalog page, the page the "View on ..." button
// opens, under the same switch as that button. It is kept apart from BuildActivity, whose output the recorded golden
// data pins.
func AddLinks(a *Activity, cfg *Config, art *Art) {
	if a == nil || !cfg.LinkButton || art == nil || !linkable(art.URL) {
		return
	}
	if a.Details != "" {
		a.DetailsURL = art.URL
	}
	if a.State != "" {
		a.StateURL = art.URL
	}
	if a.Assets != nil && art.Poster != "" && a.Assets.LargeImage == art.Poster {
		a.Assets.LargeURL = art.URL
	}
}

// buttonURL is an http(s) address Discord accepts for a button (at most 512 characters).
func buttonURL(u string) bool {
	return len(u) <= 512 && (strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://"))
}

// linkable is an http(s) address short enough for Discord's link fields.
func linkable(u string) bool {
	return len(u) <= 256 && (strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://"))
}

// HasLinks reports whether a text line or the cover is a link.
func HasLinks(a *Activity) bool {
	return a != nil && (a.DetailsURL != "" || a.StateURL != "" || (a.Assets != nil && a.Assets.LargeURL != ""))
}

// WithoutLinks is a copy of a whose text lines and cover are not links (for a Discord that refuses them).
func WithoutLinks(a *Activity) *Activity {
	if a == nil {
		return nil
	}
	c := *a
	c.DetailsURL = ""
	c.StateURL = ""
	if a.Assets != nil {
		as := *a.Assets
		as.LargeURL = ""
		c.Assets = &as
	}
	return &c
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// LegacyActivity is the plain activity (no type/name/buttons) used if Discord rejects the extended one.
func LegacyActivity(a *Activity, cfg *Config) *Activity {
	details := joinNonEmpty(" · ", a.Name, a.Details)
	if u16len(details) > 128 {
		details = u16slice(details, 0, 125) + "..."
	}
	out := &Activity{Instance: false, Details: details, State: a.State}
	if a.Timestamps != nil {
		if a.Timestamps.Start != 0 {
			out.Timestamps = &Timestamps{Start: a.Timestamps.Start}
		} else {
			t := *a.Timestamps
			out.Timestamps = &t
		}
	}
	if cfg.LargeImageKey != "" {
		out.Assets = &Assets{LargeImage: cfg.LargeImageKey, LargeText: clipText(cfg.AppName)}
	}
	return out
}

// Prev is what NeedsUpdate remembers about the last presence sent.
type Prev struct {
	File     string
	Dir      string
	State    int
	Position int
	Rate     float64
	At       int64
	Duration int
}

// NeedsUpdate decides whether Discord needs a fresh update; it returns the (possibly new) remembered state.
func NeedsUpdate(prev *Prev, info *Info, now int64) (bool, *Prev) {
	rate := info.Rate
	if !(rate > 0) {
		rate = 1
	}
	dir := strings.Join(info.Dir, "/")
	next := &Prev{File: info.File, Dir: dir, State: info.State, Position: info.Position, Rate: rate, At: now, Duration: info.Duration}
	if prev == nil || prev.File != info.File || prev.Dir != dir || prev.State != info.State || prev.Rate != rate || (prev.Duration > 0) != (info.Duration > 0) {
		return true, next
	}
	if info.State == 2 {
		expected := float64(prev.Position) + float64(now-prev.At)*rate
		if math.Abs(float64(info.Position)-expected) > 3000*rate {
			return true, next
		}
		return false, prev
	}
	if abs(info.Position-prev.Position) >= 1000 {
		return true, next
	}
	return false, prev
}

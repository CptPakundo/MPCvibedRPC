package artwork

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"net/http"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// Options configures an Artwork finder.
type Options struct {
	CacheFile  string                  // JSON file the answers are kept in (optional)
	Log        func(level, msg string) // INFO / WARN lines for the log file
	OnResolved func(art *core.Art)     // a lookup the caller stopped waiting for has arrived
	Client     *http.Client            // HTTP client (tests plug a transport in)
}

type diskEntry struct {
	art  *core.Art
	ep   *core.ArtEpisode
	isEp bool
}

type lookupFlight struct {
	done chan struct{}
	res  *core.Art
}

// Artwork finds a poster (and catalog data / episode titles) for the title being watched.
// It never fails: any problem gives nil.
type Artwork struct {
	cfg   *core.Config
	opts  Options
	net   *Net
	provs map[string]*Provider
	res   *Resolver

	mu        sync.Mutex
	disk      map[string]*diskEntry
	misses    map[string]time.Time
	inflight  map[string]*lookupFlight
	late      map[string]bool
	warned    map[string]bool
	runtimes  map[string]int
	aliases   map[string]string
	overrides map[string]string
}

func mapByTitle(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[normTitle(k)] = v
	}
	return out
}

// New creates a finder for the given settings.
func New(cfg *core.Config, opts Options) *Artwork {
	if opts.Log == nil {
		opts.Log = func(string, string) {}
	}
	if opts.OnResolved == nil {
		opts.OnResolved = func(*core.Art) {}
	}
	a := &Artwork{cfg: cfg, opts: opts, net: newNet(opts.Client, cfg.RequestGapMs), provs: buildProviders(),
		disk: map[string]*diskEntry{}, misses: map[string]time.Time{}, inflight: map[string]*lookupFlight{}, late: map[string]bool{},
		warned: map[string]bool{}, runtimes: map[string]int{}, aliases: mapByTitle(cfg.ArtworkAliases), overrides: mapByTitle(cfg.ArtworkOverrides)}
	a.res = newResolver(a)
	a.load()
	return a
}

func (a *Artwork) log(level, msg string) { a.opts.Log(level, msg) }

func (a *Artwork) warnOnce(k, msg string) {
	a.mu.Lock()
	seen := a.warned[k]
	a.warned[k] = true
	a.mu.Unlock()
	if !seen {
		a.log("WARN", msg)
	}
}

// ---- disk cache -----------------------------------------------------------------------

func (a *Artwork) load() {
	if a.opts.CacheFile == "" {
		return
	}
	b, err := os.ReadFile(a.opts.CacheFile)
	if err != nil {
		return
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) != nil {
		return
	}
	for k, v := range raw {
		if strings.HasPrefix(k, "ep|") {
			e := &diskEntry{isEp: true}
			if string(v) != "null" {
				var ep core.ArtEpisode
				if json.Unmarshal(v, &ep) == nil {
					e.ep = &ep
				}
			}
			a.disk[k] = e
		} else {
			var art core.Art
			if json.Unmarshal(v, &art) == nil {
				a.disk[k] = &diskEntry{art: &art}
			}
		}
	}
}

func (a *Artwork) save() {
	if a.opts.CacheFile == "" {
		return
	}
	a.mu.Lock()
	keys := make([]string, 0, len(a.disk))
	for k := range a.disk {
		keys = append(keys, k)
	}
	sort.Strings(keys) // insertion order is not tracked; trim deterministically when huge
	if len(keys) > 2000 {
		for _, k := range keys[:len(keys)-2000] {
			delete(a.disk, k)
		}
	}
	out := make(map[string]any, len(a.disk))
	for k, e := range a.disk {
		switch {
		case e.isEp && e.ep == nil:
			out[k] = nil
		case e.isEp:
			out[k] = e.ep
		default:
			out[k] = e.art
		}
	}
	a.mu.Unlock()
	b, err := json.Marshal(out)
	if err == nil {
		_ = os.WriteFile(a.opts.CacheFile, b, 0o644)
	}
}

// ---- helpers --------------------------------------------------------------------------

func (a *Artwork) providerOrder(media *core.Media) []*Provider {
	cfg := a.cfg
	var order []string
	for _, n := range cfg.ArtworkSources {
		if a.provs[n] != nil {
			order = append(order, n)
		}
	}
	if media.Anime { // fansub-style filenames: anime databases first, in priority order
		src := cfg.AnimeSources
		if src == nil {
			src = []string{"anilist"}
		}
		var group []string
		for _, n := range src {
			if core.Contains(order, n) {
				group = append(group, n)
			}
		}
		rest := []string{}
		for _, n := range order {
			if !core.Contains(group, n) {
				rest = append(rest, n)
			}
		}
		order = append(group, rest...)
	}
	var out []*Provider
	for _, n := range order {
		p := a.provs[n]
		if p.Enabled == nil || p.Enabled(cfg) {
			out = append(out, p)
		}
	}
	return out
}

type provResult struct {
	p     *Provider
	typ   string
	names []string
	cands []*Cand
	cand  *Cand
	hit   *core.Art
}

type future struct {
	done chan struct{}
	v    *provResult
}

func async(f func() *provResult) *future {
	fu := &future{done: make(chan struct{})}
	go func() {
		defer close(fu.done)
		fu.v = f()
	}()
	return fu
}

func (f *future) wait() *provResult { <-f.done; return f.v }

func (a *Artwork) idsOf(p *Provider, c *Cand) *core.ArtIDs {
	ids := &core.ArtIDs{}
	if im := imdbIDFrom(c.URL); im != "" {
		ids.Imdb = im
	}
	if p.Label == "TVmaze" && c.Ref != "" {
		ids.Tvmaze = c.Ref
	}
	if p.Label == "TMDB" && c.Kind == "tv" && c.Ref != "" {
		ids.Tmdb = c.Ref
	}
	if p.Label == "Kitsu" && c.Ref != "" {
		ids.Kitsu = c.Ref
	}
	return ids
}

func (a *Artwork) hitOf(p *Provider, typ string, c *Cand) *core.Art {
	art := core.NewArt()
	art.Poster, art.URL, art.URLLabel = c.Poster, c.URL, c.URLLabel
	if len(c.Titles) > 0 {
		art.Name = c.Titles[0]
	}
	if c.Year != 0 {
		art.Year = c.Year
	}
	art.Type, art.Source = typ, p.Label
	art.IDs = a.idsOf(p, c)
	art.Alt = c.Alt
	art.AniInfo = c.AniInfo
	art.ViaRank = c.ViaRank
	return art
}

func (a *Artwork) runProvider(p *Provider, typ, query string, media *core.Media, contain []string) *provResult {
	all, err := p.Search(a, typ, query, media)
	if err != nil {
		a.warnOnce(p.Label, p.Label+" lookup failed ("+err.Error()+"); skipping it.")
		return &provResult{p: p, typ: typ}
	}
	var names []string
	var usable []*Cand
	for _, c := range all {
		if len(c.Titles) > 0 && c.Titles[0] != "" {
			names = append(names, c.Titles[0])
		}
		if c.Poster != "" && u16len(c.Poster) <= 256 {
			usable = append(usable, c)
		}
	}
	if p.Ranked && media.Year0() == 0 && media.DurationMin >= 5 {
		a.attachRuntimes(usable, query, typ)
	}
	c := pickMatch(usable, query, media.Year0(), typ == "movie", PickOpts{Earliest: p.Earliest, ExactOnly: p.ExactOnly, TrustAlias: p.TrustAlias, Ranked: p.Ranked, Runtime: media.DurationMin, Contain: contain})
	r := &provResult{p: p, typ: typ, names: names, cands: usable, cand: c}
	if c != nil {
		r.hit = a.hitOf(p, typ, c)
	}
	return r
}

func describe(h *core.Art) string {
	s := h.Name
	if h.Year != core.NA && h.Year != 0 {
		s += " (" + core.Itoa(h.Year) + ")"
	}
	if h.SeasonName != "" {
		s += " - " + h.SeasonName
	}
	return s + " via " + h.Source
}

func seenOf(results []*provResult) []string {
	var out []string
	for _, r := range results {
		if len(r.names) > 0 {
			n := r.names
			if len(n) > 3 {
				n = n[:3]
			}
			q := make([]string, len(n))
			for i, x := range n {
				q[i] = `"` + x + `"`
			}
			out = append(out, r.p.Label+": "+strings.Join(q, ", "))
		}
	}
	return out
}

func labelOf(media *core.Media) string { return media.Label }

// searchResult is the outcome of a search: a hit, or "none" with the names seen.
type searchResult struct {
	hit  *core.Art
	seen []string
}

func (a *Artwork) searchSeason(media *core.Media, query string, contain []string) searchResult {
	provs := a.providerOrder(media)
	type job struct {
		rank int
		f    *future
	}
	var jobs []job
	for _, typ := range []string{"series", "movie"} { // movies too: "Third Stage" is a film
		for pi, p := range provs {
			if !p.handles(typ) {
				continue
			}
			p, typ := p, typ
			rank := pi * 2
			if typ != "series" {
				rank++
			}
			jobs = append(jobs, job{rank, async(func() *provResult { return a.runProvider(p, typ, query, media, contain) })})
		}
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].rank < jobs[j].rank })
	var results []*provResult

	// (a) the franchise lists its seasons as separate titles.
	for _, j := range jobs {
		r := j.f.wait()
		results = append(results, r)
		m := pickSeasonEntry(r.cands, query, media.Season, media.CodePrefix)
		if m != nil {
			shift := 0
			if media.Code != "" && media.Episode != core.NA {
				if ch := chainEntry(r.cands, query, media.CodePrefix, m.Cand, media.Episode); ch != nil {
					m = &SeasonPick{Cand: ch.Cand, SeasonName: ch.SeasonName}
					shift = ch.Shift
				}
			}
			c := m.Cand
			hit := a.hitOf(r.p, r.typ, c)
			hit.ViaRank = false
			hit.SeasonName, hit.Split = m.SeasonName, true
			hit.Shift = shift
			hit.EntryEps = c.Eps
			a.log("INFO", `Artwork for "`+media.Title+`" `+labelOf(media)+`: `+describe(hit)+`.`)
			return searchResult{hit: hit}
		}
	}
	// (b) one show with numbered seasons that have their own poster and name
	for _, r := range results {
		if r.typ != "series" || r.hit == nil || r.p.Season == nil {
			continue
		}
		se, err := r.p.Season(a, r.cand, media)
		if err != nil {
			a.warnOnce(r.p.Label+":season", r.p.Label+" season lookup failed ("+err.Error()+"); using the show poster.")
			continue
		}
		if se != nil && (se.Poster != "" || !genericSeasonName(se.Name)) {
			hit := *r.hit
			if se.Poster != "" {
				hit.Poster = se.Poster
			}
			if genericSeasonName(se.Name) {
				hit.SeasonName = ""
			} else {
				hit.SeasonName = se.Name
			}
			hit.SeasonNo = se.Number
			a.log("INFO", `Artwork for "`+media.Title+`" `+labelOf(media)+`: `+describe(&hit)+`.`)
			return searchResult{hit: &hit}
		}
	}
	// (c) the show as a whole
	for _, r := range results {
		if r.typ == "series" && r.hit != nil {
			a.log("INFO", `Artwork for "`+media.Title+`": `+describe(r.hit)+`.`)
			return searchResult{hit: r.hit}
		}
	}
	return searchResult{seen: seenOf(results)}
}

func (a *Artwork) searchWith(media *core.Media, query string, contain []string) searchResult {
	if media.IsEpisode && (media.Season != core.NA || media.CodePrefix != "") {
		return a.searchSeason(media, query, contain)
	}
	var seen []string
	// A leading list number ("14. Show Name") points at a series episode: series first.
	types := []string{"movie", "series"}
	if media.IsEpisode {
		types = []string{"series"}
	} else if media.Index != core.NA {
		types = []string{"series", "movie"}
	}
	memo := map[string]*future{}
	run := func(p *Provider, typ string) *future {
		k := p.Label + "|" + typ
		if memo[k] == nil {
			memo[k] = async(func() *provResult { return a.runProvider(p, typ, query, media, contain) })
		}
		return memo[k]
	}
	// Fansub-style (anime) files: AniList answers for every kind of title before any other catalog is consulted.
	if media.Anime && !media.IsEpisode {
		var ani *Provider
		for _, p := range a.providerOrder(media) {
			if p.Label == "AniList" {
				ani = p
				break
			}
		}
		if ani != nil {
			for _, typ := range types {
				r := run(ani, typ).wait()
				if r.hit != nil {
					a.log("INFO", `Artwork for "`+media.Title+`": `+describe(r.hit)+`.`)
					return searchResult{hit: r.hit}
				}
				seen = append(seen, seenOf([]*provResult{r})...)
			}
		}
	}
	for _, typ := range types {
		var provs []*Provider
		for _, p := range a.providerOrder(media) {
			if p.handles(typ) {
				provs = append(provs, p)
			}
		}
		jobs := make([]*future, len(provs))
		for i, p := range provs {
			jobs[i] = run(p, typ) // all start now...
		}
		if media.Year0() == 0 && !media.IsEpisode {
			anyRanked := false
			for _, p := range provs {
				if p.Ranked {
					anyRanked = true
				}
			}
			if anyRanked {
				// No year: a catalog that ranks by popularity can tell the well-known title from an obscure namesake.
				var all []*provResult
				for i, p := range provs {
					if p.Ranked {
						all = append(all, jobs[i].wait())
					}
				}
				for _, r := range all {
					if r.hit != nil && r.hit.ViaRank {
						a.log("INFO", `Artwork for "`+media.Title+`": `+describe(r.hit)+`.`)
						return searchResult{hit: r.hit}
					}
				}
			}
		}
		for i := range jobs { // ...best priority wins
			r := jobs[i].wait()
			if r.hit != nil {
				a.log("INFO", `Artwork for "`+media.Title+`": `+describe(r.hit)+`.`)
				return searchResult{hit: r.hit}
			}
			seen = append(seen, seenOf([]*provResult{r})...)
		}
	}
	return searchResult{seen: seen}
}

func (a *Artwork) search(media *core.Media) *core.Art {
	cfg := a.cfg
	query := media.Title
	if v := a.aliases[normTitle(media.Title)]; v != "" {
		query = v
	}
	var seen []string
	spaced := core.Trim(reWS.ReplaceStr(re(`\s+[-–—]\s+`, "g").ReplaceStr(query, " "), " "))
	vars := titleVariants(media, query)
	var lead, rest []variant
	for _, v := range vars {
		if v.first {
			lead = append(lead, v)
		} else {
			rest = append(rest, v)
		}
	}
	var numbered []variant
	if media.IndexTitle != "" && query == media.Title {
		numbered = []variant{{query: media.IndexTitle}}
	}
	// "Dr Strangelove 1964" (no technical tags, so the year was not split off): try it as title + year first.
	var trailYear []variant
	if media.Year0() == 0 && !media.IsEpisode {
		if ty := reYearTail.Exec(query); ty != nil && reHasLetter.Test(ty.Str(1)) {
			y, _ := core.ParseIntPrefix(ty.Str(2))
			trailYear = []variant{{query: ty.Str(1), year: y}}
		}
	}
	// "Marvel's Agents of S.H.I.E.L.D." is listed as "Agents of S.H.I.E.L.D.": the studio's possessive is a prefix.
	var stripped []variant
	if pm := reStudio.Exec(query); pm != nil {
		stripped = []variant{{query: pm.Str(1)}}
	}
	var seq []variant
	seq = append(seq, trailYear...)
	seq = append(seq, lead...)
	if spaced != query {
		seq = append(seq, variant{query: spaced})
	}
	seq = append(seq, variant{query: query})
	seq = append(seq, rest...)
	seq = append(seq, numbered...)
	seq = append(seq, stripped...)
	_ = cfg
	for _, v := range seq {
		m := media
		if v.year != 0 {
			cp := *media
			cp.Year = v.year
			m = &cp
		}
		r := a.searchWith(m, v.query, v.contain)
		if r.hit != nil {
			return r.hit
		}
		seen = append(seen, r.seen...)
	}
	extra := ""
	if len(seen) > 0 {
		uniq := []string{}
		have := map[string]bool{}
		for _, s := range seen {
			if !have[s] {
				have[s] = true
				uniq = append(uniq, s)
			}
		}
		if len(uniq) > 6 {
			uniq = uniq[:6]
		}
		extra = " Closest results - " + strings.Join(uniq, "; ") + "."
	}
	a.log("INFO", `No artwork match for "`+media.Title+`".`+extra+` You can pin one with "Search under another name" / "Pin a cover image" in the Advanced settings.`)
	return nil
}

var (
	reYearTail  = re(`^(.*\S)\s+((?:19|20)\d{2})$`, "")
	reHasLetter = re(`\p{L}`, "u")
	reStudio    = re(`^(?:marvel|dc|disney|pixar|tom clancy|stephen king|john carpenter|clive barker|george a\.? romero|walt disney)['’]s\s+(.{4,})$`, "i")
)

// ---- title variants -------------------------------------------------------------------

type variant struct {
	query   string
	contain []string
	first   bool
	year    int
}

var (
	reIndexWord  = re(`^(?:movie|film|ova|oav|ona|special|part|vol(?:ume)?|episode)(?:\s*\d+)?$|^\d+$`, "i")
	reIndexWord2 = re(`^(?:movie|film|ova|oav|ona|special|part)(?:\s*\d+)?$`, "i")
	reVarSplit   = re(`\s+[-–—]\s+|:\s+`, "")
	reHeadBase   = re(`\b(?:the\s+)?(?:movie|film|ova|oav|special|episode|part|vol(?:ume)?)\b.*$`, "i")
	reDashSpace  = re(`\s+[-–—]\s+`, "g")
	reArticleOf  = re(`^(?:the|a|an|of)$`, "")
)

// titleVariants: "Franchise Movie 15 - Subtitle" is "Franchise the Movie: Subtitle" in the catalog.
// If the whole name finds nothing, search on the subtitle and require the result to contain
// both the subtitle and the franchise name.
func titleVariants(media *core.Media, query string) []variant {
	if media.IsEpisode {
		return nil
	}
	var raw []string
	for _, p := range reVarSplit.Split(query) {
		if p = core.Trim(p); p != "" {
			raw = append(raw, p)
		}
	}
	indexed := false
	var parts []string
	for _, p := range raw {
		if reIndexWord2.Test(p) {
			indexed = true
		}
		if !reIndexWord.Test(p) {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 {
		return nil
	}
	head := parts[0]
	tail := core.Trim(strings.Join(parts[1:], " "))
	headBase := core.Trim(reHeadBase.ReplaceStr(head, ""))
	nTail, nHead := normTitle(tail), normTitle(headBase)
	shortOk := indexed && media.Year0() != 0 && u16len(nTail) >= 4
	if !shortOk && (u16len(nTail) < 8 || len(words(tail)) < 2) {
		return nil
	}
	contain := []string{nTail}
	if u16len(nHead) >= 3 {
		contain = []string{nTail, nHead}
	}
	var out []variant
	if headBase != "" {
		out = append(out, variant{query: headBase + " " + tail, contain: contain})
	}
	out = append(out, variant{query: tail, contain: contain})
	need := 4
	if media.Year0() != 0 {
		need = 3
	}
	if shortOk || len(words(tail)) >= need {
		out = append(out, variant{query: tail, first: indexed && media.Year0() != 0})
	}
	return out
}

// ---- lookups that enrich a match ------------------------------------------------------

// findAniList: same title on AniList: exact name (any title or synonym) and a year within one.
func (a *Artwork) findAniList(b *core.Art) (*core.ArtAlt, error) {
	types := []string{"series", "movie"}
	if b.Type == "movie" {
		types = []string{"movie", "series"}
	}
	for _, typ := range types {
		list, err := a.provs["anilist"].Search(a, typ, b.Name, nil)
		if err != nil {
			return nil, err
		}
		y := 0
		if b.Year != core.NA {
			y = b.Year
		}
		c := pickMatch(list, b.Name, y, typ == "movie", PickOpts{ExactOnly: true, Earliest: true})
		if c != nil && c.Alt != nil && (c.Alt.English != "" || c.Alt.Romaji != "") {
			return c.Alt, nil
		}
	}
	return nil, nil
}

var (
	reHours = re(`(\d+)\s*h`, "i")
	reMins  = re(`(\d+)\s*m`, "i")
)

func minutesOf(v any) int {
	s := ""
	if truthy(v) {
		s = str(v)
	}
	h, m := reHours.Exec(s), reMins.Exec(s)
	if h == nil && m == nil {
		return 0
	}
	total := 0
	if h != nil {
		n, _ := core.ParseIntPrefix(h.Str(1))
		total += n * 60
	}
	if m != nil {
		n, _ := core.ParseIntPrefix(m.Str(1))
		total += n
	}
	return total
}

// attachRuntimes: IMDb's search results carry no runtime. For same-named candidates, read it from Cinemeta.
func (a *Artwork) attachRuntimes(list []*Cand, query, typ string) {
	want := normTitle(query)
	var group []*Cand
	for _, c := range list {
		if anyName(c, func(n string) bool {
			nn := normTitle(n)
			return nn == want || (strings.HasPrefix(nn, want) && normTitle(firstSeg(n)) == want)
		}) {
			group = append(group, c)
		}
	}
	if len(group) > 4 {
		group = group[:4]
	}
	if len(group) < 2 {
		return
	}
	var wg sync.WaitGroup
	for _, c := range group {
		wg.Add(1)
		go func(c *Cand) {
			defer wg.Done()
			id := imdbIDFrom(c.URL)
			if id == "" {
				return
			}
			a.mu.Lock()
			_, have := a.runtimes[id]
			if !have {
				a.runtimes[id] = 0
			}
			a.mu.Unlock()
			if !have {
				kind := "series"
				if typ == "movie" {
					kind = "movie"
				}
				j, err := a.net.getJSON(a.cfg.CinemetaBase+"/meta/"+kind+"/"+id+".json", reqInit{TimeoutMs: 3500})
				a.mu.Lock()
				if err != nil {
					delete(a.runtimes, id)
				} else {
					a.runtimes[id] = minutesOf(get(j, "meta", "runtime"))
				}
				a.mu.Unlock()
			}
			a.mu.Lock()
			rt := a.runtimes[id]
			a.mu.Unlock()
			if rt > 0 {
				c.Runtime = rt
			}
		}(c)
	}
	wg.Wait()
}

func cleanList(v any) []string {
	var out []string
	for _, x := range asArr(v) {
		if s, ok := x.(string); ok && core.Trim(s) != "" {
			out = append(out, core.Trim(s))
		}
	}
	return out
}

// fetchInfo: genres, rating, director from Cinemeta (keyed by IMDb id) for the second line of the presence.
func (a *Artwork) fetchInfo(typ, imdb string) (*core.ArtInfo, error) {
	j, err := a.net.getJSON(a.cfg.CinemetaBase+"/meta/"+typ+"/"+imdb+".json", reqInit{TimeoutMs: 7000})
	if err != nil {
		return nil, err
	}
	m := get(j, "meta")
	if !truthy(m) {
		return nil, nil
	}
	info := &core.ArtInfo{Genres: cleanList(firstTruthy(get(m, "genres"), get(m, "genre")))}
	if len(info.Genres) > 3 {
		info.Genres = info.Genres[:3]
	}
	rating := core.ParseFloatPrefix(str(get(m, "imdbRating")))
	if rating > 0 {
		info.Rating = jsRound(rating*10) / 10
	}
	if d := cleanList(get(m, "director")); len(d) > 0 {
		info.Director = d[0]
	}
	return info, nil
}

type imdbFound struct{ id, typ string }

// findImdbId: same title in IMDb's search -> its id. Catalogs spell names differently, so it tries the catalog's
// name and the file's name, and accepts an exact name, or a candidate holding every word of the file's name when
// the years agree.
func (a *Artwork) findImdbId(b *core.Art, media *core.Media) (*imdbFound, error) {
	first := "series"
	if b.Type == "movie" {
		first = "movie"
	}
	var names []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			names = append(names, s)
		}
	}
	add(b.Name)
	if b.Alt != nil {
		add(b.Alt.English)
		add(b.Alt.Romaji)
	}
	if media != nil {
		add(media.Title)
		for _, v := range titleVariants(media, media.Title) {
			add(v.query)
		}
	}
	by := 0
	if b.Year != core.NA {
		by = b.Year
	}
	yearOk := func(c *Cand) bool { return by == 0 || c.Year == 0 || abs(c.Year-by) <= 1 }
	second := "movie"
	if first == "movie" {
		second = "series"
	}
	for _, typ := range []string{first, second} {
		for _, q := range names {
			all, err := a.provs["imdb"].Search(a, typ, reDashSpace.ReplaceStr(q, " "), nil)
			if err != nil {
				return nil, err
			}
			var list []*Cand
			for _, c := range all {
				if yearOk(c) {
					list = append(list, c)
				}
			}
			c := pickMatch(list, q, by, typ == "movie", PickOpts{ExactOnly: true})
			if c == nil && media != nil && media.Title != "" {
				var want []string
				for _, w := range words(media.Title) {
					if !reArticleOf.Test(w) {
						want = append(want, w)
					}
				}
				if len(want) >= 3 {
					for _, x := range list {
						have := map[string]bool{}
						for _, n := range x.Titles {
							for _, w := range words(n) {
								have[w] = true
							}
						}
						all := true
						for _, w := range want {
							if !have[w] {
								all = false
							}
						}
						if all {
							c = x
							break
						}
					}
				}
			}
			// IMDb found it through an alternate title: trust that when the year agrees.
			if c == nil && by != 0 && len(list) > 0 && list[0].ViaAlias && list[0].Year != 0 && abs(list[0].Year-by) <= 1 {
				c = list[0]
			}
			if c != nil && imdbIDFrom(c.URL) != "" {
				return &imdbFound{imdbIDFrom(c.URL), typ}, nil
			}
		}
	}
	return nil, nil
}

// ---- lookup ---------------------------------------------------------------------------

func (a *Artwork) getBase(key string) *core.Art {
	if e := a.disk[key]; e != nil && !e.isEp {
		return e.art
	}
	return nil
}

func (a *Artwork) getEp(key string) (*diskEntry, bool) {
	e, ok := a.disk[key]
	if !ok || !e.isEp {
		return nil, false
	}
	return e, true
}

func (a *Artwork) missed(key string) bool {
	return a.misses[key].After(time.Now())
}

func ratingStr(f float64) string { return numStr(f) }

func infoSummary(inf *core.ArtInfo) string {
	var parts []string
	if len(inf.Genres) > 0 {
		parts = append(parts, strings.Join(inf.Genres, "/"))
	}
	if inf.Rating != 0 {
		parts = append(parts, "★"+ratingStr(inf.Rating))
	}
	if inf.Director != "" {
		parts = append(parts, inf.Director)
	}
	return strings.Join(parts, ", ")
}

func hasInfoData(i *core.ArtInfo) bool { return i != nil && (len(i.Genres) > 0 || i.Rating != 0) }

// Lookup finds the artwork for the playing title. It waits at most artworkWaitMs for the network; if the answer
// is not there by then it returns nil and OnResolved fires when it arrives.
func (a *Artwork) Lookup(media *core.Media) *core.Art {
	cfg := a.cfg
	if !cfg.ShowArtwork || media == nil || media.Title == "" || media.Junk {
		return nil
	}
	nt := normTitle(media.Title)
	query := media.Title
	if v := a.aliases[nt]; v != "" {
		query = v
	}

	var overrideArt *core.Art
	if ov := a.overrides[nt]; ov != "" {
		if (strings.HasPrefix(ov, "http://") || strings.HasPrefix(ov, "https://")) && u16len(ov) <= 256 {
			overrideArt = core.NewArt()
			overrideArt.Poster, overrideArt.Name, overrideArt.Source = ov, media.Title, "override"
		} else {
			a.warnOnce("ov:"+nt, `artworkOverrides entry for "`+media.Title+`" must be an http(s) image URL (max 256 chars).`)
		}
	}

	// Poster/season info is cached per show+season; the episode's own title per episode.
	kind := "m"
	if media.IsEpisode {
		kind = "s"
	}
	yearS, seasonS := "", ""
	if truthy2(media.Year) {
		yearS = core.Itoa(media.Year)
	}
	if media.Season != core.NA {
		seasonS = core.Itoa(media.Season)
	}
	dur := ""
	if !truthy2(media.Year) && !media.IsEpisode && media.DurationMin != 0 {
		dur = "|d" + core.Itoa(int(jsRound(float64(media.DurationMin)/10)))
	}
	key := "v3|" + kind + "|" + nt + "|" + yearS + "|" + normTitle(a.aliases[nt]) + "|" + seasonS + "|" + media.CodePrefix + dur
	// A leading list number ("02. Show Name") is the episode number if the title turns out to be a series.
	epNo := media.Index
	if media.IsEpisode {
		epNo = media.Episode
	}
	wantEp := cfg.EpisodeTitles && epNo != core.NA && media.EpTitle == ""
	epNoS := "null"
	if epNo != core.NA {
		epNoS = core.Itoa(epNo)
	}
	suffix := "i"
	if media.IsEpisode {
		suffix = ""
	}
	epKey := "ep|" + key + "|" + epNoS + suffix
	fullKey := key
	if wantEp {
		fullKey = epKey
	}
	// The episode's number inside the matched catalog entry depends on the episode, so it is worked out per call.
	inEntry := func(b *core.Art) int {
		if b != nil && b.Split && media.Code != "" && media.Episode != core.NA {
			return media.Episode - b.Shift
		}
		return core.NA
	}
	merge := func(base *core.Art, ep *core.ArtEpisode) *core.Art {
		if base == nil && ep == nil {
			return nil
		}
		out := core.NewArt()
		if base != nil {
			*out = *base
		}
		if ep != nil {
			out.Episode = ep
		}
		if n := inEntry(base); n != core.NA {
			out.EpInEntry = n
		}
		return out
	}
	// A cached entry only serves episodes that fall inside it (AB065 fits the first entry, AB139 needs the entry that continues the code).
	fits := func(b *core.Art) bool {
		n := inEntry(b)
		return n == core.NA || (n >= 1 && (b.EntryEps == 0 || n <= b.EntryEps))
	}

	infoKey := "info|" + key
	infoIDOf := func(b *core.Art) string {
		if b == nil || !cfg.RichInfo || b.Info != nil {
			return ""
		}
		if b.IDs != nil && b.IDs.Imdb != "" {
			return b.IDs.Imdb
		}
		return imdbIDFrom(b.URL)
	}
	canInfo := func(b *core.Art) bool { return b != nil && cfg.RichInfo && b.Info == nil && b.Source != "override" }
	needInfo := func(b *core.Art) bool { return canInfo(b) && !a.missed(infoKey) }
	aniKey := "ani|" + key
	needAlt := func(b *core.Art) bool {
		return b != nil && cfg.AnimeTitles != "file" && b.Source != "AniList" && b.Source != "override" && b.Alt == nil && !b.AltChecked &&
			b.Info != nil && core.Contains(b.Info.Genres, "Animation") && !a.missed(aniKey)
	}

	a.mu.Lock()
	var baseHave *core.Art
	if overrideArt != nil {
		baseHave = overrideArt
	} else if b := a.getBase(key); b != nil && fits(b) {
		baseHave = b
	}
	epEntry, epPresent := a.getEp(epKey)
	if baseHave != nil && !needInfo(baseHave) && !needAlt(baseHave) && (!wantEp || epPresent) {
		var ep *core.ArtEpisode
		if wantEp {
			ep = epEntry.ep
		}
		res := merge(baseHave, ep)
		a.mu.Unlock()
		return res
	}
	if baseHave == nil && a.missed(key) && (!wantEp || a.missed(epKey)) {
		a.mu.Unlock()
		return nil
	}
	f := a.inflight[fullKey]
	if f == nil {
		f = &lookupFlight{done: make(chan struct{})}
		a.inflight[fullKey] = f
		go a.runLookup(f, fullKey, key, epKey, wantEp, media, query, overrideArt, inEntry, merge, fits, infoIDOf, needInfo, needAlt, infoKey, aniKey)
	}
	a.mu.Unlock()

	// Don't hold up the presence for slow networks: wait briefly, then carry on and refresh the presence when the
	// lookup arrives (OnResolved).
	timer := time.NewTimer(time.Duration(cfg.ArtworkWaitMs) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-f.done:
		return f.res
	case <-timer.C:
	}
	a.mu.Lock()
	select {
	case <-f.done:
		a.mu.Unlock()
		return f.res
	default:
	}
	a.late[fullKey] = true
	a.mu.Unlock()
	return nil
}

func (a *Artwork) runLookup(f *lookupFlight, fullKey, key, epKey string, wantEp bool, media *core.Media, query string, overrideArt *core.Art,
	inEntry func(*core.Art) int, merge func(*core.Art, *core.ArtEpisode) *core.Art, fits func(*core.Art) bool,
	infoIDOf func(*core.Art) string, needInfo, needAlt func(*core.Art) bool, infoKey, aniKey string) {
	var result *core.Art
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = nil
			}
		}()
		result = a.lookupBody(fullKey, key, epKey, wantEp, media, query, overrideArt, inEntry, merge, fits, infoIDOf, needInfo, needAlt, infoKey, aniKey)
	}()
	a.mu.Lock()
	delete(a.inflight, fullKey)
	f.res = result
	close(f.done)
	cb := result != nil && a.late[fullKey]
	if cb {
		delete(a.late, fullKey)
	}
	a.mu.Unlock()
	if cb {
		a.opts.OnResolved(result)
	}
}

func (a *Artwork) lookupBody(fullKey, key, epKey string, wantEp bool, media *core.Media, query string, overrideArt *core.Art,
	inEntry func(*core.Art) int, merge func(*core.Art, *core.ArtEpisode) *core.Art, fits func(*core.Art) bool,
	infoIDOf func(*core.Art) string, needInfo, needAlt func(*core.Art) bool, infoKey, aniKey string) *core.Art {

	var base *core.Art
	a.mu.Lock()
	if overrideArt != nil {
		base = overrideArt
	} else if b := a.getBase(key); b != nil && fits(b) {
		cp := *b
		base = &cp
	}
	missKey := a.missed(key)
	a.mu.Unlock()

	if base == nil && !missKey {
		base = a.search(media)
		a.mu.Lock()
		if base != nil {
			a.disk[key] = &diskEntry{art: base}
		} else {
			a.misses[key] = time.Now().Add(30 * time.Minute)
		}
		a.mu.Unlock()
	}
	if base != nil && overrideArt == nil {
		cp := *base
		base = &cp // private copy; published again below
	}
	locked := func(f func()) { a.mu.Lock(); f(); a.mu.Unlock() }
	needInfoL := func(b *core.Art) bool { a.mu.Lock(); defer a.mu.Unlock(); return needInfo(b) }
	needAltL := func(b *core.Art) bool { a.mu.Lock(); defer a.mu.Unlock(); return needAlt(b) }

	if base != nil && overrideArt == nil && needInfoL(base) && base.Source == "AniList" && hasInfoData(base.AniInfo) {
		base.Info = base.AniInfo // AniList answered with genres and a score already: no second catalog needed
		a.log("INFO", `Info for "`+media.Title+`" from AniList: `+infoSummary(base.Info)+`.`)
	} else if base != nil && overrideArt == nil && needInfoL(base) {
		why := "no data"
		var found *imdbFound
		if infoIDOf(base) == "" {
			found, _ = a.findImdbId(base, media) // AniList/TVmaze hits carry no IMDb id
		}
		imdbID := infoIDOf(base)
		if imdbID == "" && found != nil {
			imdbID = found.id
		}
		if imdbID == "" {
			why = "no IMDb id found"
		}
		if found != nil {
			ids := core.ArtIDs{}
			if base.IDs != nil {
				ids = *base.IDs
			}
			ids.Imdb = found.id
			base.IDs = &ids
		}
		var inf *core.ArtInfo
		if imdbID != "" {
			typ := "series"
			if found != nil {
				typ = found.typ
			} else if base.Type == "movie" {
				typ = "movie"
			}
			var err error
			inf, err = a.fetchInfo(typ, imdbID)
			if err != nil {
				why = err.Error()
				if why == "" {
					why = "error"
				}
				inf = nil
			}
		}
		if inf != nil {
			base.Info = inf
			s := infoSummary(inf)
			if s == "" {
				s = "none"
			}
			a.log("INFO", `Info for "`+media.Title+`": `+s+`.`)
		} else if hasInfoData(base.AniInfo) {
			base.Info = base.AniInfo // IMDb/Cinemeta didn't answer: AniList's own data will do
			a.log("WARN", `No IMDb info for "`+media.Title+`" (`+why+`); using AniList's genres and score.`)
		} else {
			locked(func() { a.misses[infoKey] = time.Now().Add(5 * time.Minute) })
			a.log("WARN", `No genres/rating for "`+media.Title+`" (`+why+`); will retry in 5 minutes. Set richInfo: false to hide this line.`)
		}
	}
	if base != nil && overrideArt == nil && needAltL(base) {
		alt, err := a.findAniList(base)
		if err != nil {
			locked(func() { a.misses[aniKey] = time.Now().Add(5 * time.Minute) })
		} else {
			if alt != nil {
				base.Alt = alt
				a.log("INFO", `AniList names for "`+base.Name+`": `+joinNonEmpty(" / ", alt.English, alt.Romaji)+`.`)
			} else {
				base.AltChecked = true // checked, no AniList entry
			}
		}
	}
	if base != nil && overrideArt == nil {
		a.mu.Lock()
		a.disk[key] = &diskEntry{art: base}
		a.mu.Unlock()
	}

	var ep *core.ArtEpisode
	if wantEp {
		a.mu.Lock()
		cached, present := a.getEp(epKey)
		missEp := a.missed(epKey)
		a.mu.Unlock()
		if present {
			ep = cached.ep
		} else if !missEp {
			var epMedia *core.Media
			if media.IsEpisode {
				epMedia = media
				if n := inEntry(base); n != core.NA && n != media.Episode {
					cp := *media
					cp.Episode, cp.CodeEpisode = n, media.Episode
					epMedia = &cp
				}
			} else if base != nil && base.Type == "series" {
				cp := *media
				cp.IsEpisode, cp.Episode = true, media.Index
				epMedia = &cp
			}
			if epMedia != nil {
				ep = a.res.Find(epMedia, base, query, a.warnOnce)
			}
			a.mu.Lock()
			if epMedia == nil && base != nil {
				a.disk[epKey] = &diskEntry{isEp: true} // a movie: the list number is not an episode
			}
			if ep != nil {
				a.disk[epKey] = &diskEntry{isEp: true, ep: ep}
			} else if epMedia != nil {
				a.misses[epKey] = time.Now().Add(30 * time.Minute)
			}
			a.mu.Unlock()
			if ep != nil {
				num := func(n int) string {
					if n == core.NA {
						return "NaN"
					}
					return core.Itoa(n)
				}
				id := "S" + pad2(ep.Season) + "E" + pad2(ep.Number)
				if ep.Absolute {
					id = "#" + num(ep.Number)
				}
				via := ""
				if ep.Via != "" {
					via = ` (looked up as "` + ep.Via + `")`
				}
				a.log("INFO", `Episode: "`+media.Title+`" `+id+` = "`+ep.Title+`" via `+ep.Source+via+`.`)
			}
		}
	}
	a.save()
	return merge(base, ep)
}

func pad2(n int) string {
	if n == core.NA {
		return "NaN"
	}
	s := core.Itoa(n)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

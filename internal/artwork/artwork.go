package artwork

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	seq  int64 // when it was added in this run (0 = read from the file): the oldest go first when the file is trimmed
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

	closed  atomic.Bool  // Close was called: this finder was replaced (see Close)
	unavail atomic.Int64 // provider searches that failed because a service was down, slow or paused (see search)
	seq     int64        // the last diskEntry.seq handed out (a.mu)
	saveMu  sync.Mutex   // one cache write at a time (and none while the cache is being cleared)

	mu        sync.Mutex
	disk      map[string]*diskEntry
	misses    map[string]time.Time
	soft      map[string]bool     // misses set by softMiss
	closest   map[string][]string // the closest results of a title nothing matched (Explain)
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
		disk: map[string]*diskEntry{}, misses: map[string]time.Time{}, soft: map[string]bool{}, closest: map[string][]string{}, inflight: map[string]*lookupFlight{}, late: map[string]bool{},
		warned: map[string]bool{}, runtimes: map[string]int{}, aliases: mapByTitle(cfg.ArtworkAliases), overrides: mapByTitle(cfg.ArtworkOverrides)}
	a.net.blocked = blockedHostFilter(cfg)
	a.net.onLate = a.lateAnswer
	a.res = newResolver(a)
	a.load()
	return a
}

// keyVersion starts every key of the saved answers; a key of another version is an old format and is not loaded.
const keyVersion = "v3|"

// Close marks this finder as replaced (the settings changed and a new one took over). Lookups still running go on
// and finish, but they no longer write to the log or call OnResolved.
func (a *Artwork) Close() { a.closed.Store(true) }

func (a *Artwork) log(level, msg string) {
	if a.closed.Load() {
		return
	}
	a.opts.Log(level, msg)
}

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
		if !strings.HasPrefix(k, keyVersion) && !strings.HasPrefix(k, "ep|"+keyVersion) {
			continue // another key version: not used any more, so it must not take one of the saved places
		}
		if strings.HasPrefix(k, "ep|") {
			e := &diskEntry{isEp: true}
			if string(v) != "null" {
				var ep core.ArtEpisode
				if json.Unmarshal(v, &ep) == nil {
					if ep.Source == "Cinemeta" && ep.Absolute {
						continue // counted through Cinemeta's seasons, which can be off for anime: looked up again
					}
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

// put adds an answer to the cache (a.mu held).
func (a *Artwork) put(key string, e *diskEntry) {
	a.seq++
	e.seq = a.seq
	a.disk[key] = e
}

func (a *Artwork) save() {
	// A finder that was replaced (presence restarted, or the cache cleared) must not write its older map over the file.
	if a.opts.CacheFile == "" || a.closed.Load() {
		return
	}
	a.saveMu.Lock()
	defer a.saveMu.Unlock()
	a.mu.Lock()
	keys := make([]string, 0, len(a.disk))
	for k := range a.disk {
		keys = append(keys, k)
	}
	// when huge, the entries read from the file go first (in a fixed order), then the oldest of this run
	sort.Slice(keys, func(i, j int) bool {
		if si, sj := a.disk[keys[i]].seq, a.disk[keys[j]].seq; si != sj {
			return si < sj
		}
		return keys[i] < keys[j]
	})
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
		writeFileAtomic(a.opts.CacheFile, b)
	}
}

// writeFileAtomic writes next to the file and renames over it, so a crash never leaves half a cache file. On Windows
// the rename can fail for a moment (a virus scanner or indexer has the file open), so it is tried a few times.
func writeFileAtomic(path string, b []byte) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return
	}
	for i := 0; i < 5; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return
		}
		if runtime.GOOS != "windows" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = os.Remove(tmp)
}

// ClearCache forgets everything that was looked up: the saved answers (and the file they live in), the remembered
// misses, the episode lists kept in memory and the short-lived network cache. The next lookups ask the services again.
// It waits for a cache write in progress, and a write started later saves only what is in memory by then, so a
// lookup that was running cannot bring the cleared file back with the old answers.
func (a *Artwork) ClearCache() {
	a.saveMu.Lock()
	defer a.saveMu.Unlock()
	a.mu.Lock()
	a.disk = map[string]*diskEntry{}
	a.misses = map[string]time.Time{}
	a.soft = map[string]bool{}
	a.closest = map[string][]string{}
	a.runtimes = map[string]int{}
	a.mu.Unlock()
	a.res.clearCaches()
	a.net.clearCache()
	if a.opts.CacheFile != "" {
		_ = os.Remove(a.opts.CacheFile)
	}
}

// ---- helpers --------------------------------------------------------------------------

func (a *Artwork) providerOrder(media *core.Media) []*Provider {
	cfg := a.cfg
	var order []string
	for _, n := range cfg.ArtworkSources {
		if a.provs[n] != nil && cfg.SourceOn(n) {
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
			if slices.Contains(order, n) {
				group = append(group, n)
			}
		}
		rest := []string{}
		for _, n := range order {
			if !slices.Contains(group, n) {
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

// async runs f on its own goroutine. A panic in f must not take the app down: it gives an empty result.
func (a *Artwork) async(f func() *provResult) *future {
	fu := &future{done: make(chan struct{})}
	go func() {
		defer close(fu.done)
		defer func() {
			if r := recover(); r != nil {
				fu.v = &provResult{}
				a.warnOnce("panic:provider", "A cover lookup failed unexpectedly.")
			}
		}()
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
		if unavailable(err) {
			a.unavail.Add(1)
		}
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
		s += " (" + strconv.Itoa(h.Year) + ")"
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

// logHit logs the artwork found for media, with the media's label (the season or code) when withLabel is set.
func (a *Artwork) logHit(media *core.Media, withLabel bool, hit *core.Art) {
	label := ""
	if withLabel {
		label = " " + media.Label
	}
	a.log("INFO", `Artwork for "`+media.Title+`"`+label+`: `+describe(hit)+`.`)
}

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
			jobs = append(jobs, job{rank, a.async(func() *provResult { return a.runProvider(p, typ, query, media, contain) })})
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
			a.logHit(media, true, hit)
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
			a.logHit(media, true, &hit)
			return searchResult{hit: &hit}
		}
	}
	// (c) the show as a whole
	for _, r := range results {
		if r.typ == "series" && r.hit != nil {
			a.logHit(media, false, r.hit)
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
			memo[k] = a.async(func() *provResult { return a.runProvider(p, typ, query, media, contain) })
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
					a.logHit(media, false, r.hit)
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
						a.logHit(media, false, r.hit)
						return searchResult{hit: r.hit}
					}
				}
			}
		}
		for i := range jobs { // ...best priority wins
			r := jobs[i].wait()
			if r.hit != nil {
				a.logHit(media, false, r.hit)
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
	spaced := core.Trim(reWS.ReplaceStr(reDashSpace.ReplaceStr(query, " "), " "))
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
	// "Some Title 1964" (no technical tags, so the year was not split off): try it as title + year first.
	var trailYear []variant
	if media.Year0() == 0 && !media.IsEpisode {
		if ty := reYearTail.Exec(query); ty != nil && reHasLetter.Test(ty.Str(1)) {
			y, _ := core.ParseIntPrefix(ty.Str(2))
			trailYear = []variant{{query: ty.Str(1), year: y}}
		}
	}
	// "Studio's Show Name" is listed as "Show Name": the studio's possessive is a prefix.
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
	seq = append(seq, franchiseSubtitle(media, query)...)
	for _, v := range seq {
		m := media
		if v.year != 0 {
			cp := *media
			cp.Year = v.year
			m = &cp
		}
		r := a.searchWith(m, v.query, v.contain)
		if r.hit != nil {
			if v.subtitle != "" && cfg.CatalogTitle {
				h := *r.hit
				h.Name = h.Name + ": " + v.subtitle // the catalog lists the title without it
				return &h
			}
			// a catalog that answers "Sample Show - Special 03" with the show itself found the special's show
			if sp := core.NamedSpecial(media); sp != nil && r.hit.Type == "series" && normTitle(r.hit.Name) == normTitle(sp.Title) {
				h := *r.hit
				h.Special = true
				return &h
			}
			return r.hit
		}
		seen = append(seen, r.seen...)
	}
	// "Sample Show - Special 03" or "Sample Show OVA 2", not found as a title of its own: the show's special (S00E03).
	if sp := core.NamedSpecial(media); sp != nil && query == media.Title {
		r := a.searchWith(sp, sp.Title, nil)
		if r.hit != nil && r.hit.Type == "series" {
			h := *r.hit
			h.Special = true
			return &h
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
		a.mu.Lock()
		if len(a.closest) >= 50 {
			clear(a.closest)
		}
		a.closest[normTitle(media.Title)] = uniq // for Explain
		a.mu.Unlock()
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
	query    string
	contain  []string
	first    bool
	year     int
	subtitle string // added back to the catalog's title (franchiseSubtitle)
}

// reFranchiseTail: a film named "<Title>: A <Franchise> Mystery" loses its colon in a file name, and the catalogs list
// it as "<Title>" alone.
var reFranchiseTail = re(`^(.+?\S)\s+((?:a|an)\s+(?:\S+\s+){1,3}(?:mystery|story|movie|film|adventure|tale|saga|legend|odyssey))$`, "i")

// franchiseSubtitle: the last-resort search for such a film: "<Title>" alone, only with the file's year (so the
// catalog's year has to agree), and only when the title is long enough to be distinctive.
func franchiseSubtitle(media *core.Media, query string) []variant {
	if media.IsEpisode || media.Year0() == 0 {
		return nil
	}
	m := reFranchiseTail.Exec(query)
	if m == nil || len(words(m.Str(1))) < 2 || u16len(normTitle(m.Str(1))) < 6 {
		return nil
	}
	return []variant{{query: m.Str(1), subtitle: m.Str(2)}}
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
	s := str(v) // a falsy value (nil, false, 0, NaN) prints as text without "<n>h" or "<n>m", so it gives 0
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
	if !a.cfg.SourceOn("cinemeta") {
		return
	}
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
			defer func() {
				if r := recover(); r != nil {
					a.warnOnce("panic:runtime", "A cover lookup failed unexpectedly.")
				}
			}()
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
	if !a.cfg.SourceOn("imdb") || a.provs["imdb"] == nil {
		return nil, nil
	}
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

// softMiss remembers for two minutes that key found nothing because a service was down or slow. A slow answer
// that arrives later (Net.onLate) clears these, so the next tick asks again at once. a.mu must be held.
func (a *Artwork) softMiss(key string) {
	a.misses[key] = time.Now().Add(2 * time.Minute)
	a.soft[key] = true
}

// lateAnswer runs when a slow catalog answer was cached in the background: titles that missed for want of it are
// asked again on the next tick.
func (a *Artwork) lateAnswer() {
	a.mu.Lock()
	for k := range a.soft {
		delete(a.misses, k)
	}
	clear(a.soft)
	a.mu.Unlock()
}

func infoSummary(inf *core.ArtInfo) string {
	var parts []string
	if len(inf.Genres) > 0 {
		parts = append(parts, strings.Join(inf.Genres, "/"))
	}
	if inf.Rating != 0 {
		parts = append(parts, "★"+numStr(inf.Rating))
	}
	if inf.Director != "" {
		parts = append(parts, inf.Director)
	}
	return strings.Join(parts, ", ")
}

func hasInfoData(i *core.ArtInfo) bool { return i != nil && (len(i.Genres) > 0 || i.Rating != 0) }

// lookupJob holds what one Lookup of one title works out before it asks anything: the cache keys, the optional
// override and the predicates that decide what a cached entry still lacks.
type lookupJob struct {
	a           *Artwork
	media       *core.Media
	special     *core.Media // the title read as a show's special (core.NamedSpecial), or nil
	query       string
	overrideArt *core.Art
	wantEp      bool
	fullKey     string // the in-flight key: epKey when the episode title is wanted, else key
	key         string // poster/season info, per show+season
	epKey       string // the episode's own title, per episode
	infoKey     string
	aniKey      string
}

// newLookupJob works out the keys, the alias query and the override of one title.
func (a *Artwork) newLookupJob(media *core.Media) *lookupJob {
	cfg := a.cfg
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
		yearS = strconv.Itoa(media.Year)
	}
	if media.Season != core.NA {
		seasonS = strconv.Itoa(media.Season)
	}
	dur := ""
	if !truthy2(media.Year) && !media.IsEpisode && media.DurationMin != 0 {
		dur = "|d" + strconv.Itoa(int(jsRound(float64(media.DurationMin)/10)))
	}
	key := keyVersion + kind + "|" + nt + "|" + yearS + "|" + normTitle(a.aliases[nt]) + "|" + seasonS + "|" + media.CodePrefix + dur
	// A leading list number ("02. Show Name") is the episode number if the title turns out to be a series.
	epNo := media.Index
	if media.IsEpisode {
		epNo = media.Episode
	}
	suffix := "i"
	special := core.NamedSpecial(media)
	if media.IsEpisode {
		suffix = ""
	} else if epNo == core.NA && special != nil {
		epNo, suffix = special.Episode, "s" // used only if the show is found through the special
	}
	wantEp := cfg.EpisodeTitles && epNo != core.NA && media.EpTitle == ""
	epNoS := "null"
	if epNo != core.NA {
		epNoS = strconv.Itoa(epNo)
	}
	epKey := "ep|" + key + "|" + epNoS + suffix
	fullKey := key
	if wantEp {
		fullKey = epKey
	}
	return &lookupJob{a: a, media: media, special: special, query: query, overrideArt: overrideArt, wantEp: wantEp,
		fullKey: fullKey, key: key, epKey: epKey, infoKey: "info|" + key, aniKey: "ani|" + key}
}

// inEntry is the episode's number inside the matched catalog entry; it depends on the episode, so it is worked out
// per call.
func (j *lookupJob) inEntry(b *core.Art) int {
	if b != nil && b.Split && j.media.Code != "" && j.media.Episode != core.NA {
		return j.media.Episode - b.Shift
	}
	return core.NA
}

func (j *lookupJob) merge(base *core.Art, ep *core.ArtEpisode) *core.Art {
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
	if n := j.inEntry(base); n != core.NA {
		out.EpInEntry = n
	}
	return out
}

// asSpecial marks a cached show as found through a named special when it is that special's show: entries cached
// before Art.Special existed get the special's name too. It never changes b itself.
func (j *lookupJob) asSpecial(b *core.Art) *core.Art {
	if b == nil || b.Special || j.special == nil || b.Type != "series" || normTitle(b.Name) != normTitle(j.special.Title) {
		return b
	}
	cp := *b
	cp.Special = true
	return &cp
}

// fits: a cached entry only serves episodes that fall inside it (AB065 fits the first entry, AB139 needs the entry
// that continues the code).
func (j *lookupJob) fits(b *core.Art) bool {
	n := j.inEntry(b)
	return n == core.NA || (n >= 1 && (b.EntryEps == 0 || n <= b.EntryEps))
}

func (j *lookupJob) infoIDOf(b *core.Art) string {
	if b == nil || !j.a.cfg.RichInfo || b.Info != nil {
		return ""
	}
	if b.IDs != nil && b.IDs.Imdb != "" {
		return b.IDs.Imdb
	}
	return imdbIDFrom(b.URL)
}

// needInfo and needAlt say what a cached entry still lacks; the caller holds a.mu.
func (j *lookupJob) needInfo(b *core.Art) bool {
	cfg := j.a.cfg
	return b != nil && cfg.RichInfo && b.Info == nil && b.Source != "override" && !j.a.missed(j.infoKey)
}

func (j *lookupJob) needAlt(b *core.Art) bool {
	cfg := j.a.cfg
	return b != nil && cfg.AnimeTitles != "file" && b.Source != "AniList" && b.Source != "override" && b.Alt == nil && !b.AltChecked &&
		b.Info != nil && slices.Contains(b.Info.Genres, "Animation") && !j.a.missed(j.aniKey)
}

// miss remembers that key found nothing, for d.
func (j *lookupJob) miss(key string, d time.Duration) {
	j.a.mu.Lock()
	j.a.misses[key] = time.Now().Add(d)
	j.a.mu.Unlock()
}

// Lookup finds the artwork for the playing title. It waits at most artworkWaitMs for the network; if the answer
// is not there by then it returns nil and OnResolved fires when it arrives.
func (a *Artwork) Lookup(media *core.Media) *core.Art {
	cfg := a.cfg
	if !cfg.ShowArtwork || media == nil || media.Title == "" || media.Junk {
		return nil
	}
	j := a.newLookupJob(media)

	a.mu.Lock()
	var baseHave *core.Art
	if j.overrideArt != nil {
		baseHave = j.overrideArt
	} else if b := a.getBase(j.key); b != nil && j.fits(b) {
		baseHave = j.asSpecial(b)
	}
	epEntry, epPresent := a.getEp(j.epKey)
	if baseHave != nil && !j.needInfo(baseHave) && !j.needAlt(baseHave) && (!j.wantEp || epPresent) {
		var ep *core.ArtEpisode
		if j.wantEp {
			ep = epEntry.ep
		}
		res := j.merge(baseHave, ep)
		a.mu.Unlock()
		return res
	}
	if baseHave == nil && a.missed(j.key) && (!j.wantEp || a.missed(j.epKey)) {
		a.mu.Unlock()
		return nil
	}
	f := a.inflight[j.fullKey]
	if f == nil {
		f = &lookupFlight{done: make(chan struct{})}
		a.inflight[j.fullKey] = f
		go j.run(f)
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
	a.late[j.fullKey] = true
	a.mu.Unlock()
	return nil
}

// run does the lookup, publishes the result to everyone waiting on f and, when the caller had already given up,
// calls OnResolved.
func (j *lookupJob) run(f *lookupFlight) {
	a := j.a
	var result *core.Art
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = nil
				a.log("WARN", "A cover lookup failed unexpectedly.") // no title: it could be private
			}
		}()
		result = j.body()
	}()
	a.mu.Lock()
	delete(a.inflight, j.fullKey)
	f.res = result
	close(f.done)
	cb := result != nil && a.late[j.fullKey]
	delete(a.late, j.fullKey) // whatever the outcome: the next lookup of this title starts clean
	a.mu.Unlock()
	if cb && !a.closed.Load() {
		func() {
			defer func() {
				if r := recover(); r != nil {
					a.log("WARN", "A cover lookup failed unexpectedly.")
				}
			}()
			a.opts.OnResolved(result)
		}()
	}
}

// body runs the lookup steps in order: the base art, the rich info and the alternative names on it, the episode.
func (j *lookupJob) body() *core.Art {
	a := j.a
	base := j.baseArt()
	if base != nil && j.overrideArt == nil {
		j.richInfo(base)
		j.altNames(base)
		a.mu.Lock()
		a.put(j.key, &diskEntry{art: base})
		a.mu.Unlock()
	}
	var ep *core.ArtEpisode
	if j.wantEp {
		ep = j.episode(base)
	}
	a.save()
	return j.merge(base, ep)
}

// baseArt is the show's art: the override, the cached entry or a fresh search. Unless it is the override it is a
// private copy that body publishes again.
func (j *lookupJob) baseArt() *core.Art {
	a := j.a
	var base *core.Art
	a.mu.Lock()
	if j.overrideArt != nil {
		base = j.overrideArt
	} else if b := a.getBase(j.key); b != nil && j.fits(b) {
		cp := *j.asSpecial(b)
		base = &cp
	}
	missKey := a.missed(j.key)
	a.mu.Unlock()

	if base == nil && !missKey {
		down := a.unavail.Load()
		base = a.search(j.media)
		a.mu.Lock()
		if base != nil {
			a.put(j.key, &diskEntry{art: base})
		} else if a.unavail.Load() != down {
			a.softMiss(j.key) // a service was down or offline: not a real "not found"
		} else {
			a.misses[j.key] = time.Now().Add(30 * time.Minute)
			delete(a.soft, j.key)
		}
		a.mu.Unlock()
	}
	if base != nil && j.overrideArt == nil {
		cp := *base
		base = &cp // private copy; published again by body
	}
	return base
}

// richInfo adds genres and a rating to base, from AniList's own data or from Cinemeta through an IMDb id.
func (j *lookupJob) richInfo(base *core.Art) {
	a, media := j.a, j.media
	a.mu.Lock()
	need := j.needInfo(base)
	a.mu.Unlock()
	if !need {
		return
	}
	if base.Source == "AniList" && hasInfoData(base.AniInfo) {
		base.Info = base.AniInfo // AniList answered with genres and a score already: no second catalog needed
		a.log("INFO", `Info for "`+media.Title+`" from AniList: `+infoSummary(base.Info)+`.`)
		return
	}
	if !a.cfg.SourceOn("cinemeta") {
		return
	}
	why := "no data"
	var found *imdbFound
	if j.infoIDOf(base) == "" {
		found, _ = a.findImdbId(base, media) // AniList/TVmaze hits carry no IMDb id
	}
	imdbID := j.infoIDOf(base)
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
		j.miss(j.infoKey, 5*time.Minute)
		a.log("WARN", `No genres/rating for "`+media.Title+`" (`+why+`); will retry in 5 minutes. Set richInfo: false to hide this line.`)
	}
}

// altNames adds AniList's English and romaji names to base when it is an anime that has none yet.
func (j *lookupJob) altNames(base *core.Art) {
	a := j.a
	a.mu.Lock()
	need := j.needAlt(base)
	a.mu.Unlock()
	if !need || !a.cfg.SourceOn("anilist") {
		return
	}
	alt, err := a.findAniList(base)
	if err != nil {
		j.miss(j.aniKey, 5*time.Minute)
	} else if alt != nil {
		base.Alt = alt
		a.log("INFO", `AniList names for "`+base.Name+`": `+joinNonEmpty(" / ", alt.English, alt.Romaji)+`.`)
	} else {
		base.AltChecked = true // checked, no AniList entry
	}
}

// episode is the episode's own title: the cached one or a fresh lookup. A source that did not answer is asked
// again sooner than one that had no such episode.
func (j *lookupJob) episode(base *core.Art) *core.ArtEpisode {
	a, media := j.a, j.media
	a.mu.Lock()
	cached, present := a.getEp(j.epKey)
	missEp := a.missed(j.epKey)
	a.mu.Unlock()
	if present {
		return cached.ep
	}
	if missEp {
		return nil
	}
	failed := false // a source did not answer (rather than not having the episode)
	var epMedia *core.Media
	if media.IsEpisode {
		epMedia = media
		if n := j.inEntry(base); n != core.NA && n != media.Episode {
			cp := *media
			cp.Episode, cp.CodeEpisode = n, media.Episode
			epMedia = &cp
		}
	} else if base != nil && base.Type == "series" && base.Special && j.special != nil {
		epMedia = j.special
	} else if base != nil && base.Type == "series" && media.Index != core.NA {
		cp := *media
		cp.IsEpisode, cp.Episode = true, media.Index
		epMedia = &cp
	}
	var ep *core.ArtEpisode
	if epMedia != nil {
		ep = a.res.Find(epMedia, base, j.query, func(k, msg string) { failed = true; a.warnOnce(k, msg) })
	}
	a.mu.Lock()
	if epMedia == nil && base != nil {
		a.put(j.epKey, &diskEntry{isEp: true}) // a movie: the list number is not an episode
	}
	if ep != nil {
		a.put(j.epKey, &diskEntry{isEp: true, ep: ep})
	} else if epMedia != nil {
		if failed {
			a.softMiss(j.epKey) // a service was down or slow: ask again soon
		} else {
			a.misses[j.epKey] = time.Now().Add(30 * time.Minute)
			delete(a.soft, j.epKey)
		}
	}
	a.mu.Unlock()
	if ep != nil {
		num := func(n int) string {
			if n == core.NA {
				return "NaN"
			}
			return strconv.Itoa(n)
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
	return ep
}

func pad2(n int) string {
	if n == core.NA {
		return "NaN"
	}
	s := strconv.Itoa(n)
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

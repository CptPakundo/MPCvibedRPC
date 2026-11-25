package artwork

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPRP/internal/core"
)

// Fix a match: the user's fixes (core.MatchFix), the catalog page a fix pins, and the searches the window's Fix this
// match dialog asks for (Suggest, Covers). Every request goes through a.net, so each host's request gap, the blocked
// hosts and the Privacy switches apply.

// fixIndex finds the fix for a playing title: a fix for the file's exact name first, then one for the read title.
type fixIndex struct {
	byFile  map[string]*core.MatchFix // lower-cased file name
	byTitle map[string]*core.MatchFix // normTitle
}

func newFixIndex(fixes []core.MatchFix) fixIndex {
	ix := fixIndex{byFile: map[string]*core.MatchFix{}, byTitle: map[string]*core.MatchFix{}}
	for i := range fixes {
		fx := &fixes[i]
		if strings.TrimSpace(fx.Match) == "" {
			continue
		}
		if fx.File {
			ix.byFile[strings.ToLower(strings.TrimSpace(fx.Match))] = fx
		} else {
			ix.byTitle[normTitle(fx.Match)] = fx
		}
	}
	return ix
}

// FixFor is the user's fix for media (its file name, set by the engine, before its read title), or nil.
func (a *Artwork) FixFor(media *core.Media) *core.MatchFix {
	if a == nil || media == nil {
		return nil
	}
	if media.File != "" {
		if fx := a.fixes.byFile[strings.ToLower(media.File)]; fx != nil {
			return fx
		}
	}
	if media.Title == "" {
		return nil
	}
	return a.fixes.byTitle[normTitle(media.Title)]
}

// Offset moves the season and episode read from the file by the offsets of the user's fix (core.ShiftMedia), so the
// catalogs are asked, and the card shows, the catalog's numbers. shifted says it did; a number that would fall below 1
// leaves media as it is, with a warning in the log.
func (a *Artwork) Offset(media *core.Media) (out *core.Media, shifted bool) {
	fx := a.FixFor(media)
	if fx == nil || (fx.SeasonOffset == 0 && fx.EpisodeOffset == 0) || !media.IsEpisode {
		return media, false
	}
	m, ok := core.ShiftMedia(media, fx.SeasonOffset, fx.EpisodeOffset)
	if !ok {
		a.warnOnce("offset:"+fx.Match, `The episode offset of the fix for "`+fx.Match+`" would give a number below 1; it is not used for this file.`)
		return media, false
	}
	return m, m != media
}

// fixKey is the part of a cache key that a fix changes: the other name searched (as before matchFixes), or the pinned
// entry. "" without either, so the keys of titles nobody fixed stay as they were.
func fixKey(fx *core.MatchFix) string {
	if fx == nil {
		return normTitle("")
	}
	if fx.Entry != "" {
		return "@" + fx.Entry
	}
	return normTitle(fx.Search)
}

// coverOK checks a fix's own image address (http or https, at most 256 characters), with a warning once otherwise.
func (a *Artwork) coverOK(fx *core.MatchFix, media *core.Media) bool {
	c := fx.Cover
	if (strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://")) && u16len(c) <= 256 {
		return true
	}
	a.warnOnce("ov:"+normTitle(fx.Match), `The cover pinned for "`+media.Title+`" must be an http(s) image address (at most 256 characters).`)
	return false
}

// pinned is the art of the catalog page a fix pins, with a show's season poster and name when the catalog has them.
// search is true when the page could not be had (its catalog is off under Privacy, or the page is gone) and the read
// title should be searched instead; false with nil art when the catalog did not answer (asked again later).
func (a *Artwork) pinned(fx *core.MatchFix, media *core.Media) (art *core.Art, search bool) {
	e, ok := core.EntryPage(fx.Entry)
	if !ok {
		return nil, true
	}
	p, typ, c, err := a.entryCand(e)
	if err != nil && unavailable(err) {
		a.unavail.Add(1)
		a.warnOnce("pin:"+fx.Entry, `The entry pinned for "`+media.Title+`" could not be read (`+err.Error()+`); trying again later.`)
		return nil, false
	}
	if c == nil {
		a.warnOnce("pin:"+fx.Entry, `The entry pinned for "`+media.Title+`" was not found (its catalog may be off under Privacy, or the page is gone); searching the title instead.`)
		return nil, true
	}
	hit := a.hitOf(p, typ, c)
	if typ == "series" && media.IsEpisode && media.Season != core.NA && p.Season != nil {
		if se, err := p.Season(a, c, media); err == nil && se != nil && (se.Poster != "" || !genericSeasonName(se.Name)) {
			if se.Poster != "" {
				hit.Poster = se.Poster
			}
			if !genericSeasonName(se.Name) {
				hit.SeasonName = se.Name
			}
			hit.SeasonNo = se.Number
		}
	}
	a.log("INFO", `Artwork for "`+media.Title+`": `+describe(hit)+` (pinned in Fix a match).`)
	return hit, false
}

// entryCand reads a pinned catalog page: the provider that answered, the kind ("movie" or "series") and the
// candidate, or a nil candidate when no catalog that is on has it. An IMDb page is read from IMDb, else Cinemeta, else
// TVmaze (shows only).
func (a *Artwork) entryCand(e core.Entry) (p *Provider, typ string, c *Cand, err error) {
	cfg := a.cfg
	on := func(id string) bool {
		pr := a.provs[id]
		return cfg.ShowArtwork && pr != nil && cfg.SourceOn(id) && (pr.Enabled == nil || pr.Enabled(cfg))
	}
	var firstErr error
	keep := func(e error) {
		if firstErr == nil || unavailable(e) {
			firstErr = e
		}
	}
	switch e.Catalog {
	case "imdb":
		if on("imdb") {
			j, err := a.net.getJSON(cfg.ImdbSuggestBase+"/suggestion/t/"+e.ID+".json", reqInit{})
			if err != nil {
				keep(err)
			}
			for _, d := range asArr(get(j, "d")) {
				if str(get(d, "id")) != e.ID {
					continue
				}
				typ := "movie"
				if q := str(get(d, "qid")); q == "tvSeries" || q == "tvMiniSeries" {
					typ = "series"
				}
				return a.provs["imdb"], typ, imdbCand(d), nil
			}
		}
		if on("cinemeta") {
			for _, typ := range []string{"series", "movie"} {
				j, err := a.net.getJSON(cfg.CinemetaBase+"/meta/"+typ+"/"+e.ID+".json", reqInit{})
				if err != nil {
					keep(err)
					continue
				}
				if m := get(j, "meta"); truthy(get(m, "name")) {
					return a.provs["cinemeta"], typ, cinemetaCand(m), nil
				}
			}
		}
		if on("tvmaze") {
			j, err := a.net.getJSON(cfg.TvmazeBase+"/lookup/shows?imdb="+e.ID, reqInit{})
			if err != nil {
				keep(err)
			} else if truthy(get(j, "name")) {
				return a.provs["tvmaze"], "series", tvmazeCand(j), nil
			}
		}
	case "tmdb":
		if on("tmdb") {
			kind := "movie"
			if e.Kind == "series" {
				kind = "tv"
			}
			j, err := a.tmdbGet("/"+kind+"/"+e.ID, nil)
			if err != nil {
				keep(err)
			} else if truthy(get(j, "id")) {
				return a.provs["tmdb"], e.Kind, tmdbCand(j, kind), nil
			}
		}
	case "tvmaze":
		if on("tvmaze") {
			j, err := a.net.getJSON(cfg.TvmazeBase+"/shows/"+e.ID, reqInit{})
			if err != nil {
				keep(err)
			} else if truthy(get(j, "name")) {
				return a.provs["tvmaze"], "series", tvmazeCand(j), nil
			}
		}
	case "anilist":
		if on("anilist") {
			id, _ := strconv.Atoi(e.ID)
			body := jsonString(struct {
				Query     string         `json:"query"`
				Variables map[string]int `json:"variables"`
			}{"query ($id: Int) { Media(id: $id, type: ANIME) { " + anilistFields + " } }", map[string]int{"id": id}})
			j, err := a.net.getJSON(cfg.AnilistURL, reqInit{Method: "POST", Headers: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, Body: body})
			if err != nil {
				keep(err)
			} else if m := get(j, "data", "Media"); truthy(get(m, "siteUrl")) {
				typ := "series"
				if get(m, "format") == "MOVIE" {
					typ = "movie"
				}
				return a.provs["anilist"], typ, anilistCand(m), nil
			}
		}
	case "kitsu":
		if on("kitsu") {
			j, err := a.net.getJSON(cfg.KitsuBase+"/anime?filter[slug]="+encURI(e.ID), reqInit{Headers: map[string]string{"Accept": "application/vnd.api+json"}, NoRetryOnTimeout: true})
			if err != nil {
				keep(err)
			} else if d := asArr(get(j, "data")); len(d) > 0 {
				typ := "series"
				if get(d[0], "attributes", "subtype") == "movie" {
					typ = "movie"
				}
				return a.provs["kitsu"], typ, kitsuCand(d[0]), nil
			}
		}
	}
	return nil, "", nil, firstErr
}

// ---- the Fix this match dialog -------------------------------------------------------

// Suggestion is one row of the Fix this match dialog: a catalog entry the user can pin.
type Suggestion struct {
	Name    string `json:"name"`
	Year    int    `json:"year,omitempty"`
	Kind    string `json:"kind"`    // "movie" or "series"
	Catalog string `json:"catalog"` // the catalog's name, as the window shows it
	Poster  string `json:"poster,omitempty"`
	Entry   string `json:"entry"` // the catalog page a fix pins
}

// Cover is one of the other catalogs' covers of a pinned title.
type Cover struct {
	URL     string `json:"url"`
	Catalog string `json:"catalog"`
}

type suggestEntry struct {
	at     time.Time
	list   []Suggestion
	covers []Cover
}

// suggestTTL is how long the dialog's answers are kept: typing the same letters again asks nothing.
const suggestTTL = 5 * time.Minute

// maxSuggestCatalogs is how many catalogs one search of the dialog asks.
const maxSuggestCatalogs = 3

// suggestOrder are the catalogs the dialog may search, in the order they are chosen (Wikipedia, Jikan and Bulbapedia
// never: they are no place to pin an entry). Cinemeta lists IMDb's entries, so it stands in for IMDb when that is off.
var suggestOrder = []string{"tmdb", "imdb", "cinemeta", "tvmaze", "anilist", "kitsu"}

// SuggestCatalogs are the catalogs the dialog's searches ask (at most three, only those on under Privacy).
func (a *Artwork) SuggestCatalogs() []*Provider {
	cfg := a.cfg
	if !cfg.ShowArtwork {
		return nil
	}
	var out []*Provider
	for _, id := range suggestOrder {
		p := a.provs[id]
		if p == nil || !cfg.SourceOn(id) || (p.Enabled != nil && !p.Enabled(cfg)) {
			continue
		}
		if id == "cinemeta" && len(out) > 0 && out[len(out)-1].Label == "IMDb" {
			continue
		}
		out = append(out, p)
		if len(out) == maxSuggestCatalogs {
			break
		}
	}
	return out
}

func (a *Artwork) cached(key string) (suggestEntry, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	e, ok := a.suggested[key]
	if ok && time.Since(e.at) < suggestTTL {
		return e, true
	}
	return suggestEntry{}, false
}

func (a *Artwork) keep(key string, e suggestEntry) {
	e.at = time.Now()
	a.mu.Lock()
	if len(a.suggested) >= 100 {
		clear(a.suggested)
	}
	a.suggested[key] = e
	a.mu.Unlock()
}

// Suggest searches the dialog's catalogs for query and gives the entries found, closest names first, each once. An
// empty list with no error: nothing found, or every catalog is off.
func (a *Artwork) Suggest(query string) ([]Suggestion, error) {
	query = core.Trim(query)
	if u16len(query) < 2 {
		return nil, nil
	}
	key := "s|" + strings.ToLower(query)
	if e, ok := a.cached(key); ok {
		return e.list, nil
	}
	type job struct {
		p    *Provider
		typ  string
		list []*Cand
		err  error
	}
	var jobs []*job
	for _, p := range a.SuggestCatalogs() {
		for _, typ := range []string{"series", "movie"} {
			if p.handles(typ) {
				jobs = append(jobs, &job{p: p, typ: typ})
			}
		}
	}
	media := &core.Media{Title: query, Year: core.NA, Season: core.NA, Episode: core.NA, Index: core.NA, CodeEpisode: core.NA, BadgeEpisode: core.NA}
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					j.err = errors.New("the answer could not be read")
				}
			}()
			j.list, j.err = j.p.Search(a, j.typ, query, media)
		}()
	}
	wg.Wait()
	// round robin over the catalogs' lists, so each one's best answers come first; then exact names first
	var out []Suggestion
	seen := map[string]bool{}
	var failed error
	for i := 0; ; i++ {
		more := false
		for _, j := range jobs {
			if j.err != nil {
				if failed == nil {
					failed = j.err
				}
				continue
			}
			if i >= len(j.list) {
				continue
			}
			more = true
			c := j.list[i]
			e, ok := core.EntryPage(c.URL)
			if !ok || len(c.Titles) == 0 || seen[c.URL] {
				continue
			}
			seen[c.URL] = true
			// the catalog whose page it is (TVmaze lists a show by its IMDb page when it knows it)
			s := Suggestion{Name: c.Titles[0], Year: c.Year, Kind: j.typ, Catalog: entryLabels[e.Catalog], Entry: c.URL}
			if len(c.Poster) <= 512 {
				s.Poster = c.Poster
			}
			out = append(out, s)
		}
		if !more {
			break
		}
	}
	nq := normTitle(query)
	rank := func(s Suggestion) int {
		n := normTitle(s.Name)
		switch {
		case n == nq:
			return 0
		case strings.HasPrefix(n, nq):
			return 1
		}
		return 2
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	if len(out) > 12 {
		out = out[:12]
	}
	if len(out) == 0 && failed != nil {
		return nil, failed
	}
	a.keep(key, suggestEntry{list: out})
	return out, nil
}

// Covers finds the covers of one title in the other catalogs that are on (not except, the catalog of the pinned
// entry): one search each, at most five covers, each once.
func (a *Artwork) Covers(name string, year int, kind, except string) []Cover {
	name = core.Trim(name)
	if name == "" || (kind != "movie" && kind != "series") || !a.cfg.ShowArtwork {
		return nil
	}
	key := "c|" + strings.ToLower(name) + "|" + strconv.Itoa(year) + "|" + kind + "|" + except
	if e, ok := a.cached(key); ok {
		return e.covers
	}
	cfg := a.cfg
	type job struct {
		p    *Provider
		list []*Cand
	}
	var jobs []*job
	for _, id := range suggestOrder {
		p := a.provs[id]
		if p == nil || p.Label == except || !p.handles(kind) || !cfg.SourceOn(id) || (p.Enabled != nil && !p.Enabled(cfg)) {
			continue
		}
		jobs = append(jobs, &job{p: p})
	}
	y := core.NA
	if year > 0 {
		y = year
	}
	media := &core.Media{Title: name, Year: y, Season: core.NA, Episode: core.NA, Index: core.NA, CodeEpisode: core.NA, BadgeEpisode: core.NA}
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = recover() }()
			j.list, _ = j.p.Search(a, kind, name, media)
		}()
	}
	wg.Wait()
	var out []Cover
	seen := map[string]bool{}
	for _, j := range jobs {
		var usable []*Cand
		for _, c := range j.list {
			if c.Poster != "" && u16len(c.Poster) <= 256 {
				usable = append(usable, c)
			}
		}
		c := pickMatch(usable, name, year, kind == "movie", PickOpts{Earliest: j.p.Earliest, ExactOnly: j.p.ExactOnly, TrustAlias: j.p.TrustAlias})
		if c == nil || seen[c.Poster] {
			continue
		}
		seen[c.Poster] = true
		out = append(out, Cover{URL: c.Poster, Catalog: j.p.Label})
		if len(out) == 5 {
			break
		}
	}
	a.keep(key, suggestEntry{covers: out})
	return out
}

// isPinned reports whether found is the entry fx pins (and not what a search found after the pinned page failed).
func isPinned(fx *core.MatchFix, found *core.Art) bool {
	e, ok := core.EntryPage(fx.Entry)
	if !ok || found == nil || found.Source == "override" {
		return false
	}
	if strings.Contains(found.URL, e.ID) {
		return true
	}
	ids := found.IDs
	return ids != nil && (ids.Imdb == e.ID || ids.Tvmaze == e.ID || ids.Tmdb == e.ID || ids.Kitsu == e.ID)
}

// entryLabels name the catalogs of the pages a fix can pin, as the window shows them.
var entryLabels = map[string]string{"imdb": "IMDb", "tmdb": "TMDB", "tvmaze": "TVmaze", "anilist": "AniList", "kitsu": "Kitsu"}

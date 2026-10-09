package artwork

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

// Looks up the real title of the episode being watched when the filename doesn't carry one
// ("Show Name Ep03" -> "S01E03 · Episode title").

var reGenericEp = re(`^(?:episode|ep\.?|folge|épisode|capítulo|capitulo)\s*#?\s*\d+(?:\.\d+)?$|^#?\d+$|^tb[ad]$`, "i")

func usable(n string) bool { return n != "" && !reGenericEp.Test(core.Trim(n)) }

// EpItem is one entry of a show's episode list.
type EpItem struct {
	Season, Number float64 // NaN = missing
	Name           string
	Entry          bool
}

type pickOpts struct {
	split     bool
	seasonNo  int // NA
	codeBlock bool
}

type picked struct {
	e        EpItem
	absolute bool
}

func nan() float64 { return math.NaN() }

func epSeasonIs(x EpItem, s int) bool { return x.Season == float64(s) }

// pickEpisode selects the episode `media` names from a list of regular episodes.
func pickEpisode(list []EpItem, media *core.Media, o pickOpts) *picked {
	n := media.Episode
	if n == core.NA || len(list) == 0 {
		return nil
	}
	nf := float64(n)
	if o.codeBlock && media.Season != core.NA {
		count := func(sn int) int {
			c := 0
			for _, x := range list {
				if epSeasonIs(x, sn) {
					c++
				}
			}
			return c
		}
		here := count(media.Season)
		off := 0
		for k := 0; media.Season-k >= 1; k++ {
			rel := n - off
			if rel >= 1 && rel <= here {
				for _, x := range list {
					if epSeasonIs(x, media.Season) && x.Number == float64(rel) {
						return &picked{x, false}
					}
				}
				return nil
			}
			prev := count(media.Season - k - 1)
			if prev == 0 {
				break
			}
			off += prev
		}
		return nil
	}
	var seasons []float64
	seen := map[float64]bool{}
	allEntry := true
	for _, x := range list {
		if !seen[x.Season] {
			seen[x.Season] = true
			seasons = append(seasons, x.Season)
		}
		if !x.Entry {
			allEntry = false
		}
	}
	sort.SliceStable(seasons, func(i, j int) bool { return seasons[i] < seasons[j] })
	// A catalog entry that is one season of a franchise ("<Franchise> Part 2") must not borrow a multi-season show's season 1.
	if o.split && len(seasons) > 1 && media.Season != 1 && !allEntry {
		return nil
	}
	s := o.seasonNo
	if o.split {
		s = 1
	} else if media.Season != core.NA {
		s = media.Season
	}
	if s != core.NA {
		for _, x := range list {
			if epSeasonIs(x, s) && x.Number == nf {
				return &picked{x, false}
			}
		}
		if o.split && len(seasons) == 1 {
			for _, x := range list {
				if x.Number == nf {
					return &picked{x, false}
				}
			}
		}
		return nil
	}
	if len(seasons) == 1 {
		for _, x := range list {
			if x.Number == nf {
				return &picked{x, false}
			}
		}
		return nil
	}
	// Several seasons and the filename names none: assume absolute numbering ("Show - 30").
	sorted := append([]EpItem(nil), list...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if d := a.Season - b.Season; d != 0 && !math.IsNaN(d) {
			return d < 0
		}
		d := a.Number - b.Number
		return d < 0 && !math.IsNaN(d)
	})
	if n >= 1 && n-1 < len(sorted) {
		return &picked{sorted[n-1], true}
	}
	return nil
}

// Bulbapedia's list pages: {{Episodelistbody|AB045|Episode title|<jp>|<translation>|<us date>|<jp date>}}
var bulbapediaPages = map[string]string{
	"XY": "List_of_Pokémon_the_Series:_XY_episodes",
	"BW": "List_of_Pokémon_the_Series:_Black_&_White_episodes",
}

// Series whose catalog numbering is known to differ from the production codes.
var noCatalogFallback = map[string]bool{"XY": true}

var (
	reEpBody  = re(`\{\{Episodelistbody\|([A-Z]{2,4}\d{3})\|([^\n]*)`, "g")
	reClean1  = re(`\{\{(?:tt|ruby)\|[^|}]*\|([^}]*)\}\}`, "g")
	reClean2  = re(`\{\{[a-z]+\|([^|}]*)(?:\|[^}]*)?\}\}`, "gi")
	reClean3  = re(`\[\[(?:[^|\]]*\|)?([^\]]*)\]\]`, "g")
	reClean4  = re(`<[^>]+>|'''?|\{\{|\}\}`, "g")
	reNbsp    = re(`&nbsp;`, "g")
	reWS      = re(`\s+`, "g")
	rePokemon = re(`^pok[eé]mon$`, "i")
	reNonAZ   = re(`[^A-Za-z]`, "g")
	reBang    = re(`[!?](?=[A-Z])`, "")
)

func cleanWiki(s string) string {
	s = reClean1.ReplaceStr(s, "$1")
	s = reClean2.ReplaceStr(s, "$1")
	s = reClean3.ReplaceStr(s, "$1")
	s = reClean4.ReplaceStr(s, "")
	s = reNbsp.ReplaceStr(s, " ")
	s = reWS.ReplaceStr(s, " ")
	return core.Trim(s)
}

// parseEpisodeList reads a Bulbapedia list page: production code -> English title (first wins).
func parseEpisodeList(wikitext string) map[string]string {
	out := map[string]string{}
	for _, m := range reEpBody.FindAll(wikitext) {
		rest := []rune(m.Str(2))
		depth, end := 0, len(rest)
		for i := 0; i < len(rest); i++ {
			two := ""
			if i+1 < len(rest) {
				two = string(rest[i : i+2])
			} else {
				two = string(rest[i:])
			}
			if two == "{{" || two == "[[" {
				depth++
				i++
			} else if two == "}}" || two == "]]" {
				depth--
				i++
			} else if rest[i] == '|' && depth <= 0 {
				end = i
				break
			}
		}
		title := cleanWiki(string(rest[:end]))
		if _, has := out[m.Str(1)]; title != "" && !has {
			out[m.Str(1)] = title
		}
	}
	return out
}

type epCtx struct {
	query     string
	split     bool
	seasonNo  int // NA
	codeBlock bool
}

type epProvider struct {
	label  string
	byCode bool
	enable func(cfg *core.Config) bool
	list   func(r *Resolver, media *core.Media, ids core.ArtIDs, ctx epCtx) ([]EpItem, error)
}

type bulbaPage struct {
	until  time.Time
	titles map[string]string
}

// Resolver finds episode titles.
type Resolver struct {
	a     *Artwork
	cfg   *core.Config
	provs map[string]*epProvider

	mu    sync.Mutex
	bulba map[string]*bulbaPage
}

func newResolver(a *Artwork) *Resolver {
	r := &Resolver{a: a, cfg: a.cfg, bulba: map[string]*bulbaPage{}}
	r.provs = r.buildProviders()
	return r
}

func (r *Resolver) enc(s string) string { return encURI(plain(s)) }

func epNumF(v any) float64 {
	if f, ok := numOK(v); ok {
		return f
	}
	return nan()
}

func (r *Resolver) buildProviders() map[string]*epProvider {
	P := map[string]*epProvider{}
	cfg := r.cfg
	a := r.a

	// Official API; needs a free key. Handles shows whose season is known.
	P["tmdb"] = &epProvider{label: "TMDB", enable: func(c *core.Config) bool { return c.TmdbAPIKey != "" },
		list: func(r *Resolver, media *core.Media, ids core.ArtIDs, ctx epCtx) ([]EpItem, error) {
			season := ctx.seasonNo
			if ctx.split {
				season = 1
			} else if media.Season != core.NA {
				season = media.Season
			}
			if season == core.NA {
				return nil, nil
			}
			id := ids.Tmdb
			if id == "" {
				q := params{}
				headers := tmdbAuth(cfg, &q)
				q.set("query", ctx.query)
				j, err := a.net.getJSON("https://api.themoviedb.org/3/search/tv?"+q.String(), reqInit{Headers: headers})
				if err != nil {
					return nil, err
				}
				var cands []*Cand
				for _, x := range asArr(get(j, "results")) {
					cands = append(cands, &Cand{Titles: nonEmptyTitles(get(x, "name"), get(x, "original_name")), Year: yearOf(get(x, "first_air_date")), ID: str(get(x, "id"))})
				}
				if c := pickMatch(cands, ctx.query, media.Year0(), false, PickOpts{ExactOnly: true}); c != nil && c.ID != "" {
					id = c.ID
				}
			}
			if id == "" {
				return nil, nil
			}
			q := params{}
			headers := tmdbAuth(cfg, &q)
			j, err := a.net.getJSON("https://api.themoviedb.org/3/tv/"+id+"/season/"+core.Itoa(season)+"?"+q.String(), reqInit{Headers: headers})
			if err != nil {
				return nil, err
			}
			var out []EpItem
			for _, e := range asArr(get(j, "episodes")) {
				out = append(out, EpItem{Season: float64(season), Number: epNumF(get(e, "episode_number")), Name: str(get(e, "name"))})
			}
			return out, nil
		}}

	// Keyless, covers most TV (and plenty of anime). Specials have no number and are skipped.
	P["tvmaze"] = &epProvider{label: "TVmaze",
		list: func(r *Resolver, media *core.Media, ids core.ArtIDs, ctx epCtx) ([]EpItem, error) {
			id := ids.Tvmaze
			if id == "" && ids.Imdb != "" {
				j, err := a.net.getJSON(cfg.TvmazeBase+"/lookup/shows?imdb="+ids.Imdb, reqInit{})
				if err != nil {
					return nil, err
				}
				if truthy(get(j, "id")) {
					id = str(get(j, "id"))
				}
			}
			if id == "" {
				arr, err := a.net.getJSON(cfg.TvmazeBase+"/search/shows?q="+r.enc(ctx.query), reqInit{})
				if err != nil {
					return nil, err
				}
				var cands []*Cand
				for _, x := range asArr(arr) {
					s := get(x, "show")
					if truthy(s) {
						cands = append(cands, &Cand{Titles: nonEmptyTitles(get(s, "name")), Year: yearOf(get(s, "premiered")), ID: str(get(s, "id"))})
					}
				}
				if c := pickMatch(cands, ctx.query, media.Year0(), false, PickOpts{ExactOnly: true}); c != nil {
					id = c.ID
				}
			}
			if id == "" {
				return nil, nil
			}
			eps, err := a.net.getJSON(cfg.TvmazeBase+"/shows/"+id+"/episodes", reqInit{})
			if err != nil {
				return nil, err
			}
			var out []EpItem
			for _, e := range asArr(eps) {
				if !truthy(e) || get(e, "number") == nil {
					continue
				}
				s, ok := numOK(get(e, "season"))
				if !ok || s < 1 {
					continue
				}
				if t := get(e, "type"); truthy(t) && t != "regular" {
					continue
				}
				out = append(out, EpItem{Season: s, Number: epNumF(get(e, "number")), Name: str(get(e, "name"))})
			}
			return out, nil
		}}

	// Production codes (AB065, CD012...) are Bulbapedia's own episode numbers.
	P["bulbapedia"] = &epProvider{label: "Bulbapedia", byCode: true,
		list: func(r *Resolver, media *core.Media, ids core.ArtIDs, ctx epCtx) ([]EpItem, error) {
			prefix := media.CodePrefix
			page := bulbapediaPages[prefix]
			if prefix == "" || page == "" || media.Code == "" || !rePokemon.Test(reNonAZ.ReplaceStr(plain(media.Title), "")) {
				return nil, nil
			}
			n := media.Episode
			if media.CodeEpisode != core.NA {
				n = media.CodeEpisode
			}
			ns := core.Itoa(n)
			for len(ns) < 3 {
				ns = "0" + ns
			}
			code := prefix + ns
			r.mu.Lock()
			pg := r.bulba[page]
			r.mu.Unlock()
			if pg == nil || pg.until.Before(time.Now()) {
				headers := map[string]string{"User-Agent": "MPCvibedRPC/1.0 (Discord rich presence for MPC-HC; reads episode titles)", "Accept": "application/json, text/plain"}
				api := cfg.BulbapediaBase + "/w/api.php?action=query&prop=revisions&rvprop=content&rvslots=main&format=json&formatversion=2&titles=" + encURI(page)
				status, text, err := a.net.fetchText(api, headers, 10*time.Second)
				if err != nil {
					return nil, err
				}
				if status < 200 || status > 299 {
					status, text, err = a.net.fetchText(cfg.BulbapediaBase+"/w/index.php?title="+encURI(page)+"&action=raw", headers, 10*time.Second)
					if err != nil {
						return nil, err
					}
				}
				if status < 200 || status > 299 {
					return nil, &FetchError{Name: "Error", Msg: "HTTP " + core.Itoa(status), Status: status}
				}
				if j, perr := parseJSON(text); perr == nil {
					pages := get(j, "query", "pages")
					var rev any
					if arr, ok := pages.([]any); ok {
						if len(arr) > 0 {
							rev = arr[0]
						}
					} else if pages != nil {
						rev = pages
					}
					var slot any
					if revs := asArr(get(rev, "revisions")); len(revs) > 0 {
						slot = revs[0]
					}
					c := ""
					if truthy(slot) {
						switch {
						case truthy(get(slot, "slots", "main")):
							c = str(firstTruthy(get(slot, "slots", "main", "content"), get(slot, "slots", "main", "*")))
							if c == "" {
								c = str(firstTruthy(get(slot, "content"), get(slot, "*")))
							}
						default:
							c = str(firstTruthy(get(slot, "content"), get(slot, "*")))
						}
					}
					text = c
				}
				pg = &bulbaPage{until: time.Now().Add(6 * time.Hour), titles: parseEpisodeList(text)}
				r.mu.Lock()
				r.bulba[page] = pg
				r.mu.Unlock()
			}
			name, ok := pg.titles[code]
			if !ok {
				return nil, nil
			}
			season := 1
			if ctx.split {
				season = 1
			} else if media.Season != core.NA {
				season = media.Season
			}
			return []EpItem{{Season: float64(season), Number: float64(media.Episode), Name: name}}, nil
		}}

	// Anime entries found on Kitsu: that entry's own episode list (numbering as the entry counts it).
	P["kitsu"] = &epProvider{label: "Kitsu",
		list: func(r *Resolver, media *core.Media, ids core.ArtIDs, ctx epCtx) ([]EpItem, error) {
			if ids.Kitsu == "" || media.Episode == core.NA {
				return nil, nil
			}
			if !ctx.split && media.Season != core.NA && media.Season > 1 {
				return nil, nil
			}
			j, err := a.net.getJSON(cfg.KitsuBase+"/anime/"+ids.Kitsu+"/episodes?filter[number]="+core.Itoa(media.Episode)+"&page[limit]=2",
				reqInit{Headers: map[string]string{"Accept": "application/vnd.api+json"}, NoRetryOnTimeout: true})
			if err != nil {
				return nil, err
			}
			season := 1
			if !ctx.split && media.Season != core.NA {
				season = media.Season
			}
			var out []EpItem
			for _, d := range asArr(get(j, "data")) {
				x := get(d, "attributes")
				if get(x, "number") == nil {
					continue
				}
				t := get(x, "titles")
				raw := str(firstTruthy(get(t, "en_us"), get(t, "en"), get(x, "canonicalTitle"), get(t, "en_jp"), ""))
				name := raw
				if m := reBang.Exec(raw); m != nil && m.Index() > 0 {
					name = string([]rune(raw)[:m.Index()+1])
				}
				out = append(out, EpItem{Season: float64(season), Number: epNumF(get(x, "number")), Name: name, Entry: true})
			}
			return out, nil
		}}

	// Stremio's IMDb-keyed catalog: full episode list per IMDb id.
	P["cinemeta"] = &epProvider{label: "Cinemeta",
		list: func(r *Resolver, media *core.Media, ids core.ArtIDs, ctx epCtx) ([]EpItem, error) {
			imdb := ids.Imdb
			if imdb == "" {
				j, err := a.net.getJSON(cfg.CinemetaBase+"/catalog/series/top/search="+r.enc(ctx.query)+".json", reqInit{})
				if err != nil {
					return nil, err
				}
				var cands []*Cand
				for _, m := range asArr(get(j, "metas")) {
					cands = append(cands, &Cand{Titles: nonEmptyTitles(get(m, "name")), Year: yearOf(get(m, "releaseInfo")), ID: str(firstTruthy(get(m, "imdb_id"), get(m, "id")))})
				}
				if c := pickMatch(cands, ctx.query, media.Year0(), false, PickOpts{ExactOnly: true}); c != nil && reTT.Test(c.ID) {
					imdb = c.ID
				}
			}
			if imdb == "" {
				return nil, nil
			}
			j, err := a.net.getJSON(cfg.CinemetaBase+"/meta/series/"+imdb+".json", reqInit{})
			if err != nil {
				return nil, err
			}
			var out []EpItem
			for _, v := range asArr(get(j, "meta", "videos")) {
				num := get(v, "episode")
				if num == nil {
					num = get(v, "number")
				}
				s, ok := numOK(get(v, "season"))
				if !ok || s < 1 || num == nil {
					continue
				}
				out = append(out, EpItem{Season: s, Number: epNumF(num), Name: str(firstTruthy(get(v, "name"), get(v, "title")))})
			}
			return out, nil
		}}
	return P
}

// sources is the episode-title services in the order to try them, without those switched off in the settings.
func (r *Resolver) sources() []string {
	var out []string
	for _, n := range r.cfg.EpisodeSources {
		if r.cfg.SourceOn(n) {
			out = append(out, n)
		}
	}
	return out
}

// Find returns the episode the media names, or nil. hit is the artwork match for the show (may be nil).
func (r *Resolver) Find(media *core.Media, hit *core.Art, query string, warn func(key, msg string)) *core.ArtEpisode {
	cfg := r.cfg
	if !cfg.EpisodeTitles || media == nil || !media.IsEpisode || media.Episode == core.NA || (media.EpTitle != "" && media.EpTitleBase == "") {
		return nil
	}
	if hit != nil && hit.Type == "movie" {
		return nil
	}
	var ids core.ArtIDs
	if hit != nil && hit.IDs != nil {
		ids = *hit.IDs
	}
	if hit != nil && ids.Imdb == "" {
		ids.Imdb = imdbIDFrom(hit.URL)
	}
	srcs := r.sources()
	isPokemon := func() bool { return rePokemon.Test(reNonAZ.ReplaceStr(plain(media.Title), "")) }
	if media.Code != "" && core.Contains(srcs, "bulbapedia") {
		list, err := r.provs["bulbapedia"].list(r, media, ids, epCtx{split: hit != nil && hit.Split, seasonNo: core.NA})
		if err != nil {
			warn("Bulbapedia", "Bulbapedia episode lookup failed ("+err.Error()+"); no episode title for this one.")
		} else if len(list) > 0 && usable(list[0].Name) {
			e := list[0]
			return &core.ArtEpisode{Season: int(e.Season), Number: int(e.Number), Title: e.Name, Source: "Bulbapedia", Absolute: false, Via: media.CodePrefix}
		}
		// TV catalogs number Pokémon in broadcast order, off from the production codes: a Pokémon series Bulbapedia
		// covers gets no fallback.
		if isPokemon() && noCatalogFallback[media.CodePrefix] {
			return nil
		}
	}
	// A catalog entry that is one season of a franchise the TV catalogs know by its own name and season numbers.
	if hit != nil && hit.Split && media.Code != "" && media.Season != core.NA {
		cm := media
		if media.CodeEpisode != core.NA {
			c2 := *media
			c2.Episode = media.CodeEpisode
			cm = &c2
		}
		q := query
		if q == "" {
			q = media.Title
		}
		fctx := epCtx{query: q, split: false, seasonNo: core.NA, codeBlock: true}
		for _, name := range srcs {
			p := r.provs[name]
			if p == nil || name == "kitsu" || p.byCode {
				continue
			}
			if p.enable != nil && !p.enable(cfg) {
				continue
			}
			i2 := ids
			i2.Kitsu = ""
			list, err := p.list(r, cm, i2, fctx)
			if err != nil {
				warn(p.label, p.label+" episode lookup failed ("+err.Error()+"); trying the next source.")
				continue
			}
			other := false
			for _, x := range list {
				if !epSeasonIs(x, media.Season) {
					other = true
				}
			}
			if other {
				if pk := pickEpisode(list, cm, pickOpts{split: fctx.split, seasonNo: fctx.seasonNo, codeBlock: true}); pk != nil && usable(pk.e.Name) {
					return &core.ArtEpisode{Season: int(pk.e.Season), Number: int(pk.e.Number), Title: core.Trim(pk.e.Name), Source: p.label, Absolute: false, Via: fctx.query}
				}
			}
		}
	}
	ctx := epCtx{query: query, seasonNo: core.NA}
	if ctx.query == "" {
		ctx.query = media.Title
	}
	if hit != nil && hit.Split && hit.Name != "" {
		ctx.query = hit.Name
	}
	ctx.split = hit != nil && hit.Split
	if hit != nil && hit.SeasonNo != core.NA {
		ctx.seasonNo = hit.SeasonNo
	}
	try := func(ctx epCtx) *core.ArtEpisode {
		for _, name := range srcs {
			p := r.provs[name]
			if p == nil || p.byCode {
				continue
			}
			if p.enable != nil && !p.enable(cfg) {
				continue
			}
			list, err := p.list(r, media, ids, ctx)
			if err != nil {
				warn(p.label, p.label+" episode lookup failed ("+err.Error()+"); trying the next source.")
				continue
			}
			pk := pickEpisode(list, media, pickOpts{split: ctx.split, seasonNo: ctx.seasonNo, codeBlock: ctx.codeBlock})
			if pk != nil && usable(pk.e.Name) {
				return &core.ArtEpisode{Season: intOrNA(pk.e.Season), Number: intOrNA(pk.e.Number), Title: core.Trim(pk.e.Name), Source: p.label, Absolute: pk.absolute, Via: ctx.query}
			}
		}
		return nil
	}
	if ep := try(ctx); ep != nil {
		return ep
	}
	// A show matched on an anime catalog is often known to the TV catalogs by another name (romaji in the file name,
	// English at TVmaze): try its English name. Only for a file without a season number, matched to a whole show that
	// the TV catalogs are asked about by name (no IMDb or TVmaze id).
	if hit == nil || hit.Alt == nil || ids.Imdb != "" || ids.Tvmaze != "" || ctx.split || media.Season != core.NA {
		return nil
	}
	if en := hit.Alt.English; en != "" && core.Lower(plain(en)) != core.Lower(plain(ctx.query)) {
		c := ctx
		c.query = en
		return try(c)
	}
	return nil
}

func intOrNA(f float64) int {
	if math.IsNaN(f) {
		return core.NA
	}
	return int(f)
}

var _ = strings.TrimSpace
var _ = jsre.NFC

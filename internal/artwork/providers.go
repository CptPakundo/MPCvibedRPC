package artwork

import (
	"sort"
	"strconv"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

var (
	reTT        = re(`^tt\d+$`, "")
	reShard     = re(`^[a-z0-9]`, "")
	reHebrew    = re(`[֐-׿]`, "")
	reWikiTitle = re(`^(.*?)\s*\(([^)]*)\)\s*$`, "")
	reWikiKind  = re(`tv|television|series|anime|film|movie|sitcom|miniseries|manga|franchise|show`, "i")
	reHr        = re(`(\d+)\s*hr`, "")
	reMin       = re(`(\d+)\s*min`, "")
	reImdbID    = re(`imdb\.com/title/(tt\d+)`, "")
)

func yearOf(v any) int {
	s := str(v)
	if truthy(v) {
		s = u16head(s, 4)
	} else {
		s = ""
	}
	n, ok := core.ParseIntPrefix(s)
	if !ok {
		return 0
	}
	return n
}

func u16head(s string, n int) string { return core.U16Slice(s, 0, n) }

func imdbURL(id string) string {
	if reTT.Test(id) {
		return "https://www.imdb.com/title/" + id + "/"
	}
	return ""
}

func imdbIDFrom(url string) string {
	if m := reImdbID.Exec(url); m != nil {
		return m.Str(1)
	}
	return ""
}

// SeasonInfo is one season of a show.
type SeasonInfo struct {
	Poster string
	Name   string
	Number int
}

// Provider is one catalog.
type Provider struct {
	Label                                   string
	Types                                   []string
	Earliest, ExactOnly, TrustAlias, Ranked bool
	Enabled                                 func(cfg *core.Config) bool
	Search                                  func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error)
	Season                                  func(a *Artwork, cand *Cand, media *core.Media) (*SeasonInfo, error)
}

func (p *Provider) handles(t string) bool {
	for _, x := range p.Types {
		if x == t {
			return true
		}
	}
	return false
}

func tmdbAuth(cfg *core.Config, q *params) map[string]string {
	h := map[string]string{}
	if u16len(cfg.TmdbAPIKey) > 40 {
		h["Authorization"] = "Bearer " + cfg.TmdbAPIKey // v4 token
	} else {
		q.set("api_key", cfg.TmdbAPIKey)
	}
	return h
}

func posterIf(path string) string {
	if path != "" {
		return "https://image.tmdb.org/t/p/w500" + path
	}
	return ""
}

func nonEmptyTitles(vals ...any) []string {
	var out []string
	for _, v := range vals {
		if truthy(v) {
			out = append(out, str(v))
		}
	}
	return out
}

func optPositive(v any) int {
	if f, ok := numOK(v); ok && f > 0 {
		return int(f)
	}
	return 0
}

func yearNum(v any) int {
	if f, ok := numOK(v); ok && f != 0 {
		return int(f)
	}
	return 0
}

func (a *Artwork) providers() map[string]*Provider { return a.provs }

func buildProviders() map[string]*Provider {
	P := map[string]*Provider{}

	// Official API, best data. Needs a free key (tmdbApiKey); skipped without one.
	P["tmdb"] = &Provider{
		Label: "TMDB", Types: []string{"movie", "series"},
		Enabled: func(cfg *core.Config) bool { return cfg.TmdbAPIKey != "" },
		Search: func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error) {
			kind := "movie"
			if typ == "series" {
				kind = "tv"
			}
			q := params{{"query", query}}
			if kind == "movie" && truthy2(media.Year) {
				q.set("year", core.Itoa(media.Year))
			}
			headers := tmdbAuth(a.cfg, &q)
			j, err := a.net.getJSON("https://api.themoviedb.org/3/search/"+kind+"?"+q.String(), reqInit{Headers: headers})
			if err != nil {
				return nil, err
			}
			var out []*Cand
			for _, r := range asArr(get(j, "results")) {
				c := &Cand{
					Titles:   nonEmptyTitles(get(r, "name"), get(r, "title"), get(r, "original_name"), get(r, "original_title")),
					Year:     yearOf(firstTruthy(get(r, "first_air_date"), get(r, "release_date"))),
					URL:      "https://www.themoviedb.org/" + kind + "/" + tmpl(get(r, "id")),
					URLLabel: "TMDB", Ref: str(get(r, "id")), Kind: kind,
				}
				if truthy(get(r, "poster_path")) {
					c.Poster = posterIf(str(get(r, "poster_path")))
				}
				out = append(out, c)
			}
			return out, nil
		},
		Season: func(a *Artwork, cand *Cand, media *core.Media) (*SeasonInfo, error) {
			if media.Season == core.NA || cand.Kind != "tv" {
				return nil, nil
			}
			q := params{}
			headers := tmdbAuth(a.cfg, &q)
			j, err := a.net.getJSON("https://api.themoviedb.org/3/tv/"+cand.Ref+"/season/"+core.Itoa(media.Season)+"?"+q.String(), reqInit{Headers: headers})
			if err != nil {
				return nil, err
			}
			s := &SeasonInfo{Name: orStr(get(j, "name"), ""), Number: media.Season}
			if truthy(get(j, "poster_path")) {
				s.Poster = posterIf(str(get(j, "poster_path")))
			}
			return s, nil
		},
	}

	// Stremio's keyless IMDb-id catalog.
	P["cinemeta"] = &Provider{
		Label: "Cinemeta", Types: []string{"movie", "series"},
		Search: func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error) {
			j, err := a.net.getJSON(a.cfg.CinemetaBase+"/catalog/"+typ+"/top/search="+encURI(plain(query))+".json", reqInit{})
			if err != nil {
				return nil, err
			}
			var out []*Cand
			for _, m := range asArr(get(j, "metas")) {
				out = append(out, &Cand{
					Titles: nonEmptyTitles(get(m, "name")), Year: yearOf(get(m, "releaseInfo")), Poster: str(get(m, "poster")),
					URL: imdbURL(str(firstTruthy(get(m, "imdb_id"), get(m, "id")))), URLLabel: "IMDb",
				})
			}
			return out, nil
		},
	}

	// IMDb's own search-box backend (unofficial, keyless). Finds titles Cinemeta's search misses.
	P["imdb"] = &Provider{
		Label: "IMDb", Types: []string{"movie", "series"}, TrustAlias: true, Ranked: true,
		Search: func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error) {
			q := jsre.ToLower(plain(query))
			shard := "x"
			if reShard.Test(q) {
				shard = firstRune(q)
			}
			j, err := a.net.getJSON(a.cfg.ImdbSuggestBase+"/suggestion/"+shard+"/"+encURI(q)+".json", reqInit{})
			if err != nil {
				return nil, err
			}
			want := []string{"movie", "feature", "tvMovie", "video", "tvSpecial", "tvShort", "short"}
			if typ == "series" {
				want = []string{"tvSeries", "tvMiniSeries"}
			}
			var list []*Cand
			for _, d := range asArr(get(j, "d")) {
				qid := str(get(d, "qid"))
				if !core.Contains(want, qid) || get(d, "qid") == nil {
					continue
				}
				c := &Cand{Titles: nonEmptyTitles(get(d, "l")), Year: yearNum(get(d, "y")), URL: imdbURL(str(get(d, "id"))), URLLabel: "IMDb"}
				if truthy(get(d, "i")) {
					c.Poster = str(get(d, "i", "imageUrl"))
				}
				c.l0 = tmpl(get(d, "l"))
				list = append(list, c)
			}
			if len(list) > 0 {
				qw := map[string]bool{}
				for _, w := range words(query) {
					qw[w] = true
				}
				some := false
				for _, w := range words(list[0].l0) {
					if qw[w] {
						some = true
					}
				}
				if !some {
					list[0].ViaAlias = true
				}
			}
			return list, nil
		},
	}

	// Worldwide TV (including non-English shows). Keyless.
	P["tvmaze"] = &Provider{
		Label: "TVmaze", Types: []string{"series"},
		Search: func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error) {
			j, err := a.net.getJSON(a.cfg.TvmazeBase+"/search/shows?q="+encURI(plain(query)), reqInit{})
			if err != nil {
				return nil, err
			}
			var out []*Cand
			for _, r := range asArr(j) {
				s := get(r, "show")
				if !truthy(s) {
					continue
				}
				c := &Cand{Titles: nonEmptyTitles(get(s, "name")), Year: yearOf(get(s, "premiered")), Ref: str(get(s, "id")), ID: str(get(s, "id"))}
				if truthy(get(s, "image")) {
					c.Poster = orStr(get(s, "image", "medium"), str(get(s, "image", "original")))
				}
				if u := imdbURL(str(get(s, "externals", "imdb"))); u != "" {
					c.URL, c.URLLabel = u, "IMDb"
				} else {
					c.URL, c.URLLabel = str(get(s, "url")), "TVmaze"
				}
				out = append(out, c)
			}
			return out, nil
		},
		// Lists every season of the show with its own name and poster (e.g. a show whose season 14 is named "Black & White").
		Season: func(a *Artwork, cand *Cand, media *core.Media) (*SeasonInfo, error) {
			j, err := a.net.getJSON(a.cfg.TvmazeBase+"/shows/"+cand.Ref+"/seasons", reqInit{})
			if err != nil {
				return nil, err
			}
			list := asArr(j)
			var found any
			if media.Season != core.NA {
				for _, x := range list {
					if f, ok := numOK(get(x, "number")); ok && truthy(x) && int(f) == media.Season && f == float64(int(f)) {
						found = x
						break
					}
				}
			}
			if found == nil && media.CodePrefix != "" {
				found = matchSeasonByCode(list, media.CodePrefix)
			}
			if found == nil {
				return nil, nil
			}
			s := &SeasonInfo{Name: orStr(get(found, "name"), ""), Number: core.NA}
			if f, ok := numOK(get(found, "number")); ok {
				s.Number = int(f)
			}
			if truthy(get(found, "image")) {
				s.Poster = orStr(get(found, "image", "medium"), str(get(found, "image", "original")))
			}
			return s, nil
		},
	}

	// Anime: matches romaji, English and native titles plus synonyms. Keyless GraphQL.
	P["anilist"] = &Provider{
		Label: "AniList", Types: []string{"movie", "series"}, Earliest: true,
		Search: func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error) {
			gql := "query ($s: String!) { Page(perPage: 25) { media(search: $s, type: ANIME, sort: SEARCH_MATCH) { " +
				"siteUrl format episodes duration genres averageScore startDate { year } title { romaji english native } synonyms coverImage { large } " +
				"staff(perPage: 8, sort: RELEVANCE) { edges { role node { name { full } } } } } } }"
			body := jsonString(struct {
				Query     string            `json:"query"`
				Variables map[string]string `json:"variables"`
			}{gql, map[string]string{"s": query}})
			j, err := a.net.getJSON(a.cfg.AnilistURL, reqInit{Method: "POST", Headers: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, Body: body})
			if err != nil {
				return nil, err
			}
			var out []*Cand
			for _, m := range asArr(get(j, "data", "Page", "media")) {
				if (typ == "movie") != (get(m, "format") == "MOVIE") {
					continue
				}
				titles := nonEmptyTitles(get(m, "title", "romaji"), get(m, "title", "english"), get(m, "title", "native"))
				titles = append(titles, nonEmptyTitles(asArr(get(m, "synonyms"))...)...)
				c := &Cand{Titles: titles, Year: yearNum(get(m, "startDate", "year")), URL: str(get(m, "siteUrl")), URLLabel: "AniList",
					Runtime: optPositive(get(m, "duration")), Eps: optPositive(get(m, "episodes")),
					Alt:     &core.ArtAlt{English: orStr(get(m, "title", "english"), ""), Romaji: orStr(get(m, "title", "romaji"), "")},
					AniInfo: aniInfoOf(m)}
				if truthy(get(m, "coverImage")) {
					c.Poster = str(get(m, "coverImage", "large"))
				}
				out = append(out, c)
			}
			return out, nil
		},
	}

	// Anime, second opinion: Kitsu (keyless JSON:API).
	P["kitsu"] = &Provider{
		Label: "Kitsu", Types: []string{"movie", "series"}, Earliest: true,
		Search: func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error) {
			j, err := a.net.getJSON(a.cfg.KitsuBase+"/anime?filter[text]="+encURI(query)+"&page[limit]=15",
				reqInit{Headers: map[string]string{"Accept": "application/vnd.api+json"}, NoRetryOnTimeout: true})
			if err != nil {
				return nil, err
			}
			var out []*Cand
			for _, d := range asArr(get(j, "data")) {
				attrs := get(d, "attributes")
				if !truthy(attrs) {
					attrs = &OMap{M: map[string]any{}}
				}
				if (typ == "movie") != (get(attrs, "subtype") == "movie") {
					continue
				}
				t := get(attrs, "titles")
				titles := nonEmptyTitles(get(attrs, "canonicalTitle"), get(t, "en"), get(t, "en_jp"), get(t, "en_us"), get(t, "ja_jp"))
				titles = append(titles, nonEmptyTitles(asArr(get(attrs, "abbreviatedTitles"))...)...)
				score := num(get(attrs, "averageRating"))
				c := &Cand{Titles: titles, Year: yearOf(get(attrs, "startDate")), Ref: str(get(d, "id")), ID: str(get(d, "id")),
					URLLabel: "Kitsu", Runtime: optPositive(get(attrs, "episodeLength")), Eps: optPositive(get(attrs, "episodeCount")),
					Alt: &core.ArtAlt{English: orStr(firstTruthy(get(t, "en"), get(t, "en_us")), ""), Romaji: orStr(firstTruthy(get(t, "en_jp"), get(attrs, "canonicalTitle")), "")}}
				if truthy(get(attrs, "posterImage")) {
					c.Poster = str(firstTruthy(get(attrs, "posterImage", "large"), get(attrs, "posterImage", "medium"), get(attrs, "posterImage", "original")))
				}
				if truthy(get(attrs, "slug")) {
					c.URL = "https://kitsu.io/anime/" + str(get(attrs, "slug"))
				}
				if score > 0 {
					c.AniInfo = &core.ArtInfo{Genres: []string{}, Rating: jsRound(score) / 10}
				}
				out = append(out, c)
			}
			return out, nil
		},
	}

	// Anime, third opinion: Jikan (MyAnimeList's unofficial API). Rate-limited, so it goes last.
	P["jikan"] = &Provider{
		Label: "MyAnimeList", Types: []string{"movie", "series"}, Earliest: true,
		Search: func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error) {
			j, err := a.net.getJSON(a.cfg.JikanBase+"/anime?q="+encURI(query)+"&limit=15", reqInit{NoRetryOnTimeout: true})
			if err != nil {
				return nil, err
			}
			var out []*Cand
			for _, m := range asArr(get(j, "data")) {
				if (typ == "movie") != (get(m, "type") == "Movie") {
					continue
				}
				dur := str(orV(get(m, "duration"), ""))
				mins := 0
				if h := reHr.Exec(dur); h != nil {
					n, _ := strconv.Atoi(h.Str(1))
					mins += n * 60
				}
				if mm := reMin.Exec(dur); mm != nil {
					n, _ := strconv.Atoi(mm.Str(1))
					mins += n
				}
				titles := nonEmptyTitles(get(m, "title"), get(m, "title_english"), get(m, "title_japanese"))
				for _, x := range asArr(get(m, "titles")) {
					titles = append(titles, nonEmptyTitles(get(x, "title"))...)
				}
				titles = append(titles, nonEmptyTitles(asArr(get(m, "title_synonyms"))...)...)
				year := yearNum(get(m, "year"))
				if year == 0 {
					year = yearNum(get(m, "aired", "prop", "from", "year"))
				}
				score := num(get(m, "score"))
				info := &core.ArtInfo{Genres: []string{}}
				for _, g := range asArr(get(m, "genres")) {
					if n := get(g, "name"); truthy(n) {
						info.Genres = append(info.Genres, str(n))
					}
				}
				if len(info.Genres) > 3 {
					info.Genres = info.Genres[:3]
				}
				if score > 0 {
					info.Rating = jsRound(score*10) / 10
				}
				c := &Cand{Titles: titles, Year: year, URL: str(get(m, "url")), URLLabel: "MyAnimeList", Runtime: mins,
					Alt: &core.ArtAlt{English: orStr(get(m, "title_english"), ""), Romaji: orStr(get(m, "title"), "")}, AniInfo: info}
				if truthy(get(m, "images", "jpg")) {
					c.Poster = str(firstTruthy(get(m, "images", "jpg", "large_image_url"), get(m, "images", "jpg", "image_url")))
				}
				out = append(out, c)
			}
			return out, nil
		},
	}

	// Last resort: the article's lead image. Exact page titles only. Hebrew titles use he.wikipedia.
	P["wikipedia"] = &Provider{
		Label: "Wikipedia", Types: []string{"movie", "series"}, ExactOnly: true,
		Search: func(a *Artwork, typ, query string, media *core.Media) ([]*Cand, error) {
			langs := []string{"en"}
			if reHebrew.Test(query) {
				langs = []string{"he", "en"}
			}
			var out []*Cand
			for _, lang := range langs {
				u := strings.ReplaceAll(a.cfg.WikipediaBase, "{lang}", lang)
				if i := strings.Index(a.cfg.WikipediaBase, "{lang}"); i >= 0 {
					u = a.cfg.WikipediaBase[:i] + lang + a.cfg.WikipediaBase[i+6:] // String.replace: first occurrence only
				}
				u += "/w/api.php?action=query&format=json&generator=search&gsrsearch=" + encURI(query) + "&gsrlimit=6&prop=pageimages&piprop=thumbnail&pithumbsize=500"
				j, err := a.net.getJSON(u, reqInit{NoRetryOnTimeout: true})
				if err != nil {
					return nil, err
				}
				var pages []any
				if o := asObj(get(j, "query", "pages")); o != nil {
					pages = o.Values()
				}
				sort.SliceStable(pages, func(x, y int) bool { return num0(get(pages[x], "index")) < num0(get(pages[y], "index")) })
				for _, p := range pages {
					title := str(get(p, "title"))
					m := reWikiTitle.Exec(title)
					if m != nil && !reWikiKind.Test(m.Str(2)) {
						continue
					}
					first := title
					if m != nil {
						first = m.Str(1)
					}
					c := &Cand{Titles: nonEmptyTitles(get(p, "title"), first),
						URL: "https://" + lang + ".wikipedia.org/wiki/" + encURI(strings.ReplaceAll(title, " ", "_")), URLLabel: "Wikipedia"}
					if truthy(get(p, "thumbnail")) {
						c.Poster = str(get(p, "thumbnail", "source"))
					}
					out = append(out, c)
				}
			}
			return out, nil
		},
	}
	return P
}

func num0(v any) float64 {
	if truthy(v) {
		if f, ok := numOK(v); ok {
			return f
		}
	}
	return 0
}

func orV(v any, d any) any {
	if truthy(v) {
		return v
	}
	return d
}

func firstTruthy(vs ...any) any {
	for _, v := range vs {
		if truthy(v) {
			return v
		}
	}
	if len(vs) > 0 {
		return vs[len(vs)-1]
	}
	return nil
}

func truthy2(n int) bool { return n != core.NA && n != 0 }

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

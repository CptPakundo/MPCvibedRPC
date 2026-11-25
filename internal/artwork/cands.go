package artwork

import "github.com/CptPakundo/MPRP/internal/core"

// The catalogs' answers read as candidates: the same for a search (providers.go) and for a page a fix pins
// (fixes.go).

// tmdbCand reads one TMDB result; kind is "movie" or "tv".
func tmdbCand(r any, kind string) *Cand {
	c := &Cand{
		Titles:   nonEmptyTitles(get(r, "name"), get(r, "title"), get(r, "original_name"), get(r, "original_title")),
		Year:     yearOf(firstTruthy(get(r, "first_air_date"), get(r, "release_date"))),
		URL:      "https://www.themoviedb.org/" + kind + "/" + tmpl(get(r, "id")),
		URLLabel: "TMDB", Ref: str(get(r, "id")), Kind: kind,
	}
	if truthy(get(r, "poster_path")) {
		c.Poster = posterIf(str(get(r, "poster_path")))
	}
	return c
}

// cinemetaCand reads one Cinemeta meta.
func cinemetaCand(m any) *Cand {
	return &Cand{
		Titles: nonEmptyTitles(get(m, "name")), Year: yearOf(get(m, "releaseInfo")), Poster: str(get(m, "poster")),
		URL: imdbURL(str(firstTruthy(get(m, "imdb_id"), get(m, "id")))), URLLabel: "IMDb",
	}
}

// imdbCand reads one IMDb suggestion.
func imdbCand(d any) *Cand {
	c := &Cand{Titles: nonEmptyTitles(get(d, "l")), Year: yearNum(get(d, "y")), URL: imdbURL(str(get(d, "id"))), URLLabel: "IMDb"}
	if truthy(get(d, "i")) {
		c.Poster = str(get(d, "i", "imageUrl"))
	}
	c.l0 = tmpl(get(d, "l"))
	return c
}

// tvmazeCand reads one TVmaze show.
func tvmazeCand(s any) *Cand {
	c := &Cand{Titles: nonEmptyTitles(get(s, "name")), Year: yearOf(get(s, "premiered")), Ref: str(get(s, "id")), ID: str(get(s, "id"))}
	if truthy(get(s, "image")) {
		c.Poster = orStr(get(s, "image", "medium"), str(get(s, "image", "original")))
	}
	if u := imdbURL(str(get(s, "externals", "imdb"))); u != "" {
		c.URL, c.URLLabel = u, "IMDb"
	} else {
		c.URL, c.URLLabel = str(get(s, "url")), "TVmaze"
	}
	return c
}

// anilistFields are the fields asked of each AniList entry.
const anilistFields = "siteUrl format episodes duration genres averageScore startDate { year } title { romaji english native } synonyms coverImage { large } " +
	"staff(perPage: 8, sort: RELEVANCE) { edges { role node { name { full } } } }"

// anilistCand reads one AniList entry.
func anilistCand(m any) *Cand {
	titles := nonEmptyTitles(get(m, "title", "romaji"), get(m, "title", "english"), get(m, "title", "native"))
	titles = append(titles, nonEmptyTitles(asArr(get(m, "synonyms"))...)...)
	c := &Cand{Titles: titles, Year: yearNum(get(m, "startDate", "year")), URL: str(get(m, "siteUrl")), URLLabel: "AniList",
		Runtime: optPositive(get(m, "duration")), Eps: optPositive(get(m, "episodes")),
		Alt:     &core.ArtAlt{English: orStr(get(m, "title", "english"), ""), Romaji: orStr(get(m, "title", "romaji"), "")},
		AniInfo: aniInfoOf(m)}
	if truthy(get(m, "coverImage")) {
		c.Poster = str(get(m, "coverImage", "large"))
	}
	return c
}

// kitsuCand reads one Kitsu entry.
func kitsuCand(d any) *Cand {
	attrs := get(d, "attributes")
	if !truthy(attrs) {
		attrs = &OMap{M: map[string]any{}}
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
	return c
}

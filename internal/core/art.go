package core

import (
	"encoding/json"
	"math"
)

// ---- catalog data handed to the builder (and kept in the artwork cache) -------------

// ArtAlt holds an anime's two AniList spellings.
type ArtAlt struct {
	English string
	Romaji  string
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (a ArtAlt) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"english": nullStr(a.English), "romaji": nullStr(a.Romaji)})
}

func (a *ArtAlt) UnmarshalJSON(b []byte) error {
	var j struct {
		English string `json:"english"`
		Romaji  string `json:"romaji"`
	}
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	a.English, a.Romaji = j.English, j.Romaji
	return nil
}

// ArtInfo is the extra facts shown on the state line.
type ArtInfo struct {
	Genres   []string
	Rating   float64 // 0 = none
	Director string
}

func (a ArtInfo) MarshalJSON() ([]byte, error) {
	g := a.Genres
	if g == nil {
		g = []string{}
	}
	var r any
	if a.Rating != 0 {
		r = a.Rating
	}
	return json.Marshal(map[string]any{"genres": g, "rating": r, "director": nullStr(a.Director)})
}

func (a *ArtInfo) UnmarshalJSON(b []byte) error {
	var j struct {
		Genres   []string `json:"genres"`
		Rating   float64  `json:"rating"`
		Director string   `json:"director"`
	}
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	a.Genres, a.Rating, a.Director = j.Genres, j.Rating, j.Director
	return nil
}

// ArtEpisode is a looked-up episode.
type ArtEpisode struct {
	Title    string
	Season   int // NA
	Number   int // NA
	Absolute bool
	Source   string
	Via      string
}

func (e ArtEpisode) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"season": naNum(e.Season), "number": naNum(e.Number), "title": e.Title,
		"absolute": e.Absolute, "source": e.Source, "via": e.Via})
}

func (e *ArtEpisode) UnmarshalJSON(b []byte) error {
	var j struct {
		Title    string   `json:"title"`
		Season   *float64 `json:"season"`
		Number   *float64 `json:"number"`
		Absolute bool     `json:"absolute"`
		Source   string   `json:"source"`
		Via      string   `json:"via"`
	}
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*e = ArtEpisode{Title: j.Title, Season: optInt(j.Season), Number: optInt(j.Number), Absolute: j.Absolute, Source: j.Source, Via: j.Via}
	return nil
}

// ArtIDs lets the episode lookup skip re-identifying the show.
type ArtIDs struct {
	Imdb, Tvmaze, Tmdb, Kitsu string
}

// Art is what the artwork lookup knows about the playing title.
type Art struct {
	Type       string // "movie" | "series"
	Name       string
	Source     string
	Split      bool
	SeasonName string
	SeasonNo   int // NA
	Poster     string
	URL        string
	URLLabel   string
	Year       int // NA
	Alt        *ArtAlt
	AltChecked bool // alt was looked up and there is none
	Info       *ArtInfo
	AniInfo    *ArtInfo
	Episode    *ArtEpisode
	EpInEntry  int // NA
	IDs        *ArtIDs
	ViaRank    bool
	Shift      int
	EntryEps   int
}

// NewArt returns an Art with the "unknown" numbers preset.
func NewArt() *Art { return &Art{Year: NA, SeasonNo: NA, EpInEntry: NA} }

func optInt(p *float64) int {
	if p == nil || math.IsNaN(*p) {
		return NA
	}
	return int(*p)
}

func naNum(n int) any {
	if n == NA {
		return nil
	}
	return n
}

type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case string:
		*f = flexID(x)
	case float64:
		*f = flexID(jsNumStr(x))
	default:
		*f = ""
	}
	return nil
}

func jsNumStr(x float64) string {
	b, _ := json.Marshal(x)
	return string(b)
}

type artJSON struct {
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	Source     string          `json:"source"`
	Split      bool            `json:"split"`
	SeasonName string          `json:"seasonName"`
	SeasonNo   *float64        `json:"seasonNo"`
	Poster     string          `json:"poster"`
	URL        string          `json:"url"`
	URLLabel   string          `json:"urlLabel"`
	Year       *float64        `json:"year"`
	Alt        json.RawMessage `json:"alt"`
	Info       *ArtInfo        `json:"info"`
	AniInfo    *ArtInfo        `json:"aniInfo"`
	EpInEntry  *float64        `json:"epInEntry"`
	ViaRank    bool            `json:"viaRank"`
	Shift      float64         `json:"shift"`
	EntryEps   float64         `json:"entryEps"`
	IDs        *struct {
		Imdb   flexID `json:"imdb"`
		Tvmaze flexID `json:"tvmaze"`
		Tmdb   flexID `json:"tmdb"`
		Kitsu  flexID `json:"kitsu"`
	} `json:"ids"`
	Episode *struct {
		Title    string   `json:"title"`
		Season   *float64 `json:"season"`
		Number   *float64 `json:"number"`
		Absolute bool     `json:"absolute"`
		Source   string   `json:"source"`
		Via      string   `json:"via"`
	} `json:"episode"`
}

// UnmarshalJSON reads the JSON shape of the recorded reference data (null/absent numbers become NA).
func (a *Art) UnmarshalJSON(b []byte) error {
	var j artJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*a = Art{Type: j.Type, Name: j.Name, Source: j.Source, Split: j.Split, SeasonName: j.SeasonName, SeasonNo: optInt(j.SeasonNo),
		Poster: j.Poster, URL: j.URL, URLLabel: j.URLLabel, Year: optInt(j.Year), Info: j.Info, AniInfo: j.AniInfo,
		EpInEntry: optInt(j.EpInEntry), ViaRank: j.ViaRank, Shift: int(j.Shift), EntryEps: int(j.EntryEps)}
	if len(j.Alt) > 0 {
		if string(j.Alt) == "null" {
			a.AltChecked = true
		} else {
			var al ArtAlt
			if err := json.Unmarshal(j.Alt, &al); err == nil {
				a.Alt = &al
			}
		}
	}
	if j.IDs != nil {
		a.IDs = &ArtIDs{Imdb: string(j.IDs.Imdb), Tvmaze: string(j.IDs.Tvmaze), Tmdb: string(j.IDs.Tmdb), Kitsu: string(j.IDs.Kitsu)}
	}
	if j.Episode != nil {
		a.Episode = &ArtEpisode{Title: j.Episode.Title, Season: optInt(j.Episode.Season), Number: optInt(j.Episode.Number),
			Absolute: j.Episode.Absolute, Source: j.Episode.Source, Via: j.Episode.Via}
	}
	return nil
}

func (a Art) MarshalJSON() ([]byte, error) {
	m := map[string]any{"poster": nullStr(a.Poster), "url": nullStr(a.URL), "urlLabel": a.URLLabel, "name": a.Name, "source": a.Source}
	if a.Type != "" {
		m["type"] = a.Type
	}
	if a.Year != NA && a.Year != 0 {
		m["year"] = a.Year
	}
	if a.Split {
		m["split"] = true
		m["shift"] = a.Shift
	}
	if a.SeasonName != "" {
		m["seasonName"] = a.SeasonName
	}
	if a.SeasonNo != NA {
		m["seasonNo"] = a.SeasonNo
	}
	if a.EntryEps != 0 {
		m["entryEps"] = a.EntryEps
	}
	if a.ViaRank {
		m["viaRank"] = true
	}
	if a.Alt != nil {
		m["alt"] = a.Alt
	} else if a.AltChecked {
		m["alt"] = nil
	}
	if a.Info != nil {
		m["info"] = a.Info
	}
	if a.AniInfo != nil {
		m["aniInfo"] = a.AniInfo
	}
	if a.EpInEntry != NA {
		m["epInEntry"] = a.EpInEntry
	}
	if a.IDs != nil {
		ids := map[string]any{}
		if a.IDs.Imdb != "" {
			ids["imdb"] = a.IDs.Imdb
		}
		if a.IDs.Tvmaze != "" {
			ids["tvmaze"] = a.IDs.Tvmaze
		}
		if a.IDs.Tmdb != "" {
			ids["tmdb"] = a.IDs.Tmdb
		}
		if a.IDs.Kitsu != "" {
			ids["kitsu"] = a.IDs.Kitsu
		}
		m["ids"] = ids
	}
	if a.Episode != nil {
		m["episode"] = a.Episode
	}
	return json.Marshal(m)
}

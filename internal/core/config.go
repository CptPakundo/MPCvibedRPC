// Package core holds the pure logic: settings, filename parsing and the Discord activity builder.
package core

import (
	"encoding/json"
	"reflect"
)

// Config is every setting, with the defaults the program ships with. A user's config.json only needs the keys it
// wants to change; anything missing keeps its default (see LoadConfig).
type Config struct {
	// Discord / MPC-HC
	ClientID     string `json:"clientId"`
	Port         int    `json:"port"`
	PollInterval int    `json:"pollInterval"`

	// Presentation
	ActivityType      string `json:"activityType"` // "watching" (progress bar) or "playing"
	TitleAsName       bool   `json:"titleAsName"`
	TextProgressBar   bool   `json:"textProgressBar"`
	ProgressWidth     int    `json:"progressWidth"`
	ShowRemainingTime bool   `json:"showRemainingTime"`

	// HideTitle shows only "Watching a video" (no title, cover or details) and looks nothing up online.
	HideTitle bool `json:"hideTitle"`
	// HideFiles lists files that are never shown (see MatchesHideList); they are not looked up either.
	HideFiles []string `json:"hideFiles"`

	// PauseClearMinutes clears the status after the video has been paused this long (0 = never). It comes back on resume.
	PauseClearMinutes int `json:"pauseClearMinutes"`

	// Filename cleanup
	SmartFormat       bool `json:"smartFormat"`
	IgnoreBrackets    bool `json:"ignoreBrackets"`
	IgnoreFiletype    bool `json:"ignoreFiletype"`
	ReplaceUnderscore bool `json:"replaceUnderscore"`
	ReplaceDots       bool `json:"replaceDots"`

	// Artwork
	ShowArtwork    bool     `json:"showArtwork"`
	ArtworkSources []string `json:"artworkSources"`
	// DisabledSources lists services (see Sources) that must never be contacted, whatever the lists below say.
	DisabledSources  []string          `json:"disabledSources"`
	TmdbAPIKey       string            `json:"tmdbApiKey"`
	ArtworkAliases   map[string]string `json:"artworkAliases"`
	ArtworkOverrides map[string]string `json:"artworkOverrides"`
	ArtworkWaitMs    int               `json:"artworkWaitMs"`
	CinemetaBase     string            `json:"cinemetaBase"`
	ImdbSuggestBase  string            `json:"imdbSuggestBase"`
	TvmazeBase       string            `json:"tvmazeBase"`
	AnilistURL       string            `json:"anilistUrl"`
	KitsuBase        string            `json:"kitsuBase"`
	BulbapediaBase   string            `json:"bulbapediaBase"`
	JikanBase        string            `json:"jikanBase"`
	AnimeSources     []string          `json:"animeSources"`
	RequestGapMs     map[string]int    `json:"requestGapMs"`
	WikipediaBase    string            `json:"wikipediaBase"`

	EpisodeInDetails     bool     `json:"episodeInDetails"`
	FolderEpisodeNumbers bool     `json:"folderEpisodeNumbers"`
	AnimeTitles          string   `json:"animeTitles"` // auto | english | romaji | file
	CatalogTitle         bool     `json:"catalogTitle"`
	RichInfo             bool     `json:"richInfo"`
	LinkButton           bool     `json:"linkButton"`
	UseFolderName        bool     `json:"useFolderName"`
	EpisodeTitles        bool     `json:"episodeTitles"`
	EpisodeSources       []string `json:"episodeSources"`

	// Discord art assets
	LargeImageKey string `json:"largeImageKey"`
	SmallImageKey string `json:"smallImageKey"`
	AppName       string `json:"appName"`
}

// DefaultConfig returns a fresh copy of the defaults.
func DefaultConfig() Config {
	return Config{
		ClientID:     "427863248734388224",
		Port:         13579,
		PollInterval: 5000,

		ActivityType:      "watching",
		TitleAsName:       true,
		TextProgressBar:   true,
		ProgressWidth:     12,
		ShowRemainingTime: false,

		SmartFormat:       true,
		IgnoreBrackets:    true,
		IgnoreFiletype:    true,
		ReplaceUnderscore: true,
		ReplaceDots:       true,

		ShowArtwork:      true,
		ArtworkSources:   []string{"tmdb", "imdb", "cinemeta", "tvmaze", "anilist", "kitsu", "jikan", "wikipedia"},
		DisabledSources:  []string{},
		HideFiles:        []string{},
		ArtworkAliases:   map[string]string{},
		ArtworkOverrides: map[string]string{},
		ArtworkWaitMs:    1500,
		CinemetaBase:     "https://v3-cinemeta.strem.io",
		ImdbSuggestBase:  "https://v3.sg.media-imdb.com",
		TvmazeBase:       "https://api.tvmaze.com",
		AnilistURL:       "https://graphql.anilist.co",
		KitsuBase:        "https://kitsu.io/api/edge",
		BulbapediaBase:   "https://bulbapedia.bulbagarden.net",
		JikanBase:        "https://api.jikan.moe/v4",
		AnimeSources:     []string{"anilist", "kitsu", "jikan"},
		RequestGapMs:     map[string]int{"graphql.anilist.co": 250, "api.jikan.moe": 400},
		WikipediaBase:    "https://{lang}.wikipedia.org",

		EpisodeInDetails:     false,
		FolderEpisodeNumbers: false,
		AnimeTitles:          "auto",
		CatalogTitle:         true,
		RichInfo:             true,
		LinkButton:           true,
		UseFolderName:        true,
		EpisodeTitles:        true,
		EpisodeSources:       []string{"bulbapedia", "tmdb", "tvmaze", "cinemeta", "kitsu"},

		LargeImageKey: "mpc-hc",
		SmallImageKey: "mpc-hc",
		AppName:       "Media Player Classic",
	}
}

// LoadConfig applies the keys present in a user's JSON on top of the defaults. It is a shallow merge:
// a key that is present replaces the default value entirely (a map or list is not merged element by element).
// Unknown keys and values of the wrong type are ignored.
func LoadConfig(userJSON []byte) Config {
	cfg := DefaultConfig()
	if len(userJSON) == 0 {
		return cfg
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(userJSON, &raw) != nil {
		return cfg
	}
	v := reflect.ValueOf(&cfg).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		key := t.Field(i).Tag.Get("json")
		msg, ok := raw[key]
		if !ok {
			continue
		}
		fresh := reflect.New(t.Field(i).Type)
		if json.Unmarshal(msg, fresh.Interface()) == nil {
			v.Field(i).Set(fresh.Elem())
		}
	}
	return cfg
}

// ConfigFrom is LoadConfig for a Go map (handy in tests and for the settings window).
func ConfigFrom(m map[string]any) Config {
	b, _ := json.Marshal(m)
	return LoadConfig(b)
}

func jsonMarshal(v any) ([]byte, error)   { return json.Marshal(v) }
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

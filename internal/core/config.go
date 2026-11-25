// Package core holds the pure logic: settings, filename parsing and the Discord activity builder.
package core

import (
	"encoding/json"
	"reflect"
)

// Config is every setting, with the defaults the program ships with. A user's config.json only needs the keys it
// wants to change; anything missing keeps its default (see LoadConfig).
//
// The settings come in four groups, embedded so that each is still cfg.<Name> and config.json stays one flat object:
//   - Settings: what the user chooses, in the window or by hand in config.json;
//   - Accounts: what a sign-in writes about the account and the server it follows (private, not secret);
//   - Secrets: the sign-ins' tokens and keys (written only by the sign-ins);
//   - Hooks: where the catalog services are (tests point them at fakes; a server of the user's own is the only other use).
//
// Every field's `reach` tag says what the running engine redoes when the setting changes (engine.ApplySettings):
//
//	check    nothing: the next check reads it (the status is sent again with it)
//	lookups  the cover and title finder is made again (every setting internal/artwork reads)
//	players  the players are looked for again: their event watches and the Plex, Stremio, Jellyfin and Emby watchers
//	discord  the connection to Discord is made again
//	none     the engine does not use it (names for the window, the sign-in's own values)
type Config struct {
	Settings
	Accounts
	Secrets
	Hooks
}

// Settings are what the user chooses.
type Settings struct {
	// Discord / the player
	ClientID     string `json:"clientId" reach:"discord"`
	Port         int    `json:"port" reach:"players"` // web interface of MPC-HC, MPC-BE and MPC-QT
	PollInterval int    `json:"pollInterval" reach:"check"`
	MpvPipe      string `json:"mpvPipe" reach:"players"`      // mpv's input-ipc-server name; empty = don't look for mpv
	VlcPort      int    `json:"vlcPort" reach:"players"`      // VLC's web interface (http-port)
	VlcPassword  string `json:"vlcPassword" reach:"players"`  // its password (http-password); empty = don't look for VLC
	KodiAddress  string `json:"kodiAddress" reach:"players"`  // Kodi's web server (host:port)
	KodiUser     string `json:"kodiUser" reach:"players"`     // its user name
	KodiPassword string `json:"kodiPassword" reach:"players"` // its password; empty = don't look for Kodi
	IinaPipe     string `json:"iinaPipe" reach:"players"`     // macOS: the input-ipc-server path in IINA's mpv options; empty = don't look for IINA
	Mpris        bool   `json:"mpris" reach:"players"`        // Linux: read video players through MPRIS (D-Bus)

	PlexAddress string `json:"plexAddress" reach:"players"` // a fixed address for the Plex server ("" = the addresses plex.tv lists)
	PlexJWT     bool   `json:"plexJwt" reach:"none"`        // sign in with Plex's newer 7-day tokens and this install's own key (off: the long-lived token)

	// Presentation
	ActivityType string `json:"activityType" reach:"check"` // "watching" (progress bar) or "playing"
	TitleAsName  bool   `json:"titleAsName" reach:"check"`
	// StreamTitle says when the status is sent as "Playing <title>", the only kind Discord names a stream after: "off",
	// "auto" (while Discord shares the screen; the engine finds that out and builds with "always" or "off") or "always".
	// The engine's screen-share watch runs while it is "auto".
	StreamTitle       string `json:"streamTitle" reach:"check"`
	TextProgressBar   bool   `json:"textProgressBar" reach:"check"`
	ProgressWidth     int    `json:"progressWidth" reach:"check"`
	ShowRemainingTime bool   `json:"showRemainingTime" reach:"check"`
	// TextLines is the layout of the two text lines (and a film's cover text): "standard" (the built-in lines),
	// "short", "detailed" or "custom" (CustomLines). See lines.go.
	TextLines   string            `json:"textLines" reach:"check"`
	CustomLines map[string]string `json:"customLines" reach:"check"` // episodeDetails, episodeState, movieDetails, movieState, movieCover

	// HideTitle shows only "Watching a video" (no title, cover or details) and looks nothing up online. A change ends
	// the lookups under way (they go quietly: no title in the log).
	HideTitle bool `json:"hideTitle" reach:"lookups"`
	// HideFiles lists files that are never shown (see MatchesHideList); they are not looked up either.
	HideFiles []string `json:"hideFiles" reach:"lookups"`

	// PauseClearMinutes clears the status after the video has been paused this long (0 = never; the default is 30). It comes back on resume.
	PauseClearMinutes int `json:"pauseClearMinutes" reach:"check"`

	// Filename cleanup
	SmartFormat       bool `json:"smartFormat" reach:"check"`
	IgnoreBrackets    bool `json:"ignoreBrackets" reach:"check"`
	IgnoreFiletype    bool `json:"ignoreFiletype" reach:"check"` // only CleanName uses it (the recorded parse tests); titles never carry the extension
	ReplaceUnderscore bool `json:"replaceUnderscore" reach:"check"`
	ReplaceDots       bool `json:"replaceDots" reach:"check"`

	// Artwork
	ShowArtwork    bool     `json:"showArtwork" reach:"lookups"`
	ArtworkSources []string `json:"artworkSources" reach:"lookups"`
	// DisabledSources lists services (see Sources) that must never be contacted, whatever the lists below say.
	DisabledSources []string `json:"disabledSources" reach:"lookups"`
	TmdbAPIKey      string   `json:"tmdbApiKey" reach:"lookups"`
	// MatchFixes is Fix a match: what the user said a title (or one file) is. nil when config.json has no
	// matchFixes key: the legacy maps below are then read as fixes (see Fixes), and the store writes them as
	// matchFixes once (store.MigrateMatchFixes).
	MatchFixes []MatchFix `json:"matchFixes" reach:"lookups"`
	// ArtworkAliases and ArtworkOverrides are the legacy form of Fix a match ("search under another name", "pin a
	// cover image"), read once into MatchFixes and kept in the file for a downgrade.
	ArtworkAliases   map[string]string `json:"artworkAliases" reach:"lookups"`
	ArtworkOverrides map[string]string `json:"artworkOverrides" reach:"lookups"`
	ArtworkWaitMs    int               `json:"artworkWaitMs" reach:"lookups"`
	AnimeSources     []string          `json:"animeSources" reach:"lookups"`
	RequestGapMs     map[string]int    `json:"requestGapMs" reach:"lookups"`

	EpisodeInDetails     bool     `json:"episodeInDetails" reach:"check"`
	FolderEpisodeNumbers bool     `json:"folderEpisodeNumbers" reach:"check"`
	AnimeTitles          string   `json:"animeTitles" reach:"lookups"` // auto | english | romaji | file
	CatalogTitle         bool     `json:"catalogTitle" reach:"lookups"`
	RichInfo             bool     `json:"richInfo" reach:"lookups"`
	LinkButton           bool     `json:"linkButton" reach:"lookups"`
	LetterboxdLink       bool     `json:"letterboxdLink" reach:"lookups"` // the link opens the title's Letterboxd page when it has one
	UseFolderName        bool     `json:"useFolderName" reach:"check"`
	EpisodeTitles        bool     `json:"episodeTitles" reach:"lookups"`
	EpisodeSources       []string `json:"episodeSources" reach:"lookups"`

	// Discord art assets
	LargeImageKey string `json:"largeImageKey" reach:"check"`
	SmallImageKey string `json:"smallImageKey" reach:"check"`
	AppName       string `json:"appName" reach:"check"`
}

// Accounts are what the sign-ins write about the account and the server followed (not typed in the window).
type Accounts struct {
	// Plex: the sign-in made with "Sign in to Plex"
	PlexAccount    int64  `json:"plexAccount" reach:"players"` // the account's id (on a server the user owns, only their own playback counts)
	PlexUser       string `json:"plexUser" reach:"none"`       // the account's name, for the window
	PlexServer     string `json:"plexServer" reach:"players"`  // the chosen server's machine identifier ("" = the account's only or first own server)
	PlexServerName string `json:"plexServerName" reach:"none"` // its name, for the window
	PlexClient     string `json:"plexClient" reach:"players"`  // this install's Plex client identifier (made at the first sign-in)

	// Jellyfin or Emby: the sign-in made on the Players tab
	JellyfinAddress    string `json:"jellyfinAddress" reach:"players"` // the server's address (ending in /emby for an Emby server that needs it)
	JellyfinUser       string `json:"jellyfinUser" reach:"players"`    // the user's id (only their own playback counts)
	JellyfinUserName   string `json:"jellyfinUserName" reach:"none"`   // the user's name, for the window
	JellyfinServerName string `json:"jellyfinServerName" reach:"none"` // the server's name, for the window
	JellyfinProduct    string `json:"jellyfinProduct" reach:"players"` // Jellyfin or Emby
	JellyfinDevice     string `json:"jellyfinDevice" reach:"players"`  // this install's device id towards the server (made at the first sign-in)
}

// Secrets are the sign-ins' tokens and keys (not typed in the window). An empty one means that service is not followed.
type Secrets struct {
	PlexToken     string `json:"plexToken" reach:"players"`     // the Plex account's token
	PlexKey       string `json:"plexKey" reach:"none"`          // this install's signing key for Plex's 7-day tokens (plexJwt)
	StremioKey    string `json:"stremioKey" reach:"players"`    // the Stremio account's key
	JellyfinToken string `json:"jellyfinToken" reach:"players"` // this install's sign-in on the Jellyfin or Emby server
}

// Hooks are where the catalog services are asked. Tests point them at fakes; a Jikan of the user's own is the only
// other use.
type Hooks struct {
	CinemetaBase    string `json:"cinemetaBase" reach:"lookups"`
	ImdbSuggestBase string `json:"imdbSuggestBase" reach:"lookups"`
	TvmazeBase      string `json:"tvmazeBase" reach:"lookups"`
	AnilistURL      string `json:"anilistUrl" reach:"lookups"`
	KitsuBase       string `json:"kitsuBase" reach:"lookups"`
	BulbapediaBase  string `json:"bulbapediaBase" reach:"lookups"`
	JikanBase       string `json:"jikanBase" reach:"lookups"`
	WikipediaBase   string `json:"wikipediaBase" reach:"lookups"`
	LetterboxdBase  string `json:"letterboxdBase" reach:"lookups"`
}

// DefaultClientID is the project's own Discord application (named MPRP, with the program's logo): the default
// clientId. Every install depends on it.
const DefaultClientID = "1575219115981934734"

// FormerClientID is the Discord application every install used before ("Media Player Classic", registered by the
// upstream project). A saved clientId of this value reads as DefaultClientID (store.settingsObj).
const FormerClientID = "427863248734388224"

// DefaultConfig returns a fresh copy of the defaults.
func DefaultConfig() Config {
	return Config{
		Settings: Settings{
			ClientID:     DefaultClientID,
			Port:         13579,
			PollInterval: 5000,
			MpvPipe:      "mpvsocket",
			VlcPort:      8080,
			KodiAddress:  "127.0.0.1:8080",
			KodiUser:     "kodi",
			IinaPipe:     "iina-mpvsocket",
			Mpris:        true,

			PauseClearMinutes: 30,

			ActivityType:      "watching",
			TitleAsName:       true,
			StreamTitle:       "auto",
			TextProgressBar:   true,
			ProgressWidth:     12,
			ShowRemainingTime: false,
			TextLines:         "standard",
			CustomLines:       map[string]string{},

			SmartFormat:       true,
			IgnoreBrackets:    true,
			IgnoreFiletype:    true,
			ReplaceUnderscore: true,
			ReplaceDots:       true,

			ShowArtwork:      true,
			ArtworkSources:   []string{"tmdb", "imdb", "cinemeta", "tvmaze", "anilist", "kitsu", "wikipedia"},
			DisabledSources:  []string{},
			HideFiles:        []string{},
			ArtworkAliases:   map[string]string{},
			ArtworkOverrides: map[string]string{},
			ArtworkWaitMs:    1500,
			AnimeSources:     []string{"anilist", "kitsu"},
			RequestGapMs:     map[string]int{"graphql.anilist.co": 250, "api.jikan.moe": 400},

			EpisodeInDetails:     false,
			FolderEpisodeNumbers: false,
			AnimeTitles:          "auto",
			CatalogTitle:         true,
			RichInfo:             true,
			LinkButton:           true,
			UseFolderName:        true,
			EpisodeTitles:        true,
			EpisodeSources:       []string{"bulbapedia", "tmdb", "tvmaze", "cinemeta", "kitsu"}, // "jikan" (a self-hosted Jikan in jikanBase) would go first for anime

			LargeImageKey: "mpc-hc",
			SmallImageKey: "mpc-hc",
			AppName:       "Media Player Classic",
		},
		Hooks: Hooks{
			CinemetaBase:    "https://v3-cinemeta.strem.io",
			ImdbSuggestBase: "https://v3.sg.media-imdb.com",
			TvmazeBase:      "https://api.tvmaze.com",
			AnilistURL:      "https://graphql.anilist.co",
			KitsuBase:       "https://kitsu.io/api/edge",
			BulbapediaBase:  "https://bulbapedia.bulbagarden.net",
			JikanBase:       "https://api.jikan.moe/v4", // the public Jikan API closed on 2026-10-01: Jikan is in no source list by default
			WikipediaBase:   "https://{lang}.wikipedia.org",
			LetterboxdBase:  "https://letterboxd.com",
		},
	}
}

// ConfigFields lists the fields of Config that hold a setting (those of its groups, not the groups themselves), in
// order. Use reflect.Value.FieldByIndex with their Index.
func ConfigFields() []reflect.StructField {
	var out []reflect.StructField
	for _, f := range reflect.VisibleFields(reflect.TypeOf(Config{})) {
		if !f.Anonymous {
			out = append(out, f)
		}
	}
	return out
}

// LoadConfig applies the keys present in a user's JSON on top of the defaults. It is a shallow merge:
// a key that is present replaces the default value entirely (a map or list is not merged element by element).
// Unknown keys, null and values of the wrong type are ignored.
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
	for _, f := range ConfigFields() {
		msg, ok := raw[f.Tag.Get("json")]
		if !ok || string(msg) == "null" {
			continue
		}
		fresh := reflect.New(f.Type)
		if json.Unmarshal(msg, fresh.Interface()) == nil {
			v.FieldByIndex(f.Index).Set(fresh.Elem())
		}
	}
	return cfg
}

// ConfigFrom is LoadConfig for a Go map (the recorded tests build their settings this way).
func ConfigFrom(m map[string]any) Config {
	b, _ := json.Marshal(m)
	return LoadConfig(b)
}

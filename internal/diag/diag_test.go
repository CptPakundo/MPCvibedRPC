package diag

import (
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
)

func TestRedact(t *testing.T) {
	for in, want := range map[string]string{
		// titles and file names stay: a wrong title can only be traced back to the name it came from
		`[2026-01-01T00:00:00.000Z] INFO Playing: Sample Show · S01E02 [artwork] (MPC-HC: "Sample.Show.S01E02.1080p.mkv")`: `[2026-01-01T00:00:00.000Z] INFO Playing: Sample Show · S01E02 [artwork] (MPC-HC: "Sample.Show.S01E02.1080p.mkv")`,
		`[2026-01-01T00:00:00.000Z] INFO Artwork for "Sample Show" S01E02: Sample Name (1999) via tvmaze.`:                 `[2026-01-01T00:00:00.000Z] INFO Artwork for "Sample Show" S01E02: Sample Name (1999) via tvmaze.`,
		// web addresses keep only their server, folder paths go
		`[2026-01-01T00:00:00.000Z] WARN Update check failed: Get "https://api.example.com/repos/x/y?q=Secret": timeout`: `[2026-01-01T00:00:00.000Z] WARN Update check failed: Get "https://api.example.com/…": timeout`,
		`[2026-01-01T00:00:00.000Z] ERROR Update failed: open C:\Users\Someone\AppData\x.exe: denied`:                    `[2026-01-01T00:00:00.000Z] ERROR Update failed: open <path>: denied`,
		`[2026-01-01T00:00:00.000Z] ERROR fetch https://host.example/a/b?q=Secret failed`:                                `[2026-01-01T00:00:00.000Z] ERROR fetch https://host.example/… failed`,
		`[2026-01-01T00:00:00.000Z] WARN fetch http://someone:secret@host.example:8096/x failed`:                         `[2026-01-01T00:00:00.000Z] WARN fetch http://host.example:8096/… failed`,
		`[2026-01-01T00:00:00.000Z] WARN Could not read /Volumes/Backup Disk/Videos: denied`:                             `[2026-01-01T00:00:00.000Z] WARN Could not read <path>: denied`,
		`[2026-01-01T00:00:00.000Z] INFO Connected to Discord.`:                                                          `[2026-01-01T00:00:00.000Z] INFO Connected to Discord.`,
		`[2026-01-01T00:00:00.000Z] INFO Settings available at http://127.0.0.1:47654/`:                                  `[2026-01-01T00:00:00.000Z] INFO Settings available at http://127.0.0.1:47654/…`,
		// a path in quotes ends at the quote; the title next to it stays
		`[2026-01-01T00:00:00.000Z] WARN Could not read "C:\Users\Someone\mpv.conf" for "Sample Show"`:         `[2026-01-01T00:00:00.000Z] WARN Could not read "<path>" for "Sample Show"`,
		`[2026-01-01T00:00:00.000Z] WARN Could not read '/home/someone/.config/mpv/mpv.conf' (denied)`:         `[2026-01-01T00:00:00.000Z] WARN Could not read '<path>' (denied)`,
		`[2026-01-01T00:00:00.000Z] INFO Following the Plex server "Someone's PC".`:                            `[2026-01-01T00:00:00.000Z] INFO Following the Plex server "…".`,
		`[2026-01-01T00:00:00.000Z] INFO Looking for mpv under the name it is set up with: /tmp/x/mpv-socket.`: `[2026-01-01T00:00:00.000Z] INFO Looking for mpv under the name it is set up with: <path>`,
	} {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestReportHidesCustomPaths(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.MpvPipe = `C:\Users\Someone\pipes\mpv`
	cfg.IinaPipe = "~/sockets/iina"
	cfg.TvmazeBase = "file:///home/someone/fake"
	cfg.JikanBase = "http://127.0.0.1:9000/secret/path?q=Secret"
	got := Report(Input{Version: "9.9.9", System: "Test OS 1.0", Config: cfg})
	for _, leak := range []string{"Someone", "someone", "sockets", "Secret", "secret", `C:\`} {
		if strings.Contains(got, leak) {
			t.Errorf("report leaks %q:\n%s", leak, got)
		}
	}
	for _, want := range []string{"mpv connection: (custom path)", `iinaPipe = "(custom path)"`, `tvmazeBase = "(custom path)"`, `jikanBase = "http://127.0.0.1:9000/…"`} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	cfg = core.DefaultConfig()
	cfg.MpvPipe = "my-mpv-pipe"
	if got := Report(Input{Config: cfg}); !strings.Contains(got, "mpv connection: my-mpv-pipe") {
		t.Errorf("a bare name is kept:\n%s", got)
	}
}

func TestReportKeepsTitlesLeavesOutPrivateThings(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.TmdbAPIKey = "SECRETKEY123"
	cfg.ClientID = "999999999999"
	cfg.HideFiles = []string{"PrivateFolder"}
	cfg.ArtworkAliases = map[string]string{"sample show": "Sample Show (2020)"}
	cfg.ArtworkOverrides = map[string]string{"sample movie": "https://img.example/secret/poster.jpg"}
	cfg.PauseClearMinutes = 5
	np, le := "Sample Show · S01E02", `failed for "Sample Show" at C:\Users\Someone\x`
	in := Input{
		Version: "9.9.9", System: "Test OS 1.0", Players: []string{"MPC-BE"},
		Config: cfg, App: store.App{UpdateRepo: "someone/secret-fork"},
		Status: engine.Status{Running: true, Discord: "connected", MPC: true, NowPlaying: &np, LastError: &le},
		Log: []string{
			`[2026-01-01T00:00:00.000Z] INFO Playing: Sample Show · S01E02 (MPC-BE: "Sample.Show.S01E02.mkv")`,
			`[2026-01-01T00:00:00.000Z] INFO Artwork for "Sample Show": found via imdb.`,
			`[2026-01-01T00:00:00.000Z] ERROR open C:\Users\Someone\Videos\Sample Show.mkv: denied`,
		},
	}
	got := Report(in)
	for _, leak := range []string{"SECRETKEY", "999999999999", "PrivateFolder", "Someone", "secret", `C:\`} {
		if strings.Contains(got, leak) {
			t.Errorf("report leaks %q:\n%s", leak, got)
		}
	}
	for _, want := range []string{"Version: 9.9.9", "Test OS 1.0", "MPC-BE", "tmdbApiKey = (set)", "clientId = (custom)", "hideFiles = (1 entries)",
		`artworkAliases = {"sample show":"Sample Show (2020)"}`, `artworkOverrides = {"sample movie":"https://img.example/…"}`, "pauseClearMinutes = 5",
		"update source: (custom)", "Discord: connected", "Now playing: Sample Show · S01E02", `(MPC-BE: "Sample.Show.S01E02.mkv")`,
		`Artwork for "Sample Show": found via imdb.`, `Last error: failed for "Sample Show" at <path>`} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
}

func TestReportDefaultsAreQuiet(t *testing.T) {
	got := Report(Input{Version: "1", System: "x", Config: core.DefaultConfig(), App: store.App{UpdateRepo: store.DefaultUpdateRepo}})
	if !strings.Contains(got, "(none)") || strings.Contains(got, "update source") || !strings.Contains(got, "Players running: none found") {
		t.Errorf("unexpected report:\n%s", got)
	}
}

func TestReportLeaksNothingAboutPlex(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.PlexToken, cfg.PlexAccount, cfg.PlexUser, cfg.PlexClient = "SECRETTOKEN", 987654321, "SecretUser", "secretclientid"
	cfg.PlexServer, cfg.PlexServerName, cfg.PlexAddress = "secretmachineid", "Secret Server", "http://192.168.1.20:32400"
	got := Report(Input{Version: "1", System: "x", Config: cfg, App: store.App{UpdateRepo: store.DefaultUpdateRepo}, Log: []string{
		`[2026-01-01T00:00:00.000Z] INFO Following the Plex server "Secret Server".`,
		`[2026-01-01T00:00:00.000Z] WARN Could not reach the Plex server "Secret Server". Is it running, and can this computer reach it?`,
	}})
	for _, leak := range []string{"SECRET", "Secret", "secret", "987654321", "192.168"} {
		if strings.Contains(got, leak) {
			t.Errorf("report leaks %q:\n%s", leak, got)
		}
	}
	for _, want := range []string{"Plex: signed in, fixed server address", "plexToken = (set)", "plexAddress = (set)", "Following the Plex server"} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	if got := Report(Input{Version: "1", System: "x", Config: core.DefaultConfig()}); !strings.Contains(got, "Plex: off (not signed in)") {
		t.Errorf("report without Plex:\n%s", got)
	}
}

func TestReportLeaksNothingAboutStremio(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.StremioKey = "SECRETSTREMIOKEY"
	got := Report(Input{Version: "1", System: "x", Config: cfg})
	if strings.Contains(got, "SECRETSTREMIOKEY") {
		t.Errorf("the key leaks:\n%s", got)
	}
	for _, want := range []string{"Stremio: signed in", "stremioKey = (set)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if got := Report(Input{Version: "1", System: "x", Config: core.DefaultConfig()}); !strings.Contains(got, "Stremio: off (not signed in)") {
		t.Errorf("report without Stremio:\n%s", got)
	}
}

func TestReportLeaksNothingAboutJellyfin(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.JellyfinToken, cfg.JellyfinAddress, cfg.JellyfinUser, cfg.JellyfinUserName = "SECRETTOKEN", "http://media.example:8096", "USERID", "SAMPLEUSER"
	cfg.JellyfinServerName, cfg.JellyfinDevice, cfg.JellyfinProduct = "SAMPLESERVER", "DEVICEID", "Jellyfin"
	got := Report(Input{Version: "1", System: "x", Config: cfg})
	for _, secret := range []string{"SECRETTOKEN", "media.example", "USERID", "SAMPLEUSER", "SAMPLESERVER", "DEVICEID"} {
		if strings.Contains(got, secret) {
			t.Errorf("%s leaks:\n%s", secret, got)
		}
	}
	for _, want := range []string{"Jellyfin / Emby: signed in to a Jellyfin server", "jellyfinToken = (set)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if got := Report(Input{Version: "1", System: "x", Config: core.DefaultConfig()}); !strings.Contains(got, "Jellyfin / Emby: off (not signed in)") {
		t.Errorf("report without Jellyfin:\n%s", got)
	}
}

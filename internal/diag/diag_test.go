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
		`[2026-01-01T00:00:00.000Z] INFO Playing: Secret Show S01E02 [artwork]`:                                          `[2026-01-01T00:00:00.000Z] INFO Playing: (title omitted)`,
		`[2026-01-01T00:00:00.000Z] INFO Paused: Secret Show`:                                                            `[2026-01-01T00:00:00.000Z] INFO Paused: (title omitted)`,
		`[2026-01-01T00:00:00.000Z] INFO Artwork for "Secret Show" S01E02: Secret Name (1999) via tvmaze.`:               `[2026-01-01T00:00:00.000Z] INFO Artwork for "…" (via tvmaze) (details omitted)`,
		`[2026-01-01T00:00:00.000Z] INFO No artwork match for "Secret Show". Seen: "Other" ...`:                          `[2026-01-01T00:00:00.000Z] INFO No artwork match for "…" (details omitted)`,
		`[2026-01-01T00:00:00.000Z] WARN Update check failed: Get "https://api.example.com/repos/x/y?q=Secret": timeout`: `[2026-01-01T00:00:00.000Z] WARN Update check failed: Get "…" (details omitted)`,
		`[2026-01-01T00:00:00.000Z] ERROR Update failed: open C:\Users\Someone\AppData\x.exe: denied`:                    `[2026-01-01T00:00:00.000Z] ERROR Update failed: open <path>: denied`,
		`[2026-01-01T00:00:00.000Z] ERROR fetch https://host.example/a/b?q=Secret failed`:                                `[2026-01-01T00:00:00.000Z] ERROR fetch https://host.example/… failed`,
		`[2026-01-01T00:00:00.000Z] INFO Connected to Discord.`:                                                          `[2026-01-01T00:00:00.000Z] INFO Connected to Discord.`,
		`[2026-01-01T00:00:00.000Z] INFO Settings available at http://127.0.0.1:47654/`:                                  `[2026-01-01T00:00:00.000Z] INFO Settings available at http://127.0.0.1:47654/…`,
	} {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestReportLeaksNothingPrivate(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.TmdbAPIKey = "SECRETKEY123"
	cfg.ClientID = "999999999999"
	cfg.HideFiles = []string{"PrivateFolder"}
	cfg.ArtworkAliases = map[string]string{"secret title": "Other Secret"}
	cfg.ArtworkOverrides = map[string]string{"secret title": "https://img.example/secret.jpg"}
	cfg.PauseClearMinutes = 5
	np, le := "Secret Show S01E02", `failed for "Secret Show" at C:\Users\Someone\x`
	in := Input{
		Version: "9.9.9", System: "Test OS 1.0", Players: []string{"MPC-BE"},
		Config: cfg, App: store.App{UpdateRepo: "someone/secret-fork"},
		Status: engine.Status{Running: true, Discord: "connected", MPC: true, NowPlaying: &np, LastError: &le},
		Log: []string{
			`[2026-01-01T00:00:00.000Z] INFO Playing: Secret Show S01E02`,
			`[2026-01-01T00:00:00.000Z] INFO Artwork for "Secret Show": found via imdb.`,
			`[2026-01-01T00:00:00.000Z] ERROR open C:\Users\Someone\Videos\Secret Show.mkv: denied`,
		},
	}
	got := Report(in)
	for _, leak := range []string{"Secret", "secret", "SECRETKEY", "999999999999", "PrivateFolder", "Someone", "img.example", `C:\`} {
		if strings.Contains(got, leak) {
			t.Errorf("report leaks %q:\n%s", leak, got)
		}
	}
	for _, want := range []string{"Version: 9.9.9", "Test OS 1.0", "MPC-BE", "tmdbApiKey = (set)", "clientId = (custom)", "hideFiles = (1 entries)", "artworkAliases = (1 entries)", "pauseClearMinutes = 5", "update source: (custom)", "Discord: connected", "via imdb"} {
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

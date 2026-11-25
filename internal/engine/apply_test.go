package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPRP/internal/core"
)

func (d *disc) handshakes() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.hellos...)
}

// A change that touches no player or Discord setting leaves the connection to Discord open: the status goes out again
// on the same connection, and is never cleared in between. That holds for every reach but Discord's: the look, the
// lookups, the players, a secret, a test hook and a name for the window.
func TestSettingsChangeKeepsTheDiscordConnection(t *testing.T) {
	t.Parallel()
	e, _, d, l := setup(t)
	e.Start()
	eventually(t, "shown", func() bool { return len(d.showing()) == 1 && e.Status().NowPlaying != nil })
	changes := []struct {
		name string
		f    func(*core.Config)
	}{
		{"the look (check)", func(c *core.Config) { c.ActivityType = "playing" }},
		{"the lookups", func(c *core.Config) { c.ArtworkWaitMs++ }},
		{"a test hook (lookups)", func(c *core.Config) { c.CinemetaBase = "http://127.0.0.1:9" }},
		{"the players", func(c *core.Config) { c.MpvPipe = "" }},
		{"a secret (players)", func(c *core.Config) { c.JellyfinToken = "sample" }}, // no server address: nothing is asked
		{"a name for the window (none)", func(c *core.Config) { c.PlexUser = "Sample User" }},
	}
	for _, ch := range changes {
		n := len(d.list())
		cfg := e.Config()
		ch.f(&cfg)
		e.ApplySettings(cfg)
		if ch.name != "a name for the window (none)" {
			eventually(t, "sent again after "+ch.name, func() bool { return len(d.list()) > n && len(d.showing()) == 1 })
		} else {
			time.Sleep(10 * testPoll)
			if len(d.list()) != n {
				t.Errorf("%s sent %d requests to Discord", ch.name, len(d.list())-n)
			}
		}
		for _, a := range d.list()[n:] {
			if _, has := a["activity"]; !has {
				t.Errorf("%s cleared the status", ch.name)
			}
		}
		if h := d.handshakes(); len(h) != 1 {
			t.Fatalf("%s connected to Discord again: %v", ch.name, h)
		}
	}
	if l.count("Connected to Discord") != 1 || l.has("Stopped sending the status to Discord") {
		t.Errorf("the run was restarted:\n%s", strings.Join(l.list(), "\n"))
	}
	if !l.has("Settings changed. Looking for a player on the web interface") {
		t.Errorf("a change of the players is not in the log:\n%s", strings.Join(l.list(), "\n"))
	}
}

// Another Discord application (clientId) needs a new connection: the old one is let go (its status with it), and what
// plays goes up on the new one.
func TestClientIDChangeReconnectsToDiscord(t *testing.T) {
	t.Parallel()
	e, _, d, l := setup(t)
	e.Start()
	eventually(t, "shown", func() bool { return len(d.showing()) == 1 })
	cfg := e.Config()
	cfg.ClientID = "123456789012345678"
	e.ApplySettings(cfg)
	eventually(t, "connected with the new application", func() bool {
		h := d.handshakes()
		return len(h) == 2 && h[1] == cfg.ClientID && e.Status().Discord == "connected"
	})
	eventually(t, "the status on the new connection only", func() bool { return len(d.showing()) == 1 })
	if h := d.handshakes(); h[0] != core.DefaultConfig().ClientID {
		t.Errorf("the first connection used %q", h[0])
	}
	if l.has("Stopped sending the status to Discord") || l.has("Lost connection") {
		t.Errorf("the reconnect was not a quiet one:\n%s", strings.Join(l.list(), "\n"))
	}
	time.Sleep(10 * testPoll)
	if h := d.handshakes(); len(h) != 2 {
		t.Errorf("connected %d times", len(h))
	}
}

// Every setting says what a change of it reaches (core.Config's reach tag).
func TestEverySettingHasAReach(t *testing.T) {
	for _, f := range core.ConfigFields() {
		if _, ok := reachNames[f.Tag.Get("reach")]; !ok {
			t.Errorf("%s has no known reach tag (one of check, lookups, players, discord, none): %q", f.Name, f.Tag.Get("reach"))
		}
	}
}

// What keeps a copy of the settings, or reads them only when it starts, must be made again when they change: every
// setting internal/artwork reads must reach "lookups" (the finder keeps its own copy), every one the player watches
// and the Plex, Stremio, Jellyfin and Emby watchers read must reach "players", and the Discord application "discord".
// Otherwise a change would not get there until the engine is started again.
func TestSettingsReachWhatReadsThem(t *testing.T) {
	reaches := map[string]string{}
	for _, f := range core.ConfigFields() {
		reaches[f.Name] = f.Tag.Get("reach")
	}
	artwork, _ := filepath.Glob(filepath.Join("..", "artwork", "*.go"))
	want := map[string][]string{
		"lookups": artwork,
		"players": {"remote.go", "events.go", "players.go"},
		"discord": {"discord.go"},
	}
	read := regexp.MustCompile(`\bcfg\.([A-Z]\w*)`)
	for reach, files := range want {
		seen := 0
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			b, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range read.FindAllStringSubmatch(string(b), -1) {
				r, field := reaches[m[1]]
				if !field {
					continue // a method (SourceOn reads disabledSources)
				}
				seen++
				if r != reach {
					t.Errorf("%s reads %s, whose reach is %q: it must be %s", filepath.ToSlash(name), m[1], r, reach)
				}
			}
		}
		if seen == 0 {
			t.Errorf("found no setting read for %s", reach)
		}
	}
}

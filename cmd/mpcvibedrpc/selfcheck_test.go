package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
)

// checkApp is an app whose engine sees no player, no Discord and no catalog on this computer.
func checkApp(t *testing.T, edit func(*core.Config)) *app {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := core.DefaultConfig()
	cfg.Port = 1 // nothing answers there
	cfg.ArtworkSources = []string{"cinemeta", "tvmaze"}
	cfg.EpisodeSources, cfg.AnimeSources = nil, nil
	edit(&cfg)
	opts := engine.Options{
		Pipes:        func(core.Config) []engine.PlayerPipe { return nil },
		MPRIS:        func(time.Duration) []*core.Info { return nil },
		PotPlayer:    func() []*core.Info { return nil },
		DiscordPaths: []string{t.TempDir() + "/no-discord"},
	}
	return &app{st: st, log: func(string, string) {}, eng: engine.New(cfg, opts)}
}

func runCheck(t *testing.T, a *app) []checkItem {
	t.Helper()
	r, err := a.handlers()["selfcheck"].Fn(nil)
	if err != nil {
		t.Fatal(err)
	}
	return r.(map[string]any)["items"].([]checkItem)
}

func find(items []checkItem, group, state string) *checkItem {
	for i := range items {
		if items[i].Group == group && items[i].State == state {
			return &items[i]
		}
	}
	return nil
}

func answering(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.NotFoundHandler()) // any HTTP answer counts, even a 404
	t.Cleanup(s.Close)
	return s
}

func TestSelfCheckCatalogsAndUpdates(t *testing.T) {
	up, down := answering(t), answering(t)
	down.Close()
	defer func(old string) { updater.APIBase = old }(updater.APIBase)
	updater.APIBase = up.URL

	a := checkApp(t, func(c *core.Config) { c.CinemetaBase, c.TvmazeBase = up.URL, down.URL })
	setApp(t, a.st, `{"checkUpdates":true}`)
	items := runCheck(t, a)
	if it := find(items, "Covers", "ok"); it == nil || it.Text != "Answering: Cinemeta." {
		t.Errorf("answering catalog: %+v", items)
	}
	if it := find(items, "Covers", "problem"); it == nil || !strings.HasPrefix(it.Text, "Not answering: TVmaze.") {
		t.Errorf("closed catalog: %+v", items)
	}
	if find(items, "Updates", "ok") == nil {
		t.Errorf("update server answers: %+v", items)
	}
	if find(items, "Discord", "problem") == nil || find(items, "Players", "problem") == nil {
		t.Errorf("no Discord and no player here: %+v", items)
	}

	updater.APIBase = down.URL
	if find(runCheck(t, a), "Updates", "problem") == nil {
		t.Error("update server closed")
	}
	setApp(t, a.st, `{"checkUpdates":false}`)
	if find(runCheck(t, a), "Updates", "info") == nil {
		t.Error("update checks off")
	}
}

func TestSelfCheckCoversOffAsksNothing(t *testing.T) {
	down := answering(t)
	down.Close()
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked++ }))
	defer srv.Close()
	a := checkApp(t, func(c *core.Config) { c.ShowArtwork = false; c.CinemetaBase = srv.URL })
	setApp(t, a.st, `{"checkUpdates":false}`)
	items := runCheck(t, a)
	if it := find(items, "Covers", "info"); it == nil || !strings.Contains(it.Text, "off") {
		t.Errorf("covers off: %+v", items)
	}
	if asked != 0 {
		t.Errorf("a catalog was asked %d times", asked)
	}
}

// A line never carries a title: the wording comes from the check, and the player lines hold names of players only.
func TestPlayerLines(t *testing.T) {
	items := playerLines([]engine.PlayerProbe{
		{Name: "MPC-HC", Answers: true, State: engine.ProbePlaying},
		{Name: "MPC-HC", Answers: true, State: engine.ProbePlaying},
		{Name: "VLC", Problem: "VLC refused the password."},
		{Name: "Kodi"},
		{Name: "Plex", Account: true, Answers: true},
		{Name: "Stremio", Account: true},
	}, "windows")
	want := []checkItem{
		{"Players", "ok", "MPC-HC answers and is playing."},
		{"Players", "problem", "VLC: VLC refused the password."},
		{"Players", "info", "No answer from: Kodi. That is fine for players you do not use."},
		{"Players", "ok", "Signed in: Plex."},
		{"Players", "info", "Not signed in: Stremio. Only needed if you use them (Accounts tab)."},
	}
	if len(items) != len(want) {
		t.Fatalf("%+v", items)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Errorf("line %d: %+v, want %+v", i, items[i], want[i])
		}
	}
	for _, goos := range []string{"windows", "darwin", "linux"} {
		if got := playerLines(nil, goos); len(got) != 1 || got[0].State != "problem" {
			t.Errorf("%s: %+v", goos, got)
		}
	}
	if noPlayerHint("darwin") == noPlayerHint("linux") || noPlayerHint("linux") == noPlayerHint("windows") {
		t.Error("the hint should name the players of each system")
	}
}

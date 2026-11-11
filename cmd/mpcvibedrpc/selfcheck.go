package main

import (
	"context"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
)

// checkItem is one line of "Run a check" (Help tab). Its text never holds a title or a file name.
type checkItem struct {
	Group string `json:"group"`
	State string `json:"state"` // ok | problem | info
	Text  string `json:"text"`
}

// checkWait is how long one address gets to answer.
const checkWait = 5 * time.Second

// selfCheck is the window's "Run a check": what works and what does not, in plain lines. It looks at Discord's
// connection point without connecting (Discord is only contacted once something plays), asks each player once, and
// asks each catalog and the update server with a plain request that carries no title.
func (a *app) selfCheck(*jsonx.Obj) (any, error) {
	cfg, set := a.eng.Config(), a.st.App()
	ctx, cancel := context.WithTimeout(context.Background(), 2*checkWait)
	defer cancel()
	var (
		wg      sync.WaitGroup
		players []engine.PlayerProbe
		covers  []checkItem
		updates []checkItem
	)
	wg.Add(3)
	engine.Go(a.log, "The player check", func() { defer wg.Done(); players = a.eng.Probe(ctx) })
	engine.Go(a.log, "The catalog check", func() { defer wg.Done(); covers = coverLines(ctx, cfg) })
	engine.Go(a.log, "The update check", func() { defer wg.Done(); updates = updateLines(ctx, set.CheckUpdates && set.UpdateRepo != "") })
	items := discordLines(a.eng.DiscordRunning(), a.eng.Status())
	wg.Wait()
	items = append(items, playerLines(players, runtime.GOOS)...)
	items = append(items, covers...)
	items = append(items, updates...)
	return map[string]any{"items": items}, nil
}

// discordLines says whether Discord's app is there and what the engine knows of the connection.
func discordLines(running bool, s engine.Status) []checkItem {
	line := func(state, text string) []checkItem { return []checkItem{{"Discord", state, text}} }
	switch {
	case !running:
		return line("problem", "Discord's desktop app was not found. Open Discord (the app, not the browser).")
	case s.DiscordProblem != "":
		return line("problem", s.DiscordProblem)
	case !s.Running:
		return line("ok", "Discord is running. Show on Discord is off.")
	case s.Discord == "connected":
		return line("ok", "Discord is running and connected.")
	case s.Discord == "waiting":
		return line("problem", "Discord is running, but did not take the connection yet.")
	}
	return line("ok", "Discord is running. It is contacted once something plays.")
}

// noPlayerHint names the players of the running system, and what to do when none answers.
func noPlayerHint(goos string) string {
	names := map[string]string{
		"windows": "MPC-HC, MPC-BE, MPC-QT, VLC, mpv, PotPlayer or Kodi",
		"darwin":  "IINA, mpv, VLC or MPC-QT",
	}[goos]
	if names == "" {
		names = "VLC, Celluloid, Haruna or mpv"
	}
	return "No player answers. Open " + names + " and play something. If it still does not answer, press Set up the player connection (Advanced)."
}

// playerLines turns the probes into lines: each player that answers, each problem that is known, one line for the
// rest, and the accounts.
func playerLines(probes []engine.PlayerProbe, goos string) []checkItem {
	var out []checkItem
	add := func(state, text string) { out = append(out, checkItem{"Players", state, text}) }
	var seen, silent, signedIn, signedOut []string
	for _, p := range probes {
		switch {
		case p.Account && p.Answers:
			signedIn = append(signedIn, p.Name)
		case p.Account:
			signedOut = append(signedOut, p.Name)
		case p.Answers:
			if slices.Contains(seen, p.Name) {
				continue // MPC-QT answers on its web interface and on its own connection
			}
			seen = append(seen, p.Name)
			switch p.State {
			case engine.ProbePlaying:
				add("ok", p.Name+" answers and is playing.")
			case engine.ProbePaused:
				add("ok", p.Name+" answers and is paused.")
			default:
				add("ok", p.Name+" answers. Nothing is playing.")
			}
		case p.Problem != "":
			add("problem", p.Name+": "+p.Problem)
		default:
			silent = append(silent, p.Name)
		}
	}
	if len(seen) == 0 {
		add("problem", noPlayerHint(goos))
	}
	if len(silent) > 0 {
		add("info", "No answer from: "+strings.Join(silent, ", ")+". That is fine for players you do not use.")
	}
	if len(signedIn) > 0 {
		add("ok", "Signed in: "+strings.Join(signedIn, ", ")+".")
	}
	if len(signedOut) > 0 {
		add("info", "Not signed in: "+strings.Join(signedOut, ", ")+". Only needed if you use them (Accounts tab).")
	}
	return out
}

// target is one address the check asks.
type target struct{ label, url string }

// catalogTargets lists the catalogs the program would use with these settings: switched on, in one of the lists,
// with an address (TMDB only with a key, MyAnimeList only through a server of your own).
func catalogTargets(cfg core.Config) []target {
	bases := map[string]string{
		"tmdb":       "https://api.themoviedb.org",
		"imdb":       cfg.ImdbSuggestBase,
		"cinemeta":   cfg.CinemetaBase,
		"tvmaze":     cfg.TvmazeBase,
		"anilist":    cfg.AnilistURL,
		"kitsu":      cfg.KitsuBase,
		"jikan":      cfg.JikanBase,
		"wikipedia":  strings.ReplaceAll(cfg.WikipediaBase, "{lang}", "en"),
		"bulbapedia": cfg.BulbapediaBase,
	}
	used := func(id string) bool {
		return slices.Contains(cfg.ArtworkSources, id) || slices.Contains(cfg.EpisodeSources, id) || slices.Contains(cfg.AnimeSources, id)
	}
	var out []target
	for _, s := range core.Sources {
		if !cfg.SourceOn(s.ID) || !used(s.ID) || bases[s.ID] == "" || (s.ID == "tmdb" && cfg.TmdbAPIKey == "") {
			continue
		}
		out = append(out, target{s.Label, bases[s.ID]})
	}
	return out
}

// reaches asks every target at once with a plain GET (no title, no query, no key); any HTTP answer counts. The
// client is the program's own, so the request is listed under Recent connections.
func reaches(ctx context.Context, targets []target) []bool {
	client := &http.Client{Timeout: checkWait, Transport: http.DefaultTransport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ok := make([]bool, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		engine.Go(func(string, string) {}, "A reachability check", func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
			if err != nil {
				return
			}
			res, err := client.Do(req)
			if err != nil {
				return
			}
			res.Body.Close()
			ok[i] = true
		})
	}
	wg.Wait()
	return ok
}

// coverLines asks the catalogs the settings use.
func coverLines(ctx context.Context, cfg core.Config) []checkItem {
	if !cfg.ShowArtwork {
		return []checkItem{{"Covers", "info", "Cover lookups are off, so no catalog is asked."}}
	}
	targets := catalogTargets(cfg)
	if len(targets) == 0 {
		return []checkItem{{"Covers", "info", "No catalog is switched on."}}
	}
	var up, down []string
	for i, ok := range reaches(ctx, targets) {
		if ok {
			up = append(up, targets[i].label)
		} else {
			down = append(down, targets[i].label)
		}
	}
	var out []checkItem
	if len(up) > 0 {
		out = append(out, checkItem{"Covers", "ok", "Answering: " + strings.Join(up, ", ") + "."})
	}
	if len(down) > 0 {
		out = append(out, checkItem{"Covers", "problem", "Not answering: " + strings.Join(down, ", ") + ". Titles and covers from them may be missing."})
	}
	return out
}

// updateLines asks the update server when update checks are on. /rate_limit is GitHub's free question: it does not
// use up the hourly allowance the real check needs.
func updateLines(ctx context.Context, on bool) []checkItem {
	if !on {
		return []checkItem{{"Updates", "info", "Update checks are off."}}
	}
	if reaches(ctx, []target{{"GitHub", updater.APIBase + "/rate_limit"}})[0] {
		return []checkItem{{"Updates", "ok", "The update server answers."}}
	}
	return []checkItem{{"Updates", "problem", "The update server does not answer."}}
}

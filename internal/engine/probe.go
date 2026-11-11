package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/kodi"
	"github.com/CptPakundo/MPCvibedRPC/internal/potplayer"
	"github.com/CptPakundo/MPCvibedRPC/internal/vlchttp"
)

// Probe states of a player that answers.
const (
	ProbePlaying = "playing"
	ProbePaused  = "paused"
	ProbeIdle    = "idle"
)

// PlayerProbe is what "Run a check" learns about one player or account. It never holds a title or a file name.
type PlayerProbe struct {
	Name    string // what the window calls it ("MPC-HC", "VLC", "Plex")
	Account bool   // Plex, Stremio, Jellyfin or Emby: Answers then means "signed in", and nothing was asked
	Answers bool
	State   string // ProbePlaying, ProbePaused or ProbeIdle while Answers (players only)
	Problem string // why a player that seems to be there does not answer, when that is known
}

// DiscordRunning reports whether Discord's connection point exists. Like the engine's own look, it never connects.
func (e *Engine) DiscordRunning() bool { return e.discordRunning() }

// stateOf names what a player's answer says, without the file.
func stateOf(in *core.Info) string {
	switch in.State {
	case 2, 3:
		return ProbePlaying
	case 1:
		return ProbePaused
	}
	return ProbeIdle
}

// Probe asks every player the engine would ask with the current settings once, and lists which answer and what they
// do. Unlike fetchPlayer it changes nothing in the engine (no problem hint, no log line, no watcher) and works
// whether or not the engine runs. It honours the same test hooks as the engine. The accounts are not asked: they are
// only reported as signed in or not.
func (e *Engine) Probe(ctx context.Context) []PlayerProbe {
	cfg := e.Config()
	pipes := PlayerPipes
	if e.opts.Pipes != nil {
		pipes = e.opts.Pipes
	}
	// each step gives its probes; they run side by side and are put back in the order the engine asks
	var steps []func() []PlayerProbe
	one := func(name string, in *core.Info, problem string) []PlayerProbe {
		if in == nil {
			return []PlayerProbe{{Name: name, Problem: problem}}
		}
		if in.Player != "" {
			name = in.Player
		}
		return []PlayerProbe{{Name: name, Answers: true, State: stateOf(in)}}
	}
	steps = append(steps, func() []PlayerProbe {
		return one(fmt.Sprintf("MPC-HC, MPC-BE or MPC-QT (web interface, port %d)", cfg.Port), e.fetchMPC(ctx, cfg.Port), "")
	})
	for _, p := range pipes(cfg) {
		steps = append(steps, func() []PlayerProbe { return one(p.Name, askPipe(p), "") })
	}
	if cfg.VlcPassword != "" {
		steps = append(steps, func() []PlayerProbe {
			c, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
			defer cancel()
			in, err := (&vlchttp.Reader{Client: e.opts.HTTPClient}).Query(c, cfg.VlcPort, cfg.VlcPassword)
			problem := ""
			switch {
			case errors.Is(err, vlchttp.ErrPassword):
				problem = "VLC refused the password."
			case errors.Is(err, vlchttp.ErrNoPassword):
				problem = "VLC's web interface has no password, so VLC keeps it closed."
			case errors.Is(err, vlchttp.ErrNotVLC):
				problem = fmt.Sprintf("Port %d answers, but not as VLC.", cfg.VlcPort)
			}
			if err != nil {
				in = nil
			} else {
				in.Player = "VLC"
			}
			return one("VLC", in, problem)
		})
	}
	if HasMPRIS && cfg.Mpris {
		steps = append(steps, func() []PlayerProbe {
			query := QueryMPRIS
			if e.opts.MPRIS != nil {
				query = e.opts.MPRIS
			}
			var out []PlayerProbe
			for _, in := range query(1500 * time.Millisecond) {
				out = append(out, one("a player on D-Bus", in, "")...)
			}
			if len(out) == 0 {
				out = one("video players on D-Bus (MPRIS)", nil, "")
			}
			return out
		})
	}
	if runtime.GOOS == "windows" || e.opts.PotPlayer != nil {
		steps = append(steps, func() []PlayerProbe {
			query := potplayer.Query
			if e.opts.PotPlayer != nil {
				query = e.opts.PotPlayer
			}
			var out []PlayerProbe
			for _, in := range query() {
				in.Player = "PotPlayer"
				out = append(out, one("PotPlayer", in, "")...)
			}
			if len(out) == 0 {
				out = one("PotPlayer", nil, "")
			}
			return out
		})
	}
	if cfg.KodiPassword != "" {
		steps = append(steps, func() []PlayerProbe {
			c, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
			defer cancel()
			in, err := (&kodi.Reader{Client: e.opts.HTTPClient}).Query(c, cfg.KodiAddress, cfg.KodiUser, cfg.KodiPassword)
			problem := ""
			switch {
			case errors.Is(err, kodi.ErrPassword):
				problem = "Kodi refused the user name or password."
			case errors.Is(err, kodi.ErrNotKodi):
				problem = "The Kodi address answers, but not as Kodi."
			}
			if err != nil {
				in = nil
			} else {
				in.Player = "Kodi"
			}
			return one("Kodi", in, problem)
		})
	}

	results := make([][]PlayerProbe, len(steps))
	var wg sync.WaitGroup
	for i, step := range steps {
		wg.Add(1)
		Go(e.log, "A player check", func() {
			defer wg.Done()
			results[i] = step()
		})
	}
	wg.Wait()
	var out []PlayerProbe
	for _, r := range results {
		out = append(out, r...)
	}
	// the accounts: read from the settings, no request
	out = append(out, PlayerProbe{Name: "Plex", Account: true, Answers: cfg.PlexToken != ""},
		PlayerProbe{Name: "Stremio", Account: true, Answers: cfg.StremioKey != ""})
	if cfg.JellyfinToken != "" && cfg.JellyfinAddress != "" {
		out = append(out, PlayerProbe{Name: orJellyfin(cfg.JellyfinProduct), Account: true, Answers: true})
	} else {
		out = append(out, PlayerProbe{Name: "Jellyfin or Emby", Account: true})
	}
	return out
}

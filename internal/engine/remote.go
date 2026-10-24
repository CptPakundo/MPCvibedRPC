package engine

import (
	"context"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
	"github.com/CptPakundo/MPCvibedRPC/internal/jellyfin"
	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
	"github.com/CptPakundo/MPCvibedRPC/internal/plex"
	"github.com/CptPakundo/MPCvibedRPC/internal/stremio"
)

// watcher follows one account or server for what is playing on it (Plex, Stremio, Jellyfin or Emby).
type watcher interface {
	Run(ctx context.Context)
	Now() *core.Info
	Problem() string
}

// remote is a watcher and the name its answers carry as the player.
type remote struct {
	name string
	w    watcher
}

// remoteSource offers what the remotes play, in order. The list is read when the source runs, so remotes stopped by
// resting meanwhile are not asked.
func (e *Engine) remoteSource(ctx context.Context) source {
	return func(offer func(*core.Info) bool) {
		e.mu.Lock()
		remotes := e.remotes
		e.mu.Unlock()
		for _, r := range remotes {
			if ctx.Err() != nil {
				return
			}
			if in := r.w.Now(); in != nil {
				in.Player = r.name
				if offer(in) {
					return
				}
			}
		}
	}
}

// discordRunning reports whether Discord's connection is there (it is looked for, never connected to).
func (e *Engine) discordRunning() bool {
	if e.opts.NewDiscord != nil {
		return true // the tests' stand-in has no endpoint
	}
	paths := e.opts.DiscordPaths
	if paths == nil {
		paths = discord.SocketPaths()
	}
	for _, p := range paths {
		if pipe.Exists(p) {
			return true
		}
	}
	return false
}

// rest reports whether this check rests: Discord is not running and the engine may rest (Options.MayRest). It
// starts and ends the resting state; the Plex, Stremio and Jellyfin/Emby watchers run only while it does not rest.
func (e *Engine) rest(r *run) bool {
	want := e.opts.MayRest != nil && e.opts.MayRest() && !e.discordRunning()
	e.mu.Lock()
	if e.cur != r {
		e.mu.Unlock()
		return true
	}
	if want {
		if e.resting {
			e.mu.Unlock()
			return true
		}
		e.resting = true
		stop := e.stopRemote
		e.stopRemote, e.remotes = nil, nil
		e.player, e.vlcProblem, e.kodiProblem = "", "", ""
		e.shown, e.prev, e.nowPlaying, e.preview, e.paused = false, nil, nil, nil, false
		e.pausedSince, e.pauseCleared, e.hidden = time.Time{}, false, false
		e.statTick = time.Time{}
		e.mu.Unlock()
		if stop != nil {
			stop()
		}
		e.log("INFO", "Discord is not running: resting until it starts (no player is asked meanwhile).")
		if e.opts.OnRest != nil {
			e.opts.OnRest()
		}
		return true
	}
	was := e.resting
	e.resting = false
	if e.stopRemote == nil {
		e.startRemote(r)
	}
	e.mu.Unlock()
	if was {
		e.log("INFO", "Resting ended: looking for a player again.")
	}
	return false
}

// startRemote starts following the Plex, Stremio and Jellyfin or Emby accounts and servers the user signed in to (e.mu
// held).
func (e *Engine) startRemote(r *run) {
	cfg := e.cfg
	ctx, cancel := context.WithCancel(r.ctx)
	e.stopRemote = cancel
	e.remotes = nil
	add := func(name string, w watcher) {
		e.remotes = append(e.remotes, remote{name, w})
		Go(e.log, "The "+name+" watcher", func() { w.Run(ctx) })
	}
	if cfg.PlexToken != "" {
		add("Plex", &plex.Watcher{
			Client:  &plex.Client{HTTP: e.opts.HTTPClient, ClientID: cfg.PlexClient, Version: e.opts.Version, TVBase: e.opts.PlexTV, ResourcesBase: e.opts.PlexTV},
			Token:   cfg.PlexToken,
			Account: cfg.PlexAccount,
			Server:  cfg.PlexServer,
			Address: cfg.PlexAddress,
			Log:     e.log,
		})
	}
	if cfg.StremioKey != "" {
		add("Stremio", &stremio.Watcher{
			Client: &stremio.Client{HTTP: e.opts.HTTPClient, APIBase: e.opts.StremioAPI, LinkBase: e.opts.StremioAPI},
			Key:    cfg.StremioKey,
			Log:    e.log,
		})
	}
	if cfg.JellyfinToken != "" && cfg.JellyfinAddress != "" {
		add(orJellyfin(cfg.JellyfinProduct), &jellyfin.Watcher{ // Jellyfin or Emby
			Client:  &jellyfin.Client{HTTP: e.opts.HTTPClient, Base: cfg.JellyfinAddress, DeviceID: cfg.JellyfinDevice, Version: e.opts.Version},
			Token:   cfg.JellyfinToken,
			UserID:  cfg.JellyfinUser,
			Product: cfg.JellyfinProduct,
			Every:   e.opts.JellyfinEvery,
			Log:     e.log,
		})
	}
}

// Wake makes the engine check at once instead of at its next check (the window opened, so it must stop resting).
func (e *Engine) Wake() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

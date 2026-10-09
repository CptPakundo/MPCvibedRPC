// Package engine is the presence engine: it polls MPC-HC, looks up artwork and keeps Discord up to date.
// It can be started, stopped and given new settings at any time (the settings window drives it).
package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/artwork"
	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
)

// Options are the engine's collaborators; all of them are optional.
type Options struct {
	Log          func(level, msg string)
	CacheFile    string
	DiscordPaths []string     // nil = the usual IPC locations
	HTTPClient   *http.Client // used for MPC-HC and artwork lookups
	// PauseUnit is how long one "minute" of Config.PauseClearMinutes lasts (zero = a real minute; tests shorten it).
	PauseUnit time.Duration
	// DiscordAttempt, when set, replaces the Discord login (tests).
	NewDiscord func() *discord.Client
}

// Status is what the settings window shows.
type Status struct {
	Running    bool    `json:"running"`
	Discord    string  `json:"discord"` // off | waiting | connected
	MPC        bool    `json:"mpc"`
	NowPlaying *string `json:"nowPlaying"`
	Paused     bool    `json:"paused"`
	// PauseCleared is true while the status is hidden because the video stayed paused (see Config.PauseClearMinutes).
	PauseCleared bool `json:"pauseCleared"`
	LastError  *string `json:"lastError"`
	Since      *int64  `json:"since"`
}

// run is one start..stop period; a stale run can never touch the newer one's state.
type run struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type Engine struct {
	opts Options
	log  func(level, msg string)

	mu         sync.Mutex
	cfg        core.Config
	cur        *run
	rpc        *discord.Client
	ready      bool
	connecting bool
	warnedDisc bool
	shown      bool
	prev       *core.Prev
	basicMode  bool
	art        *artwork.Artwork
	mpcUp      bool
	nowPlaying *string
	paused     bool
	lastError  *string
	since      *int64

	// pause handling: when the current pause began, and whether the status was cleared because of it
	pausedSince  time.Time
	pauseCleared bool
	pauseUnit    time.Duration // one "minute" of PauseClearMinutes (shortened in tests)

	// stopMu serialises Start/Stop/ApplySettings.
	stopMu sync.Mutex
}

// New makes an engine with the given settings (not started).
func New(cfg core.Config, opts Options) *Engine {
	lg := opts.Log
	if lg == nil {
		lg = func(string, string) {}
	}
	unit := opts.PauseUnit
	if unit <= 0 {
		unit = time.Minute
	}
	return &Engine{opts: opts, log: lg, cfg: cfg, pauseUnit: unit}
}

// Config returns the settings the engine is using.
func (e *Engine) Config() core.Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

func (e *Engine) client() *http.Client {
	if e.opts.HTTPClient != nil {
		return e.opts.HTTPClient
	}
	return http.DefaultClient
}

func (e *Engine) fetchMPC(ctx context.Context, port int) *core.Info {
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+strconv.Itoa(port)+"/variables.html", nil)
	if err != nil {
		return nil
	}
	res, err := e.client().Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil
	}
	return core.ParseVariables(string(body))
}

// isCurrent reports whether r is still the active run.
func (e *Engine) isCurrent(r *run) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cur == r
}

func (e *Engine) connectDiscord(r *run) {
	e.mu.Lock()
	if e.ready || e.connecting || e.cur != r {
		e.mu.Unlock()
		return
	}
	e.connecting = true
	cfg := e.cfg
	e.mu.Unlock()

	go func() {
		var client *discord.Client
		if e.opts.NewDiscord != nil {
			client = e.opts.NewDiscord()
		} else {
			client = discord.New(e.opts.DiscordPaths)
		}
		err := client.Login(cfg.ClientID, 0)

		e.mu.Lock()
		if err != nil {
			e.connecting = false
			warn := !e.warnedDisc && e.cur == r
			if warn {
				e.warnedDisc = true
			}
			e.mu.Unlock()
			client.Close()
			if warn {
				e.log("WARN", fmt.Sprintf("Could not reach Discord (is the desktop app running?). Retrying in the background. [%s]", err.Error()))
			}
			return
		}
		if e.cur != r {
			e.connecting = false
			e.mu.Unlock()
			client.Close()
			return
		}
		e.rpc = client
		e.ready = true
		e.connecting = false
		e.warnedDisc = false
		e.prev = nil
		e.mu.Unlock()
		e.log("INFO", "Connected to Discord.")

		go func() {
			select {
			case <-client.Done():
			case <-r.ctx.Done():
				return
			}
			e.mu.Lock()
			if e.rpc != client {
				e.mu.Unlock()
				return
			}
			e.ready, e.rpc, e.shown, e.prev = false, nil, false, nil
			e.mu.Unlock()
			e.log("WARN", "Lost connection to Discord. Will reconnect when needed.")
		}()
	}()
}

func (e *Engine) sendActivity(rpc *discord.Client, cfg *core.Config, a *core.Activity) error {
	e.mu.Lock()
	basic := e.basicMode
	e.mu.Unlock()
	if !basic {
		err := rpc.SetActivity(a)
		if err == nil {
			return nil
		}
		e.log("WARN", fmt.Sprintf("Discord rejected the Watching activity (%s). Falling back to basic mode.", err.Error()))
		e.mu.Lock()
		e.basicMode = true
		e.mu.Unlock()
	}
	return rpc.SetActivity(core.LegacyActivity(a, cfg))
}

func (e *Engine) tick(r *run) {
	e.mu.Lock()
	cfg := e.cfg
	e.mu.Unlock()

	info := e.fetchMPC(r.ctx, cfg.Port)
	if !e.isCurrent(r) {
		return
	}

	e.mu.Lock()
	if (info != nil) != e.mpcUp {
		e.mpcUp = info != nil
		up := e.mpcUp
		e.mu.Unlock()
		if up {
			e.log("INFO", "MPC-HC detected.")
		} else {
			e.log("INFO", "MPC-HC not reachable (closed or web interface off).")
		}
		e.mu.Lock()
	}
	active := info != nil && info.File != "" && (info.State == 1 || info.State == 2)

	if !active {
		var clear *discord.Client
		if e.shown && e.ready {
			clear = e.rpc
		}
		e.mu.Unlock()
		if clear != nil {
			_ = clear.ClearActivity()
			e.log("INFO", "Nothing playing - presence cleared.")
		}
		e.mu.Lock()
		e.shown, e.prev, e.nowPlaying = false, nil, nil
		e.pausedSince, e.pauseCleared = time.Time{}, false
		e.mu.Unlock()
		return
	}
	if !e.ready {
		e.mu.Unlock()
		e.connectDiscord(r)
		return
	}
	rpc := e.rpc
	now := time.Now()
	if e.pauseCleared && (cfg.PauseClearMinutes <= 0 || info.State != 1) {
		// the setting was switched off, or playback resumed: show the status again
		e.pauseCleared = false
		e.prev = nil
	}
	update, next := core.NeedsUpdate(e.prev, info, now.UnixMilli())
	e.prev = next
	e.paused = info.State == 1
	if info.State != 1 {
		e.pausedSince = time.Time{}
	} else if update || e.pausedSince.IsZero() {
		e.pausedSince = now // pausing, seeking or switching file restarts the wait
	}
	if update {
		e.pauseCleared = false
	}
	if info.State == 1 && cfg.PauseClearMinutes > 0 && !update && now.Sub(e.pausedSince) >= time.Duration(cfg.PauseClearMinutes)*e.pauseUnit {
		// paused for long enough: take the status down until something changes
		var clear *discord.Client
		if !e.pauseCleared {
			e.pauseCleared = true
			if e.shown {
				clear = rpc
			}
			e.shown = false
		}
		e.mu.Unlock()
		if clear != nil {
			_ = clear.ClearActivity()
			e.log("INFO", fmt.Sprintf("Paused for %d min - presence cleared (it returns when you resume).", cfg.PauseClearMinutes))
		}
		return
	}
	art := e.art
	e.mu.Unlock()
	if !update {
		return
	}

	media := core.ParseMedia(info.File, &cfg, info.Dir)
	if info.Duration > 0 {
		media.DurationMin = int(jsRound(float64(info.Duration) / 60000))
	}
	if cfg.FolderEpisodeNumbers && media.Code != "" && info.FullDir != "" {
		if ents, err := os.ReadDir(info.FullDir); err == nil {
			names := make([]string, len(ents))
			for i, en := range ents {
				names[i] = en.Name()
			}
			if n := core.FolderEpisode(media, names); n != 0 && n != core.NA {
				media.BadgeEpisode = n
			}
		}
	}
	var found *core.Art
	if art != nil {
		found = art.Lookup(media)
	}
	if !e.isCurrent(r) {
		return
	}
	activity := core.BuildActivity(info, &cfg, time.Now().UnixMilli(), media, found)
	if err := e.sendActivity(rpc, &cfg, activity); err != nil {
		msg := err.Error()
		e.mu.Lock()
		e.lastError = &msg
		e.prev = nil
		e.mu.Unlock()
		e.log("ERROR", "Failed to update presence: "+msg)
		return
	}
	e.mu.Lock()
	e.shown = true
	d := media.Display
	e.nowPlaying = &d
	e.paused = info.State == 1
	e.mu.Unlock()
	word := "Paused"
	if info.State == 2 {
		word = "Playing"
	}
	tag := ""
	if found != nil {
		tag = " [artwork]"
	}
	e.log("INFO", fmt.Sprintf("%s: %s%s", word, media.Display, tag))
}

func jsRound(x float64) float64 {
	f := float64(int64(x))
	if x < 0 && f != x {
		f--
	}
	if x-f >= 0.5 {
		return f + 1
	}
	return f
}

func (e *Engine) loop(r *run, interval time.Duration) {
	for {
		func() {
			defer func() {
				if p := recover(); p != nil {
					e.log("ERROR", fmt.Sprintf("%v", p))
				}
			}()
			e.tick(r)
		}()
		t := time.NewTimer(interval)
		select {
		case <-r.ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// Start begins watching MPC-HC (no-op when already running).
func (e *Engine) Start() {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	e.start()
}

func (e *Engine) start() {
	e.mu.Lock()
	if e.cur != nil {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &run{ctx: ctx, cancel: cancel}
	e.cur = r
	e.basicMode, e.prev, e.shown, e.mpcUp, e.warnedDisc = false, nil, false, false, false
	e.pausedSince, e.pauseCleared = time.Time{}, false
	now := time.Now().UnixMilli()
	e.since = &now
	e.lastError, e.nowPlaying = nil, nil
	cfg := e.cfg
	cfgCopy := cfg
	e.art = artwork.New(&cfgCopy, artwork.Options{
		CacheFile:  e.opts.CacheFile,
		Log:        e.log,
		Client:     e.opts.HTTPClient,
		OnResolved: func(*core.Art) { e.mu.Lock(); e.prev = nil; e.mu.Unlock() },
	})
	e.mu.Unlock()
	e.log("INFO", fmt.Sprintf("Presence started. Watching MPC-HC on port %d.", cfg.Port))
	interval := time.Duration(cfg.PollInterval) * time.Millisecond
	if interval < 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	go e.loop(r, interval)
}

// Stop clears the presence (waiting at most 1.5 s for Discord) and disconnects.
func (e *Engine) Stop() {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	e.stop()
}

func (e *Engine) stop() {
	e.mu.Lock()
	r := e.cur
	if r == nil {
		e.mu.Unlock()
		return
	}
	e.cur = nil
	c, ready := e.rpc, e.ready
	e.mu.Unlock()
	r.cancel()

	if c != nil && ready {
		done := make(chan struct{})
		go func() { _ = c.ClearActivity(); close(done) }()
		select {
		case <-done:
		case <-time.After(1500 * time.Millisecond):
		}
	}
	if c != nil {
		c.Close()
	}
	e.mu.Lock()
	e.rpc, e.ready, e.connecting, e.shown, e.prev = nil, false, false, false, nil
	e.nowPlaying, e.since = nil, nil
	e.pausedSince, e.pauseCleared = time.Time{}, false
	e.mu.Unlock()
	e.log("INFO", "Presence stopped.")
}

// ApplySettings restarts the engine with new settings if it was running.
func (e *Engine) ApplySettings(cfg core.Config) {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	e.mu.Lock()
	was := e.cur != nil
	e.mu.Unlock()
	if was {
		e.stop()
	}
	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()
	if was {
		e.start()
	}
}

// ClearArtworkCache forgets every cover and title that was looked up (in memory and on disk). While presence is\n// running the current title is looked up again, so its card is refreshed.
func (e *Engine) ClearArtworkCache() {
	e.mu.Lock()
	art := e.art
	e.prev = nil
	e.mu.Unlock()
	if art != nil {
		art.ClearCache()
	} else if e.opts.CacheFile != "" {
		_ = os.Remove(e.opts.CacheFile)
	}
}

// Status is a snapshot for the settings window.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	running := e.cur != nil
	s := Status{Running: running, Discord: "off", Paused: e.paused, LastError: e.lastError, Since: e.since}
	if running {
		s.Discord = "waiting"
		if e.ready {
			s.Discord = "connected"
		}
		s.MPC = e.mpcUp
		s.NowPlaying = e.nowPlaying
		s.PauseCleared = e.pauseCleared
	}
	return s
}

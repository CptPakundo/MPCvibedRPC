// Package engine is the presence engine: it polls the player (MPC-HC, MPC-BE, MPC-QT, mpv, VLC, IINA, PotPlayer, a player
// found through MPRIS on Linux, Kodi, Plex, Jellyfin, Emby or Stremio), looks up artwork and keeps Discord up to date.
// It can be started, stopped and given new settings at any time (the settings window drives it).
package engine

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/artwork"
	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
	"github.com/CptPakundo/MPCvibedRPC/internal/kodi"
	"github.com/CptPakundo/MPCvibedRPC/internal/vlchttp"
)

// Options are the engine's collaborators; all of them are optional.
type Options struct {
	Log          func(level, msg string)
	CacheFile    string
	DiscordPaths []string     // nil = the usual IPC locations
	HTTPClient   *http.Client // used for MPC-HC, VLC, Plex, Stremio, Jellyfin and artwork lookups
	// PauseUnit is how long one "minute" of Config.PauseClearMinutes lasts (zero = a real minute; tests shorten it).
	PauseUnit time.Duration
	// NewDiscord, when set, replaces the Discord login (tests).
	NewDiscord func() *discord.Client
	// Pipes, when not nil, replaces the IPC endpoints asked after the web interface (tests); see PlayerPipes.
	Pipes func(cfg core.Config) []PlayerPipe
	// MPRIS, when not nil, replaces the MPRIS players asked on Linux (tests); see QueryMPRIS.
	MPRIS func(timeout time.Duration) []*core.Info
	// MPRISBus, when set, is the bus watched for MPRIS players' changes (tests; see events.go). Without it the real
	// session bus is watched only when MPRIS is nil.
	MPRISBus string
	// KodiEvents replaces where Kodi's notifications are read from (tests): "none" reads none.
	KodiEvents string
	// PotPlayer, when not nil, replaces the PotPlayer windows asked on Windows (tests); see potplayer.Query.
	PotPlayer func() []*core.Info
	// Version is the program's version (Plex and Jellyfin are told it).
	Version string
	// PlexTV, when set, replaces plex.tv and clients.plex.tv (tests).
	PlexTV string
	// StatsFile keeps the counts of what was shown (see Stats; "" = counted in memory only).
	StatsFile string
	// StremioAPI, when set, replaces Stremio's account services (tests).
	StremioAPI string
	// JellyfinEvery, when set, replaces how often a Jellyfin or Emby server is asked (tests).
	JellyfinEvery time.Duration
	// MayRest, when set, says whether the engine may rest while Discord is not running (the program: while the
	// settings window is closed). Resting, it asks no player and follows no Plex, Jellyfin or Emby server or Stremio
	// account; it only looks for Discord's connection every check. nil = never rest.
	MayRest func() bool
	// OnRest is called when the engine starts resting (the program hands memory back to the system).
	OnRest func()
}

// Status is what the settings window shows.
type Status struct {
	Running    bool    `json:"running"`
	Discord    string  `json:"discord"` // off | standby (nothing played yet) | waiting (Discord not found yet) | connected
	MPC        bool    `json:"mpc"`     // some player answers (the name is historic; it is not only MPC)
	Player     string  `json:"player"`  // which one: MPC-HC, MPC-BE, MPC-QT, mpv, VLC, IINA, PotPlayer, a Linux player found through MPRIS, Kodi, Plex, Jellyfin, Emby or Stremio
	NowPlaying *string `json:"nowPlaying"`
	Paused     bool    `json:"paused"`
	// PauseCleared is true while the status is hidden because the video stayed paused (see Config.PauseClearMinutes).
	PauseCleared bool `json:"pauseCleared"`
	// Hidden is true while the playing file is on the user's "don't show" list (nothing is shown or looked up).
	Hidden bool `json:"hidden"`
	// Preview is how the current status looks on Discord (nil when nothing is shown).
	Preview *core.Preview `json:"preview"`
	// Hint explains why a player that seems to be there does not answer (VLC refusing the password, for example).
	Hint      string  `json:"hint,omitempty"`
	LastError *string `json:"lastError"`
	Since     *int64  `json:"since"`
	// Resting is true while Discord is not running and nothing is asked (see Options.MayRest; never while the
	// window is open).
	Resting bool `json:"resting,omitempty"`
	// DiscordProblem says why Discord cannot be reached when the reason is known (it refused the connection).
	DiscordProblem string `json:"discordProblem,omitempty"`
	// Why is "Why this title?" for Copy diagnostics: how the title shown was read from the file and found in the
	// catalogs. Never for a hidden title or a song. Not sent to the window.
	Why []string `json:"-"`
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
	shown      bool      // a status may be up on Discord (also after a send whose answer did not come)
	clearedAt  time.Time // when the status was last cleared: while nothing plays it is cleared again now and then
	clearStuck bool      // the last clear failed (logged once until one works)
	prev       *core.Prev
	basicMode  bool
	noLinks    bool // Discord refused the clickable title and cover: they are sent without links
	art        *artwork.Artwork
	mpcUp      bool
	player     string // the player that answered last
	vlc        *vlchttp.Reader
	vlcProblem string             // why VLC's web interface did not answer (logged once; the window shows it)
	remotes    []remote           // the Plex, Stremio and Jellyfin/Emby watchers of the run, in the order they are asked
	stopRemote context.CancelFunc // ends the remotes (they stop while resting)
	resting    bool               // Discord is not running: nothing is asked until it is (see Options.MayRest)
	wake       chan struct{}      // makes the loop check at once (the window opened)
	nudges     nudger             // early checks after mpv or IINA reported a change (events.go)
	nowPlaying *string
	why        []string // how the title shown was read and found (Status.Why; only while nowPlaying is set)
	preview    *core.Preview
	paused     bool
	lastError  *string
	since      *int64

	discRefused bool // the last attempt to reach Discord was refused (Discord running as administrator)
	kodi        *kodi.Reader
	kodiProblem string // why Kodi did not answer (logged once; the window shows it)

	// pause handling: when the current pause began, and whether the status was cleared because of it
	pausedSince  time.Time
	pauseCleared bool
	hidden       bool          // the playing file is on the user's "don't show" list
	pauseUnit    time.Duration // one "minute" of PauseClearMinutes (shortened in tests)

	// the counts kept for the window (stats.go): what was shown and for how long; the key only tells videos apart
	stats     Stats
	statDirty bool
	statSaved time.Time
	statTick  time.Time
	statKey   string
	statFound bool

	// an episode shown without its title (a catalog did not answer) is asked about again a few times while it plays
	recheckKey string
	recheckAt  time.Time
	rechecks   int

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
	now := time.Now()
	return &Engine{opts: opts, log: lg, cfg: cfg, pauseUnit: unit, vlc: &vlchttp.Reader{Client: opts.HTTPClient}, kodi: &kodi.Reader{Client: opts.HTTPClient},
		stats: loadStats(opts.StatsFile, now), statSaved: now, wake: make(chan struct{}, 1)}
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

// isCurrent reports whether r is still the active run.
func (e *Engine) isCurrent(r *run) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cur == r
}

// Start begins watching for a player (no-op when already running).
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
	e.basicMode, e.noLinks, e.prev, e.shown, e.mpcUp, e.warnedDisc, e.player = false, false, nil, false, false, false, ""
	e.pausedSince, e.pauseCleared, e.preview, e.hidden = time.Time{}, false, nil, false
	e.vlcProblem, e.kodiProblem, e.discRefused = "", "", false
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
	e.remotes, e.stopRemote, e.resting = nil, nil, false // the first check starts them (unless it rests)
	e.mu.Unlock()
	e.log("INFO", "Presence started. Looking for a player on "+lookingOn(cfg)+".")
	go e.loop(r, pollInterval(cfg))
	go e.watchPlayers(r)
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
		// Closing a pipe that is stuck in a write (Discord stopped reading) can block in Windows until the write
		// ends; stopping, and quitting, must not wait for that.
		closed := make(chan struct{})
		go func() { c.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(1500 * time.Millisecond):
			e.log("WARN", "Discord did not let go of the connection; leaving it behind.")
		}
	}
	e.mu.Lock()
	e.rpc, e.ready, e.connecting, e.shown, e.prev = nil, false, false, false, nil
	e.nowPlaying, e.since, e.preview = nil, nil, nil
	e.pausedSince, e.pauseCleared, e.hidden, e.paused = time.Time{}, false, false, false
	e.remotes, e.stopRemote, e.resting = nil, nil, false // their runs ended with the run's context
	e.saveStats(time.Now())
	e.statTick = time.Time{}
	if e.art != nil {
		e.art.Close() // lookups still under way finish quietly: no title in the log, no refresh (hideTitle may be on now)
	}
	e.mu.Unlock()
	e.log("INFO", "Presence stopped.")
}

// ApplySettings restarts the engine with new settings if it was running. Settings that did not change (a save of
// an app-only option such as "Start with Windows") leave it alone, so Discord is not cleared and reconnected for nothing.
func (e *Engine) ApplySettings(cfg core.Config) {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	e.mu.Lock()
	was := e.cur != nil
	same := reflect.DeepEqual(e.cfg, cfg)
	e.mu.Unlock()
	if same {
		return
	}
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

// ClearArtworkCache forgets every cover and title that was looked up (in memory and on disk). While presence is
// running the current title is looked up again, so its card is refreshed.
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
	s := Status{Running: running, Discord: "off", LastError: e.lastError, Since: e.since}
	if running {
		// Discord is only contacted once something plays; until then it is on standby, not missing
		switch {
		case e.ready:
			s.Discord = "connected"
		case e.connecting || e.warnedDisc:
			s.Discord = "waiting"
		default:
			s.Discord = "standby"
		}
		if e.discRefused && !e.ready {
			s.DiscordProblem = "Discord refused the connection. It is probably running as administrator: start it normally."
		}
		s.Paused = e.paused
		s.MPC = e.mpcUp
		s.Player = e.player
		s.NowPlaying = e.nowPlaying
		if e.nowPlaying != nil {
			s.Why = e.why
		}
		s.PauseCleared = e.pauseCleared
		s.Hidden = e.hidden
		s.Preview = e.preview
		var hints []string
		if e.vlcProblem != "" {
			hints = append(hints, e.vlcProblem)
		}
		if e.kodiProblem != "" {
			hints = append(hints, e.kodiProblem)
		}
		for _, r := range e.remotes {
			if p := r.w.Problem(); p != "" {
				hints = append(hints, p)
			}
		}
		s.Hint = strings.Join(hints, " ")
		s.Resting = e.resting
	}
	return s
}

// Package engine is the presence engine: it polls the player (MPC-HC, MPC-BE, MPC-QT, mpv, VLC, IINA, a player found
// through MPRIS on Linux, or Plex), looks up artwork and keeps Discord up to date.
// It can be started, stopped and given new settings at any time (the settings window drives it).
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/artwork"
	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
	"github.com/CptPakundo/MPCvibedRPC/internal/mpris"
	"github.com/CptPakundo/MPCvibedRPC/internal/mpvipc"
	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
	"github.com/CptPakundo/MPCvibedRPC/internal/plex"
	"github.com/CptPakundo/MPCvibedRPC/internal/vlchttp"
)

// Options are the engine's collaborators; all of them are optional.
type Options struct {
	Log          func(level, msg string)
	CacheFile    string
	DiscordPaths []string     // nil = the usual IPC locations
	HTTPClient   *http.Client // used for MPC-HC, VLC and artwork lookups
	// PauseUnit is how long one "minute" of Config.PauseClearMinutes lasts (zero = a real minute; tests shorten it).
	PauseUnit time.Duration
	// DiscordAttempt, when set, replaces the Discord login (tests).
	NewDiscord func() *discord.Client
	// Pipes, when not nil, replaces the IPC endpoints asked after the web interface (tests); see PlayerPipes.
	Pipes func(cfg core.Config) []PlayerPipe
	// MPRIS, when not nil, replaces the MPRIS players asked on Linux (tests); see QueryMPRIS.
	MPRIS func(timeout time.Duration) []*core.Info
	// Version is the program's version (Plex is told it).
	Version string
	// PlexTV, when set, replaces plex.tv and clients.plex.tv (tests).
	PlexTV string
	// StatsFile keeps the counts of what was shown (see Stats; "" = counted in memory only).
	StatsFile string
}

// PlayerPipe is an mpv-style IPC endpoint and the name of the player behind it.
type PlayerPipe struct{ Name, Path string }

// PlayerPipes are the endpoints asked when no web interface answers: MPC-QT's, which is always there, mpv's, which
// the user turns on with input-ipc-server (empty mpvPipe = don't look for mpv), and on macOS IINA's (an mpv option
// in IINA's settings; empty iinaPipe = don't look for IINA).
func PlayerPipes(cfg core.Config) []PlayerPipe {
	out := []PlayerPipe{{"MPC-QT", pipe.Path("cmdrkotori.mpc-qt.mpv")}}
	if cfg.MpvPipe != "" {
		out = append(out, PlayerPipe{"mpv", pipe.Path(cfg.MpvPipe)})
	}
	if runtime.GOOS == "darwin" && cfg.IinaPipe != "" {
		out = append(out, PlayerPipe{"IINA", pipe.Path(cfg.IinaPipe)})
	}
	return out
}

// HasMPRIS is true where players are read through MPRIS (Linux and the other systems that use D-Bus).
const HasMPRIS = runtime.GOOS != "windows" && runtime.GOOS != "darwin"

// QueryMPRIS asks the video players on the user's session bus.
func QueryMPRIS(timeout time.Duration) []*core.Info {
	return mpris.Query(dbus.SessionAddress(), timeout)
}

// Status is what the settings window shows.
type Status struct {
	Running    bool    `json:"running"`
	Discord    string  `json:"discord"` // off | standby (nothing played yet) | waiting (Discord not found yet) | connected
	MPC        bool    `json:"mpc"`     // a player answers
	Player     string  `json:"player"`  // which one: MPC-HC, MPC-BE, MPC-QT, mpv, VLC, IINA, a Linux player found through MPRIS, or Plex
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
	player     string // the player that answered last
	vlc        *vlchttp.Reader
	vlcProblem string // why VLC's web interface did not answer (logged once; the window shows it)
	plex       *plex.Watcher
	nowPlaying *string
	preview    *core.Preview
	paused     bool
	lastError  *string
	since      *int64

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
	return &Engine{opts: opts, log: lg, cfg: cfg, pauseUnit: unit, vlc: &vlchttp.Reader{Client: opts.HTTPClient},
		stats: loadStats(opts.StatsFile, now), statSaved: now}
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
	info := core.ParseVariables(string(body))
	if info != nil {
		info.Player = core.PlayerOf(string(body))
	}
	return info
}

// fetchVLC asks VLC's web interface. Problems that need the user (a wrong password, a port taken by another program)
// are logged once and kept for the window.
func (e *Engine) fetchVLC(ctx context.Context, cfg core.Config) *core.Info {
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	in, err := e.vlc.Query(ctx, cfg.VlcPort, cfg.VlcPassword)
	problem := ""
	switch {
	case errors.Is(err, vlchttp.ErrPassword):
		problem = "VLC refused the password. It has to match the one in VLC (Preferences > All > Interface > Main interfaces > Lua); Set up the player connection fills it in."
	case errors.Is(err, vlchttp.ErrNoPassword):
		problem = "VLC's web interface has no password, so VLC keeps it closed. Press Set up the player connection, then restart VLC."
	case errors.Is(err, vlchttp.ErrNotVLC):
		problem = fmt.Sprintf("Port %d answers, but not as VLC. Another program may be using it (Advanced > VLC web interface port).", cfg.VlcPort)
	}
	e.mu.Lock()
	changed := problem != e.vlcProblem
	e.vlcProblem = problem
	e.mu.Unlock()
	if changed && problem != "" {
		e.log("WARN", problem)
	}
	if err != nil {
		return nil
	}
	in.Player = "VLC"
	return in
}

// fetchPlayer asks the web interface first (MPC-HC and MPC-BE only have that), then the mpv-style connections, then VLC.
func (e *Engine) fetchPlayer(ctx context.Context, cfg core.Config) *core.Info {
	pipes := PlayerPipes
	if e.opts.Pipes != nil {
		pipes = e.opts.Pipes
	}
	ask := func(p PlayerPipe) *core.Info {
		conn, err := pipe.Dial(p.Path)
		if err != nil {
			return nil // not running, or not set up
		}
		in, err := mpvipc.Query(conn, 1500*time.Millisecond)
		if err != nil {
			return nil
		}
		in.Player = p.Name
		return in
	}
	// The first player that is playing something wins; failing that, the first one that answers (an idle player
	// that is open must not hide another that plays).
	var first *core.Info
	playing := func(in *core.Info) bool {
		if in == nil {
			return false
		}
		if first == nil {
			first = in
		}
		return in.File != "" && in.State >= 1 && in.State <= 3
	}
	info := e.fetchMPC(ctx, cfg.Port)
	askedQt := false
	if info != nil && info.Player == "MPC-QT" {
		// MPC-QT's page reports a meaningless speed; its own connection knows the real one
		web := info
		for _, p := range pipes(cfg) {
			if p.Name == "MPC-QT" {
				askedQt = true
				if in := ask(p); in != nil {
					info = in
					break
				}
			}
		}
		if info == web {
			info.Rate = 1
		}
	}
	if !playing(info) {
		info = nil
		for _, p := range pipes(cfg) {
			if ctx.Err() != nil {
				return nil
			}
			if askedQt && p.Name == "MPC-QT" {
				continue
			}
			if in := ask(p); playing(in) {
				info = in
				break
			}
		}
		if info == nil && cfg.VlcPassword != "" {
			if ctx.Err() != nil {
				return nil
			}
			if in := e.fetchVLC(ctx, cfg); playing(in) {
				info = in
			}
		}
		if info == nil && HasMPRIS && cfg.Mpris {
			if ctx.Err() != nil {
				return nil
			}
			query := QueryMPRIS
			if e.opts.MPRIS != nil {
				query = e.opts.MPRIS
			}
			for _, in := range query(1500 * time.Millisecond) {
				if playing(in) {
					info = in
					break
				}
			}
		}
		if info == nil {
			e.mu.Lock()
			w := e.plex
			e.mu.Unlock()
			if w != nil {
				if in := w.Now(); in != nil {
					in.Player = "Plex"
					if playing(in) {
						info = in
					}
				}
			}
		}
		if info == nil {
			info = first
		}
	}
	if info == nil {
		return nil
	}
	if info.State == 3 {
		info.State = 2 // MPC-QT reports a seek in progress as its own state; it is still playing
	}
	if !(info.Rate >= 0.05 && info.Rate <= 100) {
		info.Rate = 1 // MPC-QT's web interface can report a meaningless speed
	}
	return info
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
			e.ready, e.rpc, e.shown, e.prev, e.preview = false, nil, false, nil, nil
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
	e.statTime(time.Now(), pollInterval(cfg))
	e.mu.Unlock()

	info := e.fetchPlayer(r.ctx, cfg)
	if !e.isCurrent(r) {
		return
	}
	if info != nil && cfg.AppName == core.DefaultConfig().AppName {
		// the default name describes Media Player Classic; other players are called what they are
		switch info.Player {
		case "MPC-HC", "MPC-BE", "MPC-QT", "":
		case "VLC":
			cfg.AppName = "VLC media player"
		default:
			cfg.AppName = info.Player
		}
	}

	e.mu.Lock()
	player := ""
	if info != nil {
		player = info.Player
	}
	if (info != nil) != e.mpcUp || player != e.player {
		e.mpcUp, e.player = info != nil, player
		e.mu.Unlock()
		if player != "" {
			e.log("INFO", player+" detected.")
		} else {
			e.log("INFO", "No player reachable (closed, or its web interface or IPC is off).")
		}
		e.mu.Lock()
	}
	active := info != nil && info.File != "" && (info.State == 1 || info.State == 2)
	hidden := active && core.MatchesHideList(info.File, info.FullDir, cfg.HideFiles)

	if !active || hidden {
		var clear *discord.Client
		if e.shown && e.ready {
			clear = e.rpc
		}
		e.mu.Unlock()
		if clear != nil {
			_ = clear.ClearActivity()
			if hidden {
				e.log("INFO", "This file is on your don't-show list - presence cleared.")
			} else {
				e.log("INFO", "Nothing playing - presence cleared.")
			}
		}
		e.mu.Lock()
		e.hidden, e.paused = hidden, false
		e.shown, e.prev, e.nowPlaying, e.preview = false, nil, nil, nil
		e.pausedSince, e.pauseCleared = time.Time{}, false
		e.mu.Unlock()
		return
	}
	e.hidden = false
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
	if !update && info.State == 2 && !e.recheckAt.IsZero() && now.After(e.recheckAt) && e.recheckKey == info.Player+"/"+info.File {
		update = true // look the episode title up again (see after sending)
		e.recheckAt = time.Time{}
	}
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
			e.preview = nil
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

	var (
		activity  *core.Activity
		display   string
		found     *core.Art
		noEpTitle bool // an episode whose title was wanted but did not come
	)
	if cfg.HideTitle {
		// nothing about the file is used: no parsing, no lookups, and no title in the status, the window or the log
		activity = core.HiddenActivity(info, &cfg, time.Now().UnixMilli())
		display = "A video (title hidden)"
	} else {
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
		if art != nil {
			found = art.Lookup(media)
		}
		if !e.isCurrent(r) {
			return
		}
		activity = core.BuildActivity(info, &cfg, time.Now().UnixMilli(), media, found)
		noEpTitle = cfg.ShowArtwork && cfg.EpisodeTitles && media.IsEpisode && media.Episode != core.NA && media.EpTitle == "" &&
			(found == nil || found.Episode == nil)
		display = media.Display
	}
	if !e.isCurrent(r) {
		return
	}
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
	d := display
	e.statShown(info.Player+"/"+info.File, info.Player, found != nil) // player names have no slash
	if key := info.Player + "/" + info.File; noEpTitle {
		if e.recheckKey != key {
			e.recheckKey, e.rechecks, e.recheckAt = key, 0, time.Time{}
		}
		if e.recheckAt.IsZero() && e.rechecks < len(recheckAfter) {
			e.recheckAt = time.Now().Add(time.Duration(float64(e.pauseUnit) * recheckAfter[e.rechecks]))
			e.rechecks++
		}
	} else if e.recheckKey == key {
		e.recheckAt = time.Time{}
	}
	e.nowPlaying = &d
	e.paused = info.State == 1
	sent := activity
	if e.basicMode { // the fallback carries less: show what Discord actually got
		sent = core.LegacyActivity(activity, &cfg)
	}
	e.preview = core.PreviewOf(sent, &cfg)
	e.mu.Unlock()
	word := "Paused"
	if info.State == 2 {
		word = "Playing"
	}
	if cfg.HideTitle {
		e.log("INFO", fmt.Sprintf("%s: (title hidden)", word))
		return
	}
	tag := ""
	if found != nil {
		tag = " [artwork]"
	}
	e.log("INFO", fmt.Sprintf("%s: %s%s", word, display, tag))
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
	e.basicMode, e.prev, e.shown, e.mpcUp, e.warnedDisc, e.player = false, nil, false, false, false, ""
	e.pausedSince, e.pauseCleared, e.preview, e.hidden = time.Time{}, false, nil, false
	e.vlcProblem = ""
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
	e.plex = nil
	if cfg.PlexToken != "" {
		e.plex = &plex.Watcher{
			Client:  &plex.Client{HTTP: e.opts.HTTPClient, ClientID: cfg.PlexClient, Version: e.opts.Version, TVBase: e.opts.PlexTV, ResourcesBase: e.opts.PlexTV},
			Token:   cfg.PlexToken,
			Account: cfg.PlexAccount,
			Server:  cfg.PlexServer,
			Address: cfg.PlexAddress,
			Log:     e.log,
		}
		go e.plex.Run(ctx)
	}
	e.mu.Unlock()
	places := []string{fmt.Sprintf("the web interface on port %d (MPC-HC, MPC-BE, MPC-QT)", cfg.Port), "MPC-QT's own connection"}
	if cfg.MpvPipe != "" {
		places = append(places, fmt.Sprintf("mpv's (%s)", cfg.MpvPipe))
	}
	if runtime.GOOS == "darwin" && cfg.IinaPipe != "" {
		places = append(places, fmt.Sprintf("IINA's (%s)", cfg.IinaPipe))
	}
	if cfg.VlcPassword != "" {
		places = append(places, fmt.Sprintf("VLC's web interface on port %d", cfg.VlcPort))
	}
	if HasMPRIS && cfg.Mpris {
		places = append(places, "the video players on D-Bus (MPRIS)")
	}
	if cfg.PlexToken != "" {
		places = append(places, "your Plex server")
	}
	looking := strings.Join(places[:len(places)-1], ", ") + " and " + places[len(places)-1]
	e.log("INFO", "Presence started. Looking for a player on "+looking+".")
	go e.loop(r, pollInterval(cfg))
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
	e.plex = nil // its run ended with the run's context
	e.saveStats(time.Now())
	e.statTick = time.Time{}
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
		s.Paused = e.paused
		s.MPC = e.mpcUp
		s.Player = e.player
		s.NowPlaying = e.nowPlaying
		s.PauseCleared = e.pauseCleared
		s.Hidden = e.hidden
		s.Preview = e.preview
		s.Hint = e.vlcProblem
		if e.plex != nil {
			if p := e.plex.Problem(); p != "" && s.Hint == "" {
				s.Hint = p
			} else if p != "" {
				s.Hint += " " + p
			}
		}
	}
	return s
}

// pollInterval is how often the players are asked (at least every 250 ms).
func pollInterval(cfg core.Config) time.Duration {
	interval := time.Duration(cfg.PollInterval) * time.Millisecond
	if interval < 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	return interval
}

// recheckAfter is when an episode shown without its title is looked up again, in minutes after it was shown (or after
// the previous try): a catalog that did not answer gets another chance while the episode plays.
var recheckAfter = []float64{2.5, 5, 15}

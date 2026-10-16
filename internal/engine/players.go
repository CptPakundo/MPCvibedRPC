package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
	"github.com/CptPakundo/MPCvibedRPC/internal/mpris"
	"github.com/CptPakundo/MPCvibedRPC/internal/mpvipc"
	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
	"github.com/CptPakundo/MPCvibedRPC/internal/vlchttp"
)

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
		problem = fmt.Sprintf("Port %d answers, but not as VLC. Another program may be using it (Advanced > Players > VLC web interface port).", cfg.VlcPort)
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

// askPipe asks one mpv-style IPC endpoint; nil when the player is not running or not set up.
func askPipe(p PlayerPipe) *core.Info {
	conn, err := pipe.Dial(p.Path)
	if err != nil {
		return nil
	}
	in, err := mpvipc.Query(conn, 1500*time.Millisecond)
	if err != nil {
		return nil
	}
	in.Player = p.Name
	return in
}

// picker chooses among the players that answered: the first that is playing a video wins; failing that, the first
// one playing music, then the first one that is paused, then the first one that answers (a paused or idle player that
// is open must not hide another that plays, and music in the background must not hide a video).
type picker struct{ first, music, paused *core.Info }

// offer takes one answer (nil = none) and reports whether it is a video that plays, which ends the search.
func (p *picker) offer(in *core.Info) (wins bool) {
	if in == nil {
		return false
	}
	if p.first == nil {
		p.first = in
	}
	if in.File != "" && in.State == 1 && p.paused == nil {
		p.paused = in
	}
	if in.File == "" || (in.State != 2 && in.State != 3) {
		return false
	}
	if core.IsAudioFile(in.File) {
		if p.music == nil {
			p.music = in
		}
		return false // keep looking for a video
	}
	return true
}

// result is the best answer when no video wins: music, else a paused file, else whatever answered (nil if nothing did).
func (p *picker) result() *core.Info {
	switch {
	case p.music != nil:
		return p.music
	case p.paused != nil:
		return p.paused
	}
	return p.first
}

// source offers the answers of one kind of player, one at a time, until offer returns true.
type source func(offer func(*core.Info) bool)

// fetchPlayer asks the web interface first (MPC-HC and MPC-BE only have that), then MPC-QT's pipe, mpv, IINA, VLC,
// the MPRIS players, Plex, Stremio and last Jellyfin/Emby.
func (e *Engine) fetchPlayer(ctx context.Context, cfg core.Config) *core.Info {
	pipes := PlayerPipes
	if e.opts.Pipes != nil {
		pipes = e.opts.Pipes
	}
	var pk picker
	info := e.fetchMPC(ctx, cfg.Port)
	askedQt := false
	if info != nil && info.Player == "MPC-QT" {
		// MPC-QT's page reports a meaningless speed; its own connection knows the real one
		web := info
		for _, p := range pipes(cfg) {
			if p.Name == "MPC-QT" {
				askedQt = true
				if in := askPipe(p); in != nil {
					info = in
					break
				}
			}
		}
		if info == web {
			info.Rate = 1
		}
	}
	if !pk.offer(info) {
		info = nil
		sources := []source{func(offer func(*core.Info) bool) {
			for _, p := range pipes(cfg) {
				if ctx.Err() != nil {
					return
				}
				if askedQt && p.Name == "MPC-QT" {
					continue
				}
				if offer(askPipe(p)) {
					return
				}
			}
		}}
		if cfg.VlcPassword != "" {
			sources = append(sources, func(offer func(*core.Info) bool) { offer(e.fetchVLC(ctx, cfg)) })
		}
		if HasMPRIS && cfg.Mpris {
			sources = append(sources, func(offer func(*core.Info) bool) {
				query := QueryMPRIS
				if e.opts.MPRIS != nil {
					query = e.opts.MPRIS
				}
				for _, in := range query(1500 * time.Millisecond) {
					if offer(in) {
						return
					}
				}
			})
		}
		sources = append(sources, e.remoteSource(ctx))
		offer := func(in *core.Info) bool {
			if !pk.offer(in) {
				return false
			}
			info = in
			return true
		}
		for _, src := range sources {
			if ctx.Err() != nil {
				return nil
			}
			if src(offer); info != nil {
				break
			}
		}
		if info == nil {
			if ctx.Err() != nil {
				return nil
			}
			info = pk.result()
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

// lookingOn names the places a started engine looks for a player, for the log.
func lookingOn(cfg core.Config) string {
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
	if cfg.StremioKey != "" {
		places = append(places, "your Stremio account")
	}
	if cfg.JellyfinToken != "" && cfg.JellyfinAddress != "" {
		places = append(places, "your "+orJellyfin(cfg.JellyfinProduct)+" server")
	}
	return strings.Join(places[:len(places)-1], ", ") + " and " + places[len(places)-1]
}

// orJellyfin names a signed-in server whose kind was not saved.
func orJellyfin(product string) string {
	if product == "" {
		return "Jellyfin"
	}
	return product
}

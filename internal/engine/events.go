package engine

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/conns"
	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
	"github.com/CptPakundo/MPCvibedRPC/internal/kodi"
	"github.com/CptPakundo/MPCvibedRPC/internal/mpris"
	"github.com/CptPakundo/MPCvibedRPC/internal/mpvipc"
	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
)

var (
	watchRetry = 5 * time.Second        // how often a player that is not there is looked for again
	nudgeDelay = 250 * time.Millisecond // after an event: lets the player settle, and makes one check of a burst (a seek)
	nudgeGap   = 4 * time.Second        // at most one early check this often: Discord takes about 5 updates in 20 s
)

// nudger turns player events into early checks, paced by nudgeGap.
type nudger struct {
	mu    sync.Mutex
	timer *time.Timer
	last  time.Time
}

// nudge asks the engine for a check soon. Events that come while one is waiting are folded into it.
func (e *Engine) nudge() {
	n := &e.nudges
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.timer != nil {
		return
	}
	wait := nudgeDelay
	if since := time.Since(n.last); since < nudgeGap && nudgeGap-since > wait {
		wait = nudgeGap - since
	}
	n.timer = time.AfterFunc(wait, func() {
		n.mu.Lock()
		n.timer, n.last = nil, time.Now()
		n.mu.Unlock()
		e.Wake()
	})
}

// eventSource is one player connection that reports changes: open connects (nil when the player is not there), and
// the watch it returns runs until the connection ends.
type eventSource struct {
	key  string
	open func() (io.Closer, func() error)
}

// eventSources are the players that can tell of a change themselves: mpv and IINA (mpv's own IPC; MPC-QT's pipe is
// only asked), the MPRIS players on Linux (D-Bus signals) and Kodi (its JSON-RPC over TCP, port 9090).
func (e *Engine) eventSources(cfg core.Config) []eventSource {
	var out []eventSource
	pipes := PlayerPipes
	if e.opts.Pipes != nil {
		pipes = e.opts.Pipes
	}
	for _, p := range pipes(cfg) {
		if p.Name != "mpv" && p.Name != "IINA" {
			continue
		}
		path := p.Path
		out = append(out, eventSource{"pipe:" + path, func() (io.Closer, func() error) {
			c, err := pipe.Dial(path)
			if err != nil {
				return nil, nil
			}
			return c, func() error { return mpvipc.Watch(c, e.nudge) }
		}})
	}
	if bus := e.mprisBus(cfg); bus != "" {
		out = append(out, eventSource{"mpris", func() (io.Closer, func() error) {
			c, err := dbus.Dial(bus, 2*time.Second)
			if err != nil {
				return nil, nil
			}
			return c, func() error { return mpris.Watch(c, 2*time.Second, e.nudge) }
		}})
	}
	if addr := e.kodiEvents(cfg); addr != "" {
		out = append(out, eventSource{"kodi:" + addr, func() (io.Closer, func() error) {
			c, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				return nil, nil
			}
			if host, _, err := net.SplitHostPort(addr); err == nil {
				conns.Note(host) // a connection outside this computer is listed under Recent connections, like a request
			}
			return c, func() error { return kodi.Watch(c, e.nudge) }
		}})
	}
	return out
}

// mprisBus is the session bus to watch: the real one only where MPRIS players are read for real (tests that replace
// the MPRIS players set Options.MPRISBus to a fake bus, or get none).
func (e *Engine) mprisBus(cfg core.Config) string {
	if !HasMPRIS || !cfg.Mpris {
		return ""
	}
	if e.opts.MPRISBus != "" {
		return e.opts.MPRISBus
	}
	if e.opts.MPRIS != nil {
		return ""
	}
	return dbus.SessionAddress()
}

// kodiEvents is where Kodi's notifications come from, once Kodi is set up (its password is set).
func (e *Engine) kodiEvents(cfg core.Config) string {
	if cfg.KodiPassword == "" {
		return ""
	}
	switch e.opts.KodiEvents {
	case "":
	case "none":
		return ""
	default:
		return e.opts.KodiEvents
	}
	return kodi.EventAddress(cfg.KodiAddress)
}

// watchPlayers keeps a connection open to each player that reports its changes, so that a pause, a seek or a new
// file shows at once instead of at the next check. What plays is still read by the checks; a player that is not
// there is looked for again every watchRetry, and none is while the engine rests.
func (e *Engine) watchPlayers(r *run) {
	var mu sync.Mutex
	watching := map[string]bool{}
	t := time.NewTicker(watchRetry)
	defer t.Stop()
	for {
		e.mu.Lock()
		cfg, resting := e.cfg, e.resting
		e.mu.Unlock()
		for _, s := range e.eventSources(cfg) {
			mu.Lock()
			busy := watching[s.key]
			if !resting && !busy {
				watching[s.key] = true
			}
			mu.Unlock()
			if resting || busy {
				continue
			}
			go func(s eventSource) { // on its own: a Kodi that does not answer must not hold up the others
				defer func() {
					mu.Lock()
					delete(watching, s.key)
					mu.Unlock()
				}()
				if r.ctx.Err() != nil {
					return
				}
				c, watch := s.open()
				if c == nil {
					return
				}
				stop := context.AfterFunc(r.ctx, func() { c.Close() })
				defer stop()
				_ = watch()
			}(s)
		}
		select {
		case <-r.ctx.Done():
			return
		case <-t.C:
		}
	}
}

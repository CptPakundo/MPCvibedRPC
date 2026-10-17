package engine

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/mpvipc"
	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
)

var (
	watchRetry = 5 * time.Second        // how often a closed mpv or IINA is looked for again
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

// watchPlayers keeps an event connection to mpv and IINA (mpv's own IPC), so that a pause, a seek or a new file
// shows at once instead of at the next check. What plays is still read by the checks; a player that is closed is
// looked for again every watchRetry, and none is while the engine rests. MPC-QT's pipe is only asked, not watched.
func (e *Engine) watchPlayers(r *run) {
	var mu sync.Mutex
	watching := map[string]bool{}
	t := time.NewTicker(watchRetry)
	defer t.Stop()
	for {
		e.mu.Lock()
		cfg, resting := e.cfg, e.resting
		e.mu.Unlock()
		pipes := PlayerPipes
		if e.opts.Pipes != nil {
			pipes = e.opts.Pipes
		}
		for _, p := range pipes(cfg) {
			if resting || (p.Name != "mpv" && p.Name != "IINA") {
				continue
			}
			mu.Lock()
			busy := watching[p.Path]
			mu.Unlock()
			if busy {
				continue
			}
			conn, err := pipe.Dial(p.Path)
			if err != nil {
				continue
			}
			mu.Lock()
			watching[p.Path] = true
			mu.Unlock()
			go func(path string, c io.ReadWriteCloser) {
				stop := context.AfterFunc(r.ctx, func() { c.Close() })
				defer stop()
				_ = mpvipc.Watch(c, e.nudge)
				mu.Lock()
				delete(watching, path)
				mu.Unlock()
			}(p.Path, conn)
		}
		select {
		case <-r.ctx.Done():
			return
		case <-t.C:
		}
	}
}

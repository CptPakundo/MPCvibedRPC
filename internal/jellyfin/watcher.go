package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/remote"
)

// Watcher asks one server's session list what the user plays there, every few seconds. Run it in a goroutine; Now is
// safe to call at any time.
type Watcher struct {
	remote.Base // Problem: why the server cannot be followed (for the window)
	Client      *Client
	Token       string // the sign-in
	UserID      string // whose playback counts (an administrator sees everyone's sessions)
	Product     string // Jellyfin or Emby, for messages
	Every       time.Duration
	Log         remote.LogFunc

	mu    sync.Mutex
	conn  bool // the last request was answered
	play  *playback
	now   remote.Clock
	retry time.Duration // the first wait after a failure (tests shorten it)
}

type playback struct {
	key    string // the session and item
	item   Item
	paused bool
	offset int       // ms, at "at"
	at     time.Time // local time of offset
}

// session is one entry of /Sessions (the parts used).
type session struct {
	ID                  string `json:"Id"`
	UserID              string `json:"UserId"`
	LastPlaybackCheckIn string `json:"LastPlaybackCheckIn"`
	NowPlayingItem      *Item  `json:"NowPlayingItem"`
	PlayState           struct {
		PositionTicks *int64 `json:"PositionTicks"`
		IsPaused      bool   `json:"IsPaused"`
	} `json:"PlayState"`
}

func (w *Watcher) product() string {
	if w.Product != "" {
		return w.Product
	}
	return "Jellyfin"
}

// Now is what the user plays on the server: nil while the server does not answer, an Info with State -1 when nothing
// of theirs plays, else the video.
func (w *Watcher) Now() *core.Info {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.conn {
		return nil
	}
	p := w.play
	if p == nil {
		return core.NewInfo("", "", "", -1, 0, 0, 1)
	}
	pos, state := p.offset, 1
	if !p.paused {
		state = 2
		pos += int(w.now.Now().Sub(p.at) / time.Millisecond) // players report every few seconds; the time runs on between
	}
	dur := p.item.Duration()
	if dur > 0 && pos > dur {
		pos = dur
	}
	return core.NewInfo(p.item.FileName(), "", "", state, pos, dur, 1)
}

// Run asks the server until ctx ends, waiting longer after each failure.
func (w *Watcher) Run(ctx context.Context) {
	every := w.Every
	if every <= 0 {
		every = 5 * time.Second
	}
	first := 5 * time.Second
	if w.retry > 0 {
		first = w.retry
	}
	wait, ok, seen := first, false, false // ok: the last poll worked; seen: one has worked since the start
	for ctx.Err() == nil {
		err := w.poll(ctx)
		if ctx.Err() != nil {
			return
		}
		next := every
		if err != nil {
			w.mu.Lock()
			w.conn, w.play = false, nil
			w.mu.Unlock()
			if errors.Is(err, ErrToken) {
				w.SetProblem(w.product()+" refused the sign-in. Sign in again (Accounts > Jellyfin / Emby).", w.Log)
			} else {
				w.SetProblem(fmt.Sprintf("Could not reach your %s server (%v). Is it running, and can this computer reach it?", w.product(), Reason(err)), w.Log)
			}
			next, ok = wait, false
			if wait < 2*time.Minute {
				wait *= 2
			}
		} else {
			if !ok && !seen {
				w.Log.Print("INFO", "Following your "+w.product()+" server.")
			} else if !ok {
				w.Log.Print("INFO", "Your "+w.product()+" server answers again.")
			}
			seen = true
			w.SetProblem("", w.Log)
			ok, wait = true, first
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(next):
		}
	}
}

// sameID compares two ids the way servers write them (with or without dashes, in any case).
func sameID(a, b string) bool {
	n := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "-", "")) }
	return a != "" && n(a) == n(b)
}

// staleAfter is how long a playing session may go without a report before it no longer counts (Jellyfin keeps the
// session of a player that crashed or lost its network until it times out itself).
const staleAfter = 5 * time.Minute

// poll asks the server once and remembers what the user plays.
func (w *Watcher) poll(ctx context.Context) error {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var list []session
	date, err := w.Client.do(c, "GET", "/Sessions", w.Token, nil, &list)
	if err != nil {
		return err
	}
	now := w.now.Now()
	var best *session
	bestAt := time.Time{}
	for i := range list {
		s := &list[i]
		if s.NowPlayingItem == nil || !s.NowPlayingItem.IsVideo() || !sameID(s.UserID, w.UserID) {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, s.LastPlaybackCheckIn)
		if !s.PlayState.IsPaused && !at.IsZero() && !date.IsZero() && date.Sub(at) > staleAfter {
			continue // a player that went away without saying so: playing players report every few seconds
		}
		switch {
		case best == nil,
			best.PlayState.IsPaused && !s.PlayState.IsPaused,                    // one playing beats one paused
			best.PlayState.IsPaused == s.PlayState.IsPaused && at.After(bestAt): // then the most recent report
			best, bestAt = s, at
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.conn = true
	if best == nil {
		w.play = nil
		return nil
	}
	pos := 0
	if t := best.PlayState.PositionTicks; t != nil {
		pos = int(*t / 10000)
	}
	// The position is the one the player last reported. How long ago that was, by the server's clock, moves it on to
	// now (both times come from the server, so this computer's clock does not matter).
	at := now
	if !bestAt.IsZero() && !date.IsZero() {
		if age := date.Sub(bestAt); age > 0 && age < time.Minute {
			at = now.Add(-age)
		}
	}
	key := best.ID + "/" + best.NowPlayingItem.ID
	if p := w.play; p != nil && p.key == key && p.offset == pos && p.paused == best.PlayState.IsPaused {
		// no new report since the last poll: the time keeps running from when that report came (Emby says nothing
		// about when it was, so without this the time would jump back to the report at every poll)
		at = p.at
	}
	w.play = &playback{key: key, item: *best.NowPlayingItem, paused: best.PlayState.IsPaused, offset: pos, at: at}
	return nil
}

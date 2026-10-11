package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// Watcher asks one server's session list what the user plays there, every few seconds. Run it in a goroutine; Now is
// safe to call at any time.
type Watcher struct {
	Client  *Client
	Token   string // the sign-in
	UserID  string // whose playback counts (an administrator sees everyone's sessions)
	Product string // Jellyfin or Emby, for messages
	Every   time.Duration
	Log     func(level, msg string)

	mu      sync.Mutex
	conn    bool   // the last request was answered
	problem string // why the server cannot be followed (for the window)
	play    *playback
	now     func() time.Time
	retry   time.Duration // the first wait after a failure (tests shorten it)
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

func (w *Watcher) log(level, msg string) {
	if w.Log != nil {
		w.Log(level, msg)
	}
}

func (w *Watcher) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

func (w *Watcher) product() string {
	if w.Product != "" {
		return w.Product
	}
	return "Jellyfin"
}

// Problem says why the server cannot be followed right now ("" when all is well).
func (w *Watcher) Problem() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.problem
}

func (w *Watcher) setProblem(p string) {
	w.mu.Lock()
	changed := p != w.problem
	w.problem = p
	w.mu.Unlock()
	if changed && p != "" {
		w.log("WARN", p)
	}
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
		pos += int(w.clock().Sub(p.at) / time.Millisecond) // players report every few seconds; the time runs on between
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
				w.setProblem(w.product() + " refused the sign-in. Sign in again (Advanced > " + w.product() + ").")
			} else {
				w.setProblem(fmt.Sprintf("Could not reach your %s server (%v). Is it running, and can this computer reach it?", w.product(), Reason(err)))
			}
			next, ok = wait, false
			if wait < 2*time.Minute {
				wait *= 2
			}
		} else {
			if !ok && !seen {
				w.log("INFO", "Following your "+w.product()+" server.")
			} else if !ok {
				w.log("INFO", "Your "+w.product()+" server answers again.")
			}
			seen = true
			w.setProblem("")
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

// poll asks the server once and remembers what the user plays.
func (w *Watcher) poll(ctx context.Context) error {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var list []session
	date, err := w.Client.do(c, "GET", "/Sessions", w.Token, nil, &list)
	if err != nil {
		return err
	}
	now := w.clock()
	var best *session
	bestAt := time.Time{}
	for i := range list {
		s := &list[i]
		if s.NowPlayingItem == nil || !s.NowPlayingItem.IsVideo() || !sameID(s.UserID, w.UserID) {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, s.LastPlaybackCheckIn)
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

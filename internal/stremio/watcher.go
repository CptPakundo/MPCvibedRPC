package stremio

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/remote"
)

// How often Stremio's player updates the library entry of what it plays (stremio-core's PUSH_TO_LIBRARY_EVERY):
// 90 seconds up to core 0.65.0, 180 seconds since 0.65.1, and both are in use while the apps catch up. The regular
// update comes once that long has passed since the last regular one; the extra updates when playback starts, pauses,
// resumes and stops do not move that timer.
const (
	PushShort = 90 * time.Second
	PushLong  = 180 * time.Second
)

// Watcher follows the account's library and knows what plays. The library does not say "paused": an update that
// comes before the next regular one is due, after the video moved on, is a pause or a stop, and the status is taken
// down; an update that finds the video where it was is a start or a resume. When the regular updates stop coming,
// the video stopped (or was paused) without Stremio saying so. Which interval the player uses is learned from its
// updates (see judge). Run it in a goroutine; Now is safe to call at any time.
type Watcher struct {
	remote.Base // Problem: why Stremio cannot be followed (for the window)
	Client      *Client
	Key         string        // the account's key
	Every       time.Duration // how often the library is asked (0 = every 20 seconds)
	Log         remote.LogFunc

	mu     sync.Mutex
	ok     bool             // the library answered at least once
	mtimes map[string]int64 // when each entry last changed, as last seen (ms)
	known  map[string]entry // the entries fetched since the watcher started
	cur    *entry           // what plays (nil = nothing)
	now    remote.Clock

	push     time.Duration // the player's regular interval, once learned (0 = not known yet)
	reg      *entry        // the last update taken for a regular one (or the first seen) of the video that plays
	paused   *entry        // the last update taken for a pause
	lastPoll time.Time     // when the library last answered
	gap      time.Duration // the longest wait between two answers since reg (a long one may hide an update)
}

// entry is one fetched library entry and when its update was seen.
type entry struct {
	item  Item
	mtime int64     // when Stremio says it changed (ms, the clock of the device that played)
	seen  time.Time // when it was seen here
}

func (w *Watcher) every() time.Duration {
	if w.Every > 0 {
		return w.Every
	}
	return 20 * time.Second
}

// interval is the player's regular update interval: the learned one, else the longer (so nothing is given up early).
func (w *Watcher) interval() time.Duration {
	if w.push > 0 {
		return w.push
	}
	return PushLong
}

// stale is how long after its last update a playing video is given up (a regular update is late).
func (w *Watcher) stale() time.Duration { return w.interval() + 2*w.every() + 15*time.Second }

// Now is what the user watches in Stremio: nil while the library has not answered, an Info with State -1 when
// nothing plays, else the video, playing.
func (w *Watcher) Now() *core.Info {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.ok {
		return nil
	}
	now := w.now.Now()
	if w.cur != nil && now.Sub(w.cur.seen) > w.stale() {
		w.cur = nil // the regular updates stopped
	}
	if w.cur == nil {
		return core.NewInfo("", "", "", -1, 0, 0, 1)
	}
	c := w.cur
	// the time runs on since the update: from when the device says it was made, unless its clock is far off ours
	since := now.Sub(c.seen)
	if age := now.UnixMilli() - c.mtime; age >= 0 && time.Duration(age)*time.Millisecond <= PushLong+w.every() {
		since = time.Duration(age) * time.Millisecond
	}
	pos := c.item.State.TimeOffset + since.Milliseconds()
	dur := c.item.State.Duration
	if dur > 0 && pos > dur {
		pos = dur
	}
	return core.NewInfo(c.item.FileName(), "", "", 2, int(pos), int(dur), 1)
}

// Run follows the library until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	wait := w.every()
	for ctx.Err() == nil {
		err := w.poll(ctx)
		if ctx.Err() != nil {
			return
		}
		next := w.every()
		switch {
		case errors.Is(err, ErrKey):
			w.SetProblem("Stremio refused the sign-in. Sign in to Stremio again (Accounts > Stremio).", w.Log)
			next = 2 * time.Minute
		case err != nil:
			w.SetProblem(fmt.Sprintf("Could not reach Stremio's library (%v).", err), w.Log)
			if wait < 2*time.Minute {
				wait *= 2
			}
			next = wait
		default:
			w.SetProblem("", w.Log)
			wait = w.every()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(next):
		}
	}
}

// poll asks which library entries changed, fetches the ones that did and decides what plays.
func (w *Watcher) poll(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	m, err := w.Client.mtimes(cctx, w.Key)
	if err != nil {
		return err
	}
	now := w.now.Now()
	w.mu.Lock()
	first := w.mtimes == nil
	var changed []string
	for id, t := range m {
		if first {
			// at the start, only an entry updated within the last regular update or so can be playing
			if age := now.UnixMilli() - t; age < (PushLong+w.every()).Milliseconds() && age > -time.Minute.Milliseconds() {
				changed = append(changed, id)
			}
		} else if t > w.mtimes[id] {
			changed = append(changed, id)
		}
	}
	w.mu.Unlock()
	sort.Slice(changed, func(i, j int) bool { return m[changed[i]] > m[changed[j]] })
	if len(changed) > 5 {
		changed = changed[:5] // a library sync from another device: the newest ones are enough
	}
	var items []Item
	if len(changed) > 0 {
		if items, err = w.Client.items(cctx, w.Key, changed); err != nil {
			return err
		}
	}
	sort.Slice(items, func(i, j int) bool { return m[items[i].ID] < m[items[j].ID] }) // the newest is taken last
	w.mu.Lock()
	defer w.mu.Unlock()
	// stored only now: a failed fetch leaves the change to be found again by the next poll
	w.mtimes = m
	if w.known == nil {
		w.known = map[string]entry{}
	}
	if !w.lastPoll.IsZero() {
		w.gap = max(w.gap, now.Sub(w.lastPoll))
	}
	w.lastPoll = now
	for _, it := range items {
		w.take(it, m[it.ID], now, first)
	}
	w.ok = true
	return nil
}

// take looks at a changed entry (w.mu held).
func (w *Watcher) take(it Item, mtime int64, now time.Time, first bool) {
	prev, had := w.known[it.ID]
	e := entry{item: it, mtime: mtime, seen: now}
	if first {
		if age := now.UnixMilli() - mtime; age > 0 {
			e.seen = now.Add(-time.Duration(age) * time.Millisecond)
		}
	}
	w.known[it.ID] = e
	if !it.video() {
		return
	}
	if w.judge(prev, had, e) {
		w.cur = &e
	} else if w.cur != nil && w.cur.item.ID == it.ID {
		w.cur = nil
	}
}

// sameVideo is true when two updates are of the same entry and video.
func sameVideo(a, b *entry) bool {
	return a != nil && b != nil && a.item.ID == b.item.ID && a.item.State.VideoID == b.item.State.VideoID
}

// keptPlaying is true when the time moved on from a to b about as much as the clock did (no pause or seek between).
func keptPlaying(a, b *entry) bool {
	dO, dM := b.item.State.TimeOffset-a.item.State.TimeOffset, b.mtime-a.mtime
	return dO >= 30_000 && dO-dM > -10_000 && dO-dM < 10_000
}

// setReg takes e for the last regular update (w.mu held).
func (w *Watcher) setReg(e entry) {
	w.reg, w.gap = &e, 0
}

// judge decides from an entry's update whether its video plays now (w.mu held). An update with the time moved on is a
// regular one when the player's interval has passed since the last regular one, else a pause or a stop. The interval
// is learned: two regular updates about 180 s apart with the video playing all along (and no long wait between two
// answers of the library, which could hide an update) mean 180 s; an update taken for a pause after which the video
// kept playing means the player updates more often, so 90 s. Until 180 s is known, any update 80 s or more after the
// last regular one counts as regular, as for a 90-second player.
func (w *Watcher) judge(prev entry, had bool, e entry) bool {
	o, d := e.item.State.TimeOffset, e.item.State.Duration
	if d > 0 && o >= d-1000 {
		return false // the end
	}
	if !had {
		p := o > 0 || e.item.State.VideoID != "" // a start, or a resume, or a regular update
		if p {
			w.setReg(e)
		}
		return p
	}
	po := prev.item.State.TimeOffset
	if o == 0 && po > 0 && e.item.State.VideoID == prev.item.State.VideoID {
		return false // watched to the end (Stremio rewinds the entry)
	}
	if e.item.State.VideoID != prev.item.State.VideoID {
		w.setReg(e)
		return true // the next episode
	}
	if !sameVideo(w.reg, &e) {
		w.setReg(prev) // another entry played in between: its updates are the best guess
	}
	steady := w.gap <= 45*time.Second
	if w.push == PushLong && steady && sameVideo(w.paused, &e) && w.paused.mtime == prev.mtime && keptPlaying(&prev, &e) {
		w.push = PushShort // the "pause" was a regular update of a 90-second player
		w.setReg(prev)
	}
	due := w.push
	if due == 0 {
		due = PushShort
	}
	g := time.Duration(e.mtime-w.reg.mtime) * time.Millisecond
	if dO := o - po; dO > -3000 && dO < 3000 {
		if g >= due-10*time.Second {
			w.setReg(e) // a resume after the interval ran out: the regular update follows at once
		}
		return true // it is where it was: a start or a resume
	}
	if g >= due-10*time.Second {
		if steady && g >= PushLong-10*time.Second && g <= PushLong+20*time.Second && keptPlaying(w.reg, &e) {
			w.push = PushLong
		}
		w.setReg(e)
		return true // a regular update while it plays
	}
	w.paused = &e
	return false // it moved on and Stremio said so early: paused or stopped
}

// video is true for entries that are watched (live channels and the like count; "other" is not synced anyway).
func (it *Item) video() bool { return it.Type != "other" && it.Name != "" }

// FileName writes the entry the way a well-named file would be called ("Sample Show - S01E02", "Sample Movie"), so the
// same parsing and catalog lookups apply as for a file.
func (it *Item) FileName() string {
	name := core.CleanTitle(it.Name)
	if it.Type != "series" && it.Type != "anime" {
		return name
	}
	// the episode is in the video id: "<item id>:<season>:<episode>" (Cinemeta) or "<item id>:<episode>" (Kitsu)
	rest, ok := strings.CutPrefix(it.State.VideoID, it.ID+":")
	if !ok {
		return name
	}
	var nums []int
	for _, p := range strings.Split(rest, ":") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return name
		}
		nums = append(nums, n)
	}
	switch len(nums) {
	case 2:
		return fmt.Sprintf("%s - S%02dE%02d", name, nums[0], nums[1])
	case 1:
		return fmt.Sprintf("%s - E%02d", name, nums[0])
	}
	return name
}

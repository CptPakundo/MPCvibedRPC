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
)

// PushEvery is how often Stremio's player updates the library entry of what it plays (stremio-core's
// PUSH_TO_LIBRARY_EVERY). It also updates the entry when playback starts, pauses, resumes and stops.
const PushEvery = 90 * time.Second

// Watcher follows the account's library and knows what plays. The library does not say "paused": an update that
// comes between the regular ones after the video moved on is a pause or a stop, and the status is taken down; an
// update that finds the video where it was is a start or a resume. When the regular updates stop coming, the video
// stopped (or was paused) without Stremio saying so. Run it in a goroutine; Now is safe to call at any time.
type Watcher struct {
	Client *Client
	Key    string        // the account's key
	Every  time.Duration // how often the library is asked (0 = every 20 seconds)
	Log    func(level, msg string)

	mu      sync.Mutex
	ok      bool             // the library answered at least once
	problem string           // why Stremio cannot be followed (for the window)
	mtimes  map[string]int64 // when each entry last changed, as last seen (ms)
	known   map[string]entry // the entries fetched since the watcher started
	cur     *entry           // what plays (nil = nothing)
	now     func() time.Time
}

// entry is one fetched library entry and when its update was seen.
type entry struct {
	item  Item
	mtime int64     // when Stremio says it changed (ms, the clock of the device that played)
	seen  time.Time // when it was seen here
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

func (w *Watcher) every() time.Duration {
	if w.Every > 0 {
		return w.Every
	}
	return 20 * time.Second
}

// Problem says why Stremio cannot be followed right now ("" when all is well).
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

// stale is how long after its last update a playing video is given up (a regular update is late).
func (w *Watcher) stale() time.Duration { return PushEvery + 2*w.every() + 15*time.Second }

// Now is what the user watches in Stremio: nil while the library has not answered, an Info with State -1 when
// nothing plays, else the video, playing.
func (w *Watcher) Now() *core.Info {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.ok {
		return nil
	}
	now := w.clock()
	if w.cur != nil && now.Sub(w.cur.seen) > w.stale() {
		w.cur = nil // the regular updates stopped
	}
	if w.cur == nil {
		return core.NewInfo("", "", "", -1, 0, 0, 1)
	}
	c := w.cur
	// the time runs on since the update: from when the device says it was made, unless its clock is far off ours
	since := now.Sub(c.seen)
	if age := now.UnixMilli() - c.mtime; age >= 0 && time.Duration(age)*time.Millisecond <= PushEvery+w.every() {
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
			w.setProblem("Stremio refused the sign-in. Sign in to Stremio again (Accounts > Stremio).")
			next = 2 * time.Minute
		case err != nil:
			w.setProblem(fmt.Sprintf("Could not reach Stremio's library (%v).", err))
			if wait < 2*time.Minute {
				wait *= 2
			}
			next = wait
		default:
			w.setProblem("")
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
	now := w.clock()
	w.mu.Lock()
	first := w.mtimes == nil
	var changed []string
	for id, t := range m {
		if first {
			// at the start, only an entry updated within the last regular update or so can be playing
			if age := now.UnixMilli() - t; age < (PushEvery+w.every()).Milliseconds() && age > -time.Minute.Milliseconds() {
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
	if playing(prev, had, it, mtime) {
		w.cur = &e
	} else if w.cur != nil && w.cur.item.ID == it.ID {
		w.cur = nil
	}
}

// playing decides from an entry's update whether its video plays now.
func playing(prev entry, had bool, it Item, mtime int64) bool {
	o, d := it.State.TimeOffset, it.State.Duration
	if d > 0 && o >= d-1000 {
		return false // the end
	}
	if !had {
		return o > 0 || it.State.VideoID != "" // a start, or a resume, or a regular update
	}
	po := prev.item.State.TimeOffset
	if o == 0 && po > 0 && it.State.VideoID == prev.item.State.VideoID {
		return false // watched to the end (Stremio rewinds the entry)
	}
	if it.State.VideoID != prev.item.State.VideoID {
		return true // the next episode
	}
	dM, dO := mtime-prev.mtime, o-po
	switch {
	case dM >= (PushEvery - 10*time.Second).Milliseconds():
		return true // a regular update while it plays (or a resume after a long pause)
	case dO > -3000 && dO < 3000:
		return true // it is where it was: a start or a resume
	}
	return false // it moved on and Stremio said so early: paused or stopped
}

// video is true for entries that are watched (live channels and the like count; "other" is not synced anyway).
func (it *Item) video() bool { return it.Type != "other" && it.Name != "" }

// clean keeps a title from looking like a folder path to the file-name parser.
func clean(s string) string {
	s = strings.NewReplacer("/", "-", `\`, "-", "\r", " ", "\n", " ").Replace(s)
	return strings.TrimSpace(s)
}

// FileName writes the entry the way a well-named file would be called ("Sample Show - S01E02", "Sample Movie"), so the
// same parsing and catalog lookups apply as for a file.
func (it *Item) FileName() string {
	name := clean(it.Name)
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

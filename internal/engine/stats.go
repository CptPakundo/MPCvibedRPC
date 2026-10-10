package engine

import (
	"encoding/json"
	"os"
	"time"
)

// Stats counts what was shown on Discord. They are numbers only (never titles) and stay on this computer, in
// stats.json in the data folder; nothing is sent anywhere.
type Stats struct {
	Since     string         `json:"since"`     // the day counting began (YYYY-MM-DD)
	Videos    int            `json:"videos"`    // videos shown on Discord (pausing, seeking or replaying one counts once)
	Matched   int            `json:"matched"`   // of those, found in an online catalog (cover art, details)
	WatchedMs int64          `json:"watchedMs"` // time something played while it was shown
	Players   map[string]int `json:"players"`   // videos by player
}

func freshStats(now time.Time) Stats {
	return Stats{Since: now.Format("2006-01-02"), Players: map[string]int{}}
}

func loadStats(file string, now time.Time) Stats {
	s := freshStats(now)
	if file == "" {
		return s
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return s
	}
	var got Stats
	if json.Unmarshal(b, &got) != nil || got.Since == "" {
		return s
	}
	if got.Players == nil {
		got.Players = map[string]int{}
	}
	return got
}

// saveStats writes the counts if they changed (e.mu held).
func (e *Engine) saveStats(now time.Time) {
	if !e.statDirty || e.opts.StatsFile == "" {
		return
	}
	b, err := json.MarshalIndent(e.stats, "", "  ")
	if err != nil {
		return
	}
	tmp := e.opts.StatsFile + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil && os.Rename(tmp, e.opts.StatsFile) == nil {
		e.statDirty, e.statSaved = false, now
	}
}

// statTime adds the time since the last tick to the watched time when the status showed something playing in between
// (e.mu held). A long gap (the computer slept) is not counted.
func (e *Engine) statTime(now time.Time, interval time.Duration) {
	if e.shown && !e.paused && !e.pauseCleared && !e.statTick.IsZero() {
		if d := now.Sub(e.statTick); d > 0 && d <= 2*interval+time.Second {
			e.stats.WatchedMs += d.Milliseconds()
			e.statDirty = true
		}
	}
	e.statTick = now
	if now.Sub(e.statSaved) >= time.Minute {
		e.saveStats(now)
	}
}

// statShown counts a video the first time it is shown, and once that it was found in a catalog (e.mu held). key
// tells videos apart in memory only.
func (e *Engine) statShown(key, player string, found bool) {
	if key != e.statKey {
		e.statKey, e.statFound = key, false
		e.stats.Videos++
		if player == "" {
			player = "MPC-HC"
		}
		e.stats.Players[player]++
		e.statDirty = true
	}
	if found && !e.statFound {
		e.statFound = true
		e.stats.Matched++
		e.statDirty = true
	}
}

// Stats is a copy of the counts for the window.
func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.stats
	s.Players = map[string]int{}
	for k, v := range e.stats.Players {
		s.Players[k] = v
	}
	return s
}

// ResetStats starts counting again from today.
func (e *Engine) ResetStats() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	e.stats, e.statKey, e.statFound, e.statDirty = freshStats(now), "", false, true
	e.saveStats(now)
}

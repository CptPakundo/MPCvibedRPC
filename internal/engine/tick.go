package engine

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/artwork"
	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
)

// appNameFor gives the default app name the playing player's name: the default describes Media Player Classic, other
// players are called what they are and get their own logo as the icon (see core.PlayerIcon). A name the user chose
// is kept.
func appNameFor(cfg core.Config, player string) core.Config {
	def := core.DefaultConfig()
	if cfg.AppName != def.AppName {
		return cfg
	}
	switch player {
	case "MPC-HC", "MPC-BE", "MPC-QT", "":
	case "VLC":
		cfg.AppName = "VLC media player"
	default:
		cfg.AppName = player
	}
	if cfg.AppName != def.AppName && cfg.SmallImageKey == def.SmallImageKey {
		cfg.SmallImageKey = core.PlayerIcon(player) // the default corner icon is Media Player Classic's logo: others get their own, or none
	}
	if icon := core.PlayerIcon(player); icon != "" && cfg.LargeImageKey == def.LargeImageKey {
		cfg.LargeImageKey = icon // without a cover the large image is the player's logo, not Media Player Classic's
	}
	return cfg
}

// built is an activity ready to send and what the log, the window and the stats need to know about it.
type built struct {
	activity   *core.Activity
	display    string
	found      *core.Art
	music      bool
	noEpTitle  bool     // an episode whose title was wanted but did not come
	fromFolder bool     // the show's name came from the file's folder
	why        []string // "Why this title?" (see Status.Why)
}

// buildActivity makes the activity for what plays: titles are parsed and looked up (art may be nil) unless the title
// is hidden; songs are shown as they are named. It touches no engine state.
func buildActivity(info *core.Info, cfg *core.Config, art *artwork.Artwork) built {
	b := built{music: core.IsAudioFile(info.File)}
	switch {
	case cfg.HideTitle:
		// nothing about the file is used: no parsing, no lookups, and no title in the status, the window or the log
		b.activity = core.HiddenActivity(info, cfg, time.Now().UnixMilli())
		b.display = "A video (title hidden)"
		if b.music && b.activity != nil {
			core.Listening(b.activity)
			b.activity.Details, b.display = "Listening to music", "Music (title hidden)"
		}
	case b.music:
		// a song: shown as it is named, as "Listening to"; never sent to the movie and TV catalogs
		media := core.MusicMedia(info.File)
		b.activity = core.BuildActivity(info, cfg, time.Now().UnixMilli(), media, nil)
		core.Listening(b.activity)
		b.display = media.Display
	default:
		media := core.ParseMedia(info.File, cfg, info.Dir)
		if info.Duration > 0 {
			media.DurationMin = int(math.Floor(float64(info.Duration)/60000 + 0.5))
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
			b.found = art.Lookup(media)
		}
		b.activity = core.BuildActivity(info, cfg, time.Now().UnixMilli(), media, b.found)
		core.AddLinks(b.activity, cfg, b.found)
		b.noEpTitle = cfg.ShowArtwork && cfg.EpisodeTitles && media.IsEpisode && media.Episode != core.NA && media.EpTitle == "" &&
			(b.found == nil || b.found.Episode == nil)
		b.display = media.Display
		b.fromFolder = media.FromFolder
		b.why = append(readAs(info, media), art.Explain(media, b.found)...)
	}
	return b
}

func (e *Engine) tick(r *run) {
	if e.rest(r) {
		return
	}
	e.mu.Lock()
	cfg := e.cfg
	e.statTime(time.Now(), pollInterval(cfg))
	e.mu.Unlock()

	info := e.fetchPlayer(r.ctx, cfg)
	if !e.isCurrent(r) {
		return
	}
	if info != nil {
		cfg = appNameFor(cfg, info.Player)
	}

	e.mu.Lock()
	player := ""
	if info != nil {
		player = info.Player
	}
	if player != e.player {
		e.player = player
		e.mu.Unlock()
		if player != "" {
			e.log("INFO", player+" detected.")
		} else {
			e.log("INFO", "No player reachable (closed, or its web interface or IPC is off).")
		}
		e.mu.Lock()
	}
	ended := endedAt(info)
	active := info != nil && info.File != "" && (info.State == 1 || info.State == 2) && !ended
	hidden := active && (core.MatchesHideList(info.File, info.FullDir, cfg.HideFiles) || hiddenWhere(info.Where, cfg.HideFiles))

	if !active || hidden {
		// Clear what is shown; and while nothing plays, clear again now and then (reclearEvery), so a clear Discord
		// missed, or a status it applied although its answer never came, cannot stay up.
		var clear *discord.Client
		shown := e.shown
		if e.ready && (shown || time.Since(e.clearedAt) >= reclearEvery) {
			clear = e.rpc
		}
		e.mu.Unlock()
		failed := false
		if clear != nil {
			err := clear.ClearActivity()
			e.mu.Lock()
			failed = err != nil
			first := failed && !e.clearStuck
			e.clearStuck = failed
			if !failed {
				e.clearedAt = time.Now()
			}
			e.mu.Unlock()
			switch {
			case first:
				e.log("WARN", "Discord did not take the clearing of the status ("+err.Error()+"). Trying again at the next check.")
			case failed || !shown:
			case hidden:
				e.log("INFO", "This file is on your don't-show list - presence cleared.")
			case ended:
				e.log("INFO", "The video ended - presence cleared.")
			default:
				e.log("INFO", "Nothing playing - presence cleared.")
			}
		}
		e.mu.Lock()
		e.hidden, e.paused = hidden, false
		e.shown = shown && failed // a clear Discord did not take is tried again at the next check
		e.prev, e.nowPlaying, e.preview = nil, nil, nil
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
			if err := clear.ClearActivity(); err != nil {
				e.mu.Lock()
				e.pauseCleared, e.shown = false, true // not cleared: the next check tries again
				e.mu.Unlock()
				e.log("WARN", "Discord did not take the clearing of the status ("+err.Error()+"). Trying again at the next check.")
				return
			}
			e.mu.Lock()
			e.clearedAt = time.Now()
			e.mu.Unlock()
			e.log("INFO", fmt.Sprintf("Paused for %d min - presence cleared (it returns when you resume).", cfg.PauseClearMinutes))
		}
		return
	}
	art := e.art
	e.mu.Unlock()
	if !update {
		return
	}

	b := buildActivity(info, &cfg, art)
	if !e.isCurrent(r) {
		return
	}
	sent, err := e.sendActivity(rpc, &cfg, b.activity)
	if err != nil {
		msg := err.Error()
		e.mu.Lock()
		e.lastError = &msg
		e.prev = nil
		e.shown = true // Discord may have applied it although its answer did not come: it must be cleared later
		e.mu.Unlock()
		e.log("ERROR", "Failed to update presence: "+msg)
		return
	}
	e.mu.Lock()
	e.shown = true
	e.lastError = nil
	d := b.display
	if !b.music {
		e.statShown(info.Player+"/"+info.File, info.Player, b.found != nil) // player names have no slash; songs are not videos
	}
	if key := info.Player + "/" + info.File; b.noEpTitle {
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
	e.why = b.why
	e.paused = info.State == 1
	e.preview = core.PreviewOf(sent, &cfg) // what Discord actually got (the basic fallback carries less)
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
	if b.found != nil {
		tag = " [artwork]"
	}
	// The name the player gave is logged too (never the folder path): a wrong title can only be traced back to it.
	src := info.Player + `: "` + info.File + `"`
	if b.fromFolder {
		src += ", show name from its folder"
	}
	e.log("INFO", fmt.Sprintf("%s: %s%s (%s)", word, b.display, tag, src))
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
		case <-e.wake:
			t.Stop()
		}
	}
}

// pollInterval is how often the players are asked (at least every 250 ms).
func pollInterval(cfg core.Config) time.Duration {
	interval := time.Duration(cfg.PollInterval) * time.Millisecond
	if interval < 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	return interval
}

// reclearEvery is how often the status is cleared again while nothing plays and Discord is connected (shortened in
// tests): Discord limits a status to five changes in 20 seconds, and once a minute keeps far below that.
var reclearEvery = time.Minute

// endSlack: a player paused this close to the end of the file has finished it (some stay paused on the last frame).
const endSlack = 3000 // ms

// endedAt reports whether the player is paused at the end of its file: that is a finished video, not a pause.
func endedAt(info *core.Info) bool {
	return info != nil && info.State == 1 && info.Duration > 0 && info.Position >= info.Duration-endSlack
}

// recheckAfter is when an episode shown without its title is looked up again, in minutes after it was shown (or after
// the previous try): a catalog that did not answer gets another chance while the episode plays.
var recheckAfter = []float64{2.5, 5, 15}

// hiddenWhere reports whether the Never show list matches where a server keeps the item (see core.Info.Where).
func hiddenWhere(where, entries []string) bool {
	for _, w := range where {
		if core.MatchesHideList(w, "", entries) {
			return true
		}
	}
	return false
}

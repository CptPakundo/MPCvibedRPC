package engine

import (
	"reflect"
	"time"

	"github.com/CptPakundo/MPRP/internal/core"
	"github.com/CptPakundo/MPRP/internal/discord"
)

// reach is what the running engine redoes for a changed setting: the `reach` tag of each core.Config field.
type reach uint8

const (
	reachCheck   reach = 1 << iota // the next check reads it: the status is only sent again
	reachLookups                   // the cover and title finder is made again
	reachPlayers                   // the players are looked for again (event watches, Plex, Stremio, Jellyfin and Emby)
	reachDiscord                   // the connection to Discord is made again
)

// reachNames are the tag's values; "none" (the engine does not use the setting) is no bit.
var reachNames = map[string]reach{"none": 0, "check": reachCheck, "lookups": reachLookups, "players": reachPlayers, "discord": reachDiscord}

// settingReach is each setting's reach, in the order of core.ConfigFields. A field without a known tag counts as
// everything (TestEverySettingHasAReach keeps that from happening).
var settingReach = func() []reach {
	fields := core.ConfigFields()
	out := make([]reach, len(fields))
	for i, f := range fields {
		r, ok := reachNames[f.Tag.Get("reach")]
		if !ok {
			r = reachCheck | reachLookups | reachPlayers | reachDiscord
		}
		out[i] = r
	}
	return out
}()

// reachOf is what a change from old to cfg needs, and whether anything changed at all.
func reachOf(old, cfg core.Config) (need reach, changed bool) {
	a, b := reflect.ValueOf(old), reflect.ValueOf(cfg)
	for i, f := range core.ConfigFields() {
		if !reflect.DeepEqual(a.FieldByIndex(f.Index).Interface(), b.FieldByIndex(f.Index).Interface()) {
			need |= settingReach[i]
			changed = true
		}
	}
	return need, changed
}

// ApplySettings gives the engine new settings. A running engine redoes only what the changed settings reach (see
// core.Config): the run goes on, so the status does not blink, the pause clock and a status cleared after a long pause
// stay as they are, and Discord's connection stays open unless the Discord application changed. Settings that did not
// change (a save of an app-only option such as "Start with Windows") change nothing.
func (e *Engine) ApplySettings(cfg core.Config) {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	e.mu.Lock()
	old := e.cfg
	need, changed := reachOf(old, cfg)
	if !changed {
		e.mu.Unlock()
		return
	}
	e.cfg = cfg
	r := e.cur
	if r == nil || need == 0 {
		e.mu.Unlock()
		return
	}
	e.cfgGen++ // a check under way with the old settings sends nothing (the next one, asked for below, does)
	if need&reachLookups != 0 {
		// lookups still under way finish quietly: no title in the log, no refresh (hideTitle may be on now)
		e.art.Close()
		e.art = e.newArtwork(cfg)
	}
	if need&reachPlayers != 0 {
		if e.stopRemote != nil {
			e.stopRemote()
		}
		e.remotes, e.stopRemote = nil, nil // the next check starts them with the new sign-ins (unless it rests)
		e.vlcProblem, e.kodiProblem = "", ""
		e.startPlayerWatch(r)
	}
	if (old.StreamTitle == "auto") != (cfg.StreamTitle == "auto") {
		e.startShareWatch(r, cfg)
	}
	var drop *discord.Client
	if need&reachDiscord != 0 {
		// The next check connects with the new settings; an attempt still under way for the old ones is let go.
		drop, e.discGen = e.rpc, e.discGen+1
		e.rpc, e.ready, e.connecting, e.shown, e.preview = nil, false, false, false, nil
		e.warnedDisc, e.discRefused = false, false
		e.modeKey, e.basicMode, e.noLinks = "", false, false
		e.sendFails, e.retryAt = 0, time.Time{}
	}
	e.resend = true // what plays goes out again with the new settings (a status cleared after a long pause stays cleared)
	e.mu.Unlock()

	if need&reachPlayers != 0 {
		e.log("INFO", "Settings changed. Looking for a player on "+lookingOn(cfg)+".")
	}
	if drop != nil {
		e.log("INFO", "Settings changed. Connecting to Discord again.")
		Go(e.log, "Closing the old Discord connection", func() { closeDiscord(e.log, drop, true) })
	}
	e.Wake()
}

// closeDiscord clears the status on c (when clear is set) and closes the connection, waiting at most 1.5 s for each:
// closing a pipe that is stuck in a write (Discord stopped reading) can block in Windows until the write ends, and
// stopping, quitting or a settings change must not wait for that.
func closeDiscord(log func(level, msg string), c *discord.Client, clear bool) {
	if clear {
		done := make(chan struct{})
		go func() { _ = c.ClearActivity(); close(done) }()
		select {
		case <-done:
		case <-time.After(1500 * time.Millisecond):
		}
	}
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(1500 * time.Millisecond):
		log("WARN", "Discord did not let go of the connection; leaving it behind.")
	}
}

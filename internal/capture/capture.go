// Package capture tells whether the Discord desktop app is sharing the screen right now.
//
// Discord names a stream after the streamer's Playing activity, so the engine sends that form of the status while a
// stream is on (core.Config.StreamTitle "auto"). Discord itself tells a local program nothing about a stream: its
// SCREENSHARE_STATE_UPDATE event needs a permission Discord grants to approved applications only. Windows does: it
// keeps, for every program, when it last started and stopped capturing the screen (Settings > Privacy & security >
// Screenshots and screen recording shows the same record), and a stop time of zero means "still capturing".
//
// The record is read, never written. It exists on Windows 11 and on Windows 10 from the version that knows the
// capture permission; where it is missing, or on another system, nothing is reported and the status keeps its
// usual form.
package capture

import (
	"strings"
	"time"
)

// entry is Windows' capture record for one program file.
type entry struct {
	path  string    // the program file
	start time.Time // when it last began to capture (zero: never)
	stop  time.Time // when that ended; zero while it goes on
}

// minLive is how long a capture must have run before it counts. Discord's "share your screen" picker captures every
// window for a second or two to draw its previews; that is not a stream, and the status must not flip for it.
const minLive = 3 * time.Second

// discordNames are the program files of the Discord desktop app's editions.
var discordNames = map[string]bool{"discord.exe": true, "discordptb.exe": true, "discordcanary.exe": true, "discorddevelopment.exe": true}

// isDiscord reports whether path is a Discord program file.
func isDiscord(path string) bool {
	return discordNames[strings.ToLower(path[strings.LastIndexAny(path, `\/`)+1:])]
}

// sharing decides from the records: a Discord program that began to capture at least minLive ago and has not
// stopped. Discord keeps one folder per version and removes the old ones, and a capture that never ended properly
// (Discord was ended, or updated itself, in the middle of a stream) leaves a record without a stop time behind: a
// record whose program file is gone is such a leftover, so exists is asked.
func sharing(entries []entry, now time.Time, exists func(path string) bool) bool {
	for _, e := range entries {
		if !isDiscord(e.path) || e.start.IsZero() || !e.stop.IsZero() {
			continue
		}
		if now.Sub(e.start) >= minLive && exists(e.path) {
			return true
		}
	}
	return false
}

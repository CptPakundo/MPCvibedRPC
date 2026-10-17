// Package potplayer reads PotPlayer (Windows) through the window messages its control header documents
// (InternalSimpleCmd.h): play state, position, length, speed, and the playing file's path. Read-only: nothing is sent
// that changes the player. Elsewhere Query finds nothing.
package potplayer

import (
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// The messages, sent as WM_USER with one of these in wParam. The answers come back as the result, except the file
// path, which PotPlayer sends to the window named in lParam as WM_COPYDATA (UTF-8, dwData = getFileName).
const (
	getTotalTime   = 0x5002 // ms
	getCurrentTime = 0x5004 // ms
	getPlayStatus  = 0x5006 // 0 stopped, 1 paused, 2 playing
	getSpeed       = 0x5015 // 200..12000 (0.2x..12x); 0 from versions without it
	getFileName    = 0x6020
)

// info builds the Info of one PotPlayer window from its answers. path is the file (or address) PotPlayer plays, title
// the window title ("<file> - PotPlayer"), used when the path did not come.
func info(status, pos, dur, speed int, path, title string) *core.Info {
	state := -1
	switch status {
	case 0:
		state = 0
	case 1:
		state = 1
	case 2:
		state = 2
	}
	rate := 1.0
	if speed >= 200 && speed <= 12000 {
		rate = float64(speed) / 1000
	}
	if dur < 0 {
		dur = 0
	}
	if pos < 0 {
		pos = 0
	}
	path = strings.TrimSpace(path)
	if path == "" {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(title), "- PotPlayer"))
		if name == "" || name == "PotPlayer" {
			return core.NewInfo("", "", "", state, pos, dur, rate)
		}
		return core.NewInfo(name, "", "", state, pos, dur, rate)
	}
	if strings.Contains(path, "://") { // a stream: named by the last part of its address, no folder
		u := strings.SplitN(path, "?", 2)[0]
		return core.NewInfo(u[strings.LastIndex(u, "/")+1:], "", "", state, pos, dur, rate)
	}
	file := path
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		file = path[i+1:]
	}
	return core.NewInfo(file, path, "", state, pos, dur, rate)
}

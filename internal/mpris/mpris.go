// Package mpris reads what video players are playing through MPRIS, the D-Bus interface most Linux players offer
// (VLC, Celluloid, Haruna, SMPlayer, GNOME's players, mpv with the mpv-mpris script, ...). Nothing has to be set up in
// the player. Only known video players are asked: music players and web browsers offer MPRIS too, and must not show
// up as "Watching".
package mpris

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
)

const (
	prefix      = "org.mpris.MediaPlayer2."
	objectPath  = "/org/mpris/MediaPlayer2"
	playerIface = "org.mpris.MediaPlayer2.Player"
	propsIface  = "org.freedesktop.DBus.Properties"
)

// videoPlayers maps a part of a player's bus name to the name shown for it. A bus name matches when one of its
// dot-separated parts (after the MPRIS prefix, without an ".instance123" suffix) is a key here, so both
// "org.mpris.MediaPlayer2.vlc" and "org.mpris.MediaPlayer2.io.github.celluloid_player.Celluloid" are found.
var videoPlayers = map[string]string{
	"vlc":          "VLC",
	"mpv":          "mpv",
	"celluloid":    "Celluloid",
	"haruna":       "Haruna",
	"smplayer":     "SMPlayer",
	"totem":        "GNOME Videos",
	"showtime":     "Showtime",
	"clapper":      "Clapper",
	"dragonplayer": "Dragon Player",
	"mpc-qt":       "MPC-QT",
	"mpcqt":        "MPC-QT",
	"kodi":         "Kodi",
	"parole":       "Parole",
	"xplayer":      "Xplayer",
	"qmplay2":      "QMPlay2",
}

// PlayerName returns the display name of a video player's bus name, or "" when the name is not a known video player.
func PlayerName(busName string) string {
	if !strings.HasPrefix(busName, prefix) {
		return ""
	}
	parts := strings.Split(strings.ToLower(busName[len(prefix):]), ".")
	if n := len(parts); n > 1 && strings.HasPrefix(parts[n-1], "instance") {
		parts = parts[:n-1]
	}
	for _, p := range parts {
		if name, ok := videoPlayers[p]; ok {
			return name
		}
	}
	return ""
}

// Query asks every video player on the bus at address what it plays, in bus-name order. Players that do not answer
// are left out. timeout bounds each call.
func Query(address string, timeout time.Duration) []*core.Info {
	if address == "" {
		return nil
	}
	c, err := dbus.Dial(address, timeout)
	if err != nil {
		return nil
	}
	defer c.Close()
	got, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "ListNames", timeout, "")
	if err != nil || len(got) == 0 {
		return nil
	}
	all, _ := got[0].([]any)
	var names []string
	for _, n := range all {
		if s, ok := n.(string); ok && PlayerName(s) != "" {
			names = append(names, s)
		}
	}
	sort.Strings(names)
	var out []*core.Info
	for _, n := range names {
		r, err := c.Call(n, objectPath, propsIface, "GetAll", timeout, "s", playerIface)
		if err != nil || len(r) == 0 {
			continue
		}
		props, _ := r[0].(map[string]any)
		if props == nil {
			continue
		}
		in := ToInfo(props)
		in.Player = PlayerName(n)
		out = append(out, in)
	}
	return out
}

// ToInfo turns the Player interface's properties into the Info the other players give.
func ToInfo(props map[string]any) *core.Info {
	meta, _ := props["Metadata"].(map[string]any)
	status, _ := props["PlaybackStatus"].(string)
	link, _ := meta["xesam:url"].(string)
	title, _ := meta["xesam:title"].(string)
	if status == "Stopped" || status == "" || (link == "" && title == "") {
		return core.NewInfo("", "", "", -1, 0, 0, 1)
	}
	state := 2
	if status == "Paused" {
		state = 1
	}
	file, filePath, dir := title, "", ""
	if u, err := url.Parse(link); err == nil && u.Scheme == "file" && u.Path != "" {
		filePath = u.Path
		i := strings.LastIndex(filePath, "/")
		dir, file = filePath[:i], filePath[i+1:]
	} else if file == "" {
		file = link // a stream without a title: its address is the best name there is
	}
	pos, _ := number(props["Position"])
	dur, _ := number(meta["mpris:length"])
	rate, ok := number(props["Rate"])
	if !ok || !(rate > 0) {
		rate = 1
	}
	return core.NewInfo(file, filePath, dir, state, int(pos/1000), int(dur/1000), rate)
}

// number reads any D-Bus number (players disagree on the types of Position and mpris:length).
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case uint64:
		return float64(x), true
	case int32:
		return float64(x), true
	case uint32:
		return float64(x), true
	case int16:
		return float64(x), true
	case uint16:
		return float64(x), true
	case byte:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

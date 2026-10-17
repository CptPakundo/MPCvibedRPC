package mpris

import (
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
)

// changeProps are the player properties whose change is news: playing or paused, another file, another speed.
var changeProps = []string{"PlaybackStatus", "Metadata", "Rate"}

// Watch listens on c (a connection to the session bus) for MPRIS players that play, pause, stop, seek or change
// file, and calls onChange for each, until the connection ends; it closes c. Any player's signals count (the checks
// only read the video players), and nothing of what they carry is kept.
func Watch(c *dbus.Conn, timeout time.Duration, onChange func()) error {
	defer c.Close()
	return c.Watch("type='signal',path='"+objectPath+"'", timeout, func(m *dbus.Message) {
		if changed(m) {
			onChange()
		}
	})
}

// changed reports whether a signal from a player's object says that what it shows has changed.
func changed(m *dbus.Message) bool {
	switch {
	case m.Interface == playerIface && m.Member == "Seeked":
		return true
	case m.Interface == propsIface && m.Member == "PropertiesChanged" && len(m.Body) >= 2:
		if iface, _ := m.Body[0].(string); iface != playerIface {
			return false
		}
		props, _ := m.Body[1].(map[string]any)
		for _, p := range changeProps {
			if _, ok := props[p]; ok {
				return true
			}
		}
		if len(m.Body) >= 3 { // properties that changed without their value
			names, _ := m.Body[2].([]any)
			for _, n := range names {
				for _, p := range changeProps {
					if n == p {
						return true
					}
				}
			}
		}
	}
	return false
}

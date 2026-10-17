package kodi

import (
	"encoding/json"
	"io"
	"net"
	"net/url"
)

// EventPort is the port of Kodi's JSON-RPC over TCP, which sends notifications (Kodi's default; the web server
// itself has none). Kodi answers on it when "Allow remote control from applications on this system" is on
// (the default), or "on other systems" for a Kodi elsewhere.
const EventPort = "9090"

// EventAddress is where Kodi at address (as in the settings) sends its notifications: the same host on EventPort.
func EventAddress(address string) string {
	u, err := url.Parse(Base(address))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return net.JoinHostPort(u.Hostname(), EventPort)
}

// watchMethods are the notifications after which what Kodi shows has changed.
var watchMethods = map[string]bool{
	"Player.OnPlay": true, "Player.OnPause": true, "Player.OnResume": true, "Player.OnStop": true,
	"Player.OnSeek": true, "Player.OnSpeedChanged": true, "Player.OnAVStart": true,
}

// Watch reads Kodi's notifications from conn and calls onChange when it plays, pauses, resumes, stops, seeks or
// changes speed, until conn closes. Nothing else is sent or kept: what plays is still read with Query.
func Watch(conn io.ReadCloser, onChange func()) error {
	defer conn.Close()
	dec := json.NewDecoder(conn) // notifications follow each other without separators
	for {
		var n struct {
			Method string `json:"method"`
		}
		if err := dec.Decode(&n); err != nil {
			return err
		}
		if watchMethods[n.Method] {
			onChange()
		}
	}
}

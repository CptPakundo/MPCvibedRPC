package kodi

import (
	"encoding/json"
	"errors"
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

// maxNotification bounds what the decoder may hold of one notification that is not complete yet.
const maxNotification = 1 << 20

// limitedReader fails a read once the decoder holds more than maxNotification bytes it has not used: a notification
// that never ends would otherwise be buffered without limit. Bytes of a finished message count no more.
type limitedReader struct {
	r    io.Reader
	dec  *json.Decoder
	read int64
}

var errTooLarge = errors.New("Kodi sent a notification that is too large.")

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.read-l.dec.InputOffset() > maxNotification {
		return 0, errTooLarge
	}
	n, err := l.r.Read(p)
	l.read += int64(n)
	return n, err
}

// Watch reads Kodi's notifications from conn and calls onChange when it plays, pauses, resumes, stops, seeks or
// changes speed, until conn closes. Nothing else is sent or kept: what plays is still read with Query.
func Watch(conn io.ReadCloser, onChange func()) error {
	defer conn.Close()
	lim := &limitedReader{r: conn}
	dec := json.NewDecoder(lim) // notifications follow each other without separators
	lim.dec = dec
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

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
	"github.com/CptPakundo/MPCvibedRPC/internal/jellyfin"
	"github.com/CptPakundo/MPCvibedRPC/internal/kodi"
	"github.com/CptPakundo/MPCvibedRPC/internal/mpris"
	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
)

// TestLivePlayer runs against a real player on this computer (MPC-QT, mpv, VLC, IINA or a Linux player that offers
// MPRIS, set up as their docs say) and the test's own fake Discord. It only runs on request:
//
//	MPCRPC_LIVE_PLAYER=mpv MPCRPC_LIVE_TITLE="Sample Movie" go test -run TestLivePlayer ./internal/engine
//
// MPCRPC_LIVE_PORT optionally names a web-interface port to ask first (default: none). For VLC, set
// MPCRPC_LIVE_VLC_PASSWORD (and MPCRPC_LIVE_VLC_PORT when it is not 8080). MPCRPC_LIVE_MPV_PIPE and
// MPCRPC_LIVE_IINA_PIPE replace the default connection names of mpv and IINA. For Plex, MPCRPC_LIVE_PLEX_ADDRESS is the
// server (followed without plex.tv; MPCRPC_LIVE_PLEX_TOKEN is its token, if it needs one). For Jellyfin or
// Emby, MPCRPC_LIVE_JELLYFIN_ADDRESS, MPCRPC_LIVE_JELLYFIN_USER and MPCRPC_LIVE_JELLYFIN_PASSWORD sign in to the server. For Kodi, MPCRPC_LIVE_KODI_PASSWORD
// (and MPCRPC_LIVE_KODI_ADDRESS when it is not 127.0.0.1:8080; the user is kodi). PotPlayer needs nothing. MPCRPC_LIVE_PLAYER is the
// name the player is shown under: MPC-QT, mpv, VLC, IINA, PotPlayer, Kodi, Celluloid, Plex, ...
func TestLivePlayer(t *testing.T) {
	want := os.Getenv("MPCRPC_LIVE_PLAYER")
	if want == "" {
		t.Skip("set MPCRPC_LIVE_PLAYER to run against a real player")
	}
	port := deadPort(t)
	if p := os.Getenv("MPCRPC_LIVE_PORT"); p != "" {
		port = atoiOr(p, port)
	}
	e, d, l := pipeEngine(t, port, nil)
	e.opts.Pipes, e.opts.MPRIS, e.opts.PotPlayer, e.opts.KodiEvents = nil, nil, nil, "" // the real ones: MPC-QT's, mpv's, IINA's (macOS), MPRIS (Linux), PotPlayer (Windows)
	if p := os.Getenv("MPCRPC_LIVE_MPV_PIPE"); p != "" {
		e.cfg.MpvPipe = p
	}
	if p := os.Getenv("MPCRPC_LIVE_IINA_PIPE"); p != "" {
		e.cfg.IinaPipe = p
	}
	if a := os.Getenv("MPCRPC_LIVE_PLEX_ADDRESS"); a != "" {
		e.cfg.PlexAddress, e.cfg.PlexToken = a, os.Getenv("MPCRPC_LIVE_PLEX_TOKEN")
		if e.cfg.PlexToken == "" {
			e.cfg.PlexToken = "none" // a server that lets this computer in without signing in ignores it
		}
	}
	if a := os.Getenv("MPCRPC_LIVE_JELLYFIN_ADDRESS"); a != "" {
		c := &jellyfin.Client{Base: a, DeviceID: "ci-engine", Version: "0.0.0"}
		srv, err := c.Find(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		auth, err := c.SignIn(context.Background(), os.Getenv("MPCRPC_LIVE_JELLYFIN_USER"), os.Getenv("MPCRPC_LIVE_JELLYFIN_PASSWORD"))
		if err != nil {
			t.Fatal(err)
		}
		e.cfg.JellyfinAddress, e.cfg.JellyfinToken, e.cfg.JellyfinUser, e.cfg.JellyfinProduct, e.cfg.JellyfinDevice = c.Base, auth.Token, auth.UserID, srv.Product, c.DeviceID
	}
	if pw := os.Getenv("MPCRPC_LIVE_KODI_PASSWORD"); pw != "" {
		e.cfg.KodiPassword = pw
		if a := os.Getenv("MPCRPC_LIVE_KODI_ADDRESS"); a != "" {
			e.cfg.KodiAddress = a
		}
	}
	if pw := os.Getenv("MPCRPC_LIVE_VLC_PASSWORD"); pw != "" {
		e.cfg.VlcPassword = pw
		if p := os.Getenv("MPCRPC_LIVE_VLC_PORT"); p != "" {
			e.cfg.VlcPort = atoiOr(p, e.cfg.VlcPort)
		}
	}
	e.Start()
	eventually(t, want+" detected", func() bool { return e.Status().Player == want })
	eventually(t, "activity sent", func() bool { return len(d.list()) > 0 && !isClear(d) })
	time.Sleep(300 * time.Millisecond)
	s := e.Status()
	act := d.list()[len(d.list())-1]["activity"].(map[string]any)
	t.Logf("status: player=%s playing=%v paused=%v", s.Player, *s.NowPlaying, s.Paused)
	t.Logf("activity: %v", act)
	if title := os.Getenv("MPCRPC_LIVE_TITLE"); title != "" && !strings.Contains(*s.NowPlaying, title) {
		t.Errorf("expected %q in %q", title, *s.NowPlaying)
	}
	if !l.has(want + " detected.") {
		t.Error("log should name the player")
	}
	l.mu.Lock()
	for _, line := range l.l {
		if strings.Contains(line, "Playing: ") || strings.Contains(line, "Paused: ") {
			t.Logf("log: %s", line)
		}
	}
	l.mu.Unlock()
	if !l.has(`(` + want + `: "`) {
		t.Error("the log should name what the player played")
	}
	if how := os.Getenv("MPCRPC_LIVE_EVENTS"); how != "" {
		livePauseEvent(t, e, d, want, how)
	}
}

// livePauseEvent pauses the real player and wants the pause shown at once, from the player's own event, while the
// regular checks are a minute apart. how says how the player is paused: "pipe" (mpv, IINA: their IPC), "kodi" (its
// JSON-RPC, as user kodi with MPCRPC_LIVE_KODI_PASSWORD) or "mpris" (D-Bus). It changes the player's state, so it is
// opt-in (MPCRPC_LIVE_EVENTS; CI sets it for its throwaway players), and it resumes the player afterwards.
func livePauseEvent(t *testing.T, e *Engine, d *disc, name, how string) {
	e.Stop()
	e.cfg.PollInterval = 60000
	e.Start()
	eventually(t, "connected again", func() bool { return e.Status().Discord == "connected" })
	time.Sleep(time.Second) // the event connection is made at the start
	set := livePauser(t, e.cfg, name, how)
	n := len(d.list())
	start := time.Now()
	set(true)
	defer set(false)
	deadline := time.Now().Add(nudgeGap + time.Second) // an event just before can make it wait for the gap
	for !(len(d.list()) > n && e.Status().Paused) {
		if time.Now().After(deadline) {
			t.Fatalf("the pause did not show within %v (regular checks are a minute apart)", nudgeGap+time.Second)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("the pause showed after %v (from %s's event)", time.Since(start).Round(time.Millisecond), name)
}

func atoiOr(s string, def int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// livePauser returns a function that pauses (true) or resumes (false) the real player, the way how names.
func livePauser(t *testing.T, cfg core.Config, name, how string) func(paused bool) {
	switch how {
	case "pipe":
		var path string
		for _, p := range PlayerPipes(cfg) {
			if p.Name == name {
				path = p.Path
			}
		}
		return func(paused bool) {
			c, err := pipe.Dial(path)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			b, _ := json.Marshal(map[string]any{"command": []any{"set_property", "pause", paused}})
			if _, err := c.Write(append(b, '\n')); err != nil {
				t.Fatal(err)
			}
			time.Sleep(100 * time.Millisecond)
		}
	case "kodi":
		return func(paused bool) {
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "Player.PlayPause", "params": map[string]any{"playerid": 1, "play": !paused}})
			req, err := http.NewRequest("POST", kodi.Base(cfg.KodiAddress)+"/jsonrpc", bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			req.SetBasicAuth("kodi", cfg.KodiPassword)
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
		}
	case "mpris":
		return func(paused bool) {
			c, err := dbus.Dial(dbus.SessionAddress(), 2*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			got, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "ListNames", 2*time.Second, "")
			if err != nil || len(got) == 0 {
				t.Fatal("no bus names: ", err)
			}
			names, _ := got[0].([]any)
			member := "Play"
			if paused {
				member = "Pause"
			}
			for _, n := range names {
				if s, _ := n.(string); mpris.PlayerName(s) == name {
					if _, err := c.Call(s, "/org/mpris/MediaPlayer2", "org.mpris.MediaPlayer2.Player", member, 2*time.Second, ""); err != nil {
						t.Fatal(err)
					}
					return
				}
			}
			t.Fatalf("%s is not on the bus", name)
		}
	}
	t.Fatalf("MPCRPC_LIVE_EVENTS=%q: want pipe, kodi or mpris", how)
	return nil
}

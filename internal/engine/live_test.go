package engine

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/jellyfin"
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
// Emby, MPCRPC_LIVE_JELLYFIN_ADDRESS, MPCRPC_LIVE_JELLYFIN_USER and MPCRPC_LIVE_JELLYFIN_PASSWORD sign in to the server. MPCRPC_LIVE_PLAYER is the
// name the player is shown under: MPC-QT, mpv, VLC, IINA, Celluloid, Plex, ...
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
	e.opts.Pipes, e.opts.MPRIS = nil, nil // the real endpoints: MPC-QT's, mpv's, IINA's (macOS) and MPRIS (Linux)
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

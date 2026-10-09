package engine

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestLivePlayer runs against a real player on this computer (MPC-QT or mpv, set up as their docs say) and the
// test's own fake Discord. It only runs on request:
//
//	MPCRPC_LIVE_PLAYER=mpv MPCRPC_LIVE_TITLE="Sample Movie" go test -run TestLivePlayer ./internal/engine
//
// MPCRPC_LIVE_PORT optionally names a web-interface port to ask first (default: none).
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
	e.opts.Pipes = nil // the real endpoints: MPC-QT's and mpv's default name
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

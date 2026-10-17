package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// newFakeKodi is Kodi's JSON-RPC web server playing a library episode (user kodi, password secret).
func newFakeKodi(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "kodi" || p != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := map[string]string{
			"Player.GetActivePlayers": `[{"playerid":1,"playertype":"internal","type":"video"}]`,
			"Player.GetItem":          `{"item":{"type":"episode","label":"Episode title","title":"Episode title","showtitle":"Sample Show","season":1,"episode":2,"year":2020,"file":"/storage/shows/Sample Show/S01E02.mkv"}}`,
			"Player.GetProperties":    `{"speed":1,"time":{"hours":0,"minutes":10,"seconds":0,"milliseconds":0},"totaltime":{"hours":0,"minutes":24,"seconds":0,"milliseconds":0}}`,
		}[req.Method]
		w.Write([]byte(`{"id":1,"jsonrpc":"2.0","result":` + result + `}`))
	}))
	t.Cleanup(s.Close)
	return s
}

func withKodi(address, password string) testMod {
	return func(cfg *core.Config, _ *Options) { cfg.KodiAddress, cfg.KodiPassword = address, password }
}

func TestKodiPlayback(t *testing.T) {
	k := newFakeKodi(t)
	e, d, l := newTestEngine(t, withPort(deadPort(t)), withKodi(k.URL, "secret"))
	e.Start()
	eventually(t, "Kodi shown", func() bool { s := e.Status(); return s.Player == "Kodi" && s.NowPlaying != nil })
	if np := *e.Status().NowPlaying; !strings.Contains(np, "Sample Show") || !strings.Contains(np, "S01E02") {
		t.Errorf("now playing %q", np)
	}
	if !l.has("Kodi's web server") || !l.has("Kodi detected.") {
		t.Error("the log should name Kodi")
	}
	eventually(t, "sent", func() bool {
		a := lastActivity(d)
		return a != nil && a["name"] == "Sample Show" && a["details"] == "Episode title"
	})
}

// A refused password is shown in the window (once in the log); without a password Kodi is not asked at all.
func TestKodiProblemsAndOff(t *testing.T) {
	k := newFakeKodi(t)
	e, _, l := newTestEngine(t, withPort(deadPort(t)), withKodi(k.URL, "wrong"))
	e.Start()
	eventually(t, "the hint", func() bool { return strings.Contains(e.Status().Hint, "Kodi refused") })
	if !l.has("Kodi refused the user name or password") {
		t.Error("the log should say so")
	}
	e.Stop()

	var asked atomic.Int32
	quiet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked.Add(1) }))
	defer quiet.Close()
	off, _, _ := newTestEngine(t, withPort(deadPort(t)), withKodi(quiet.URL, ""))
	off.Start()
	time.Sleep(700 * time.Millisecond) // a few checks
	off.Stop()
	if n := asked.Load(); n != 0 {
		t.Fatalf("Kodi was asked %d times without a password", n)
	}
}

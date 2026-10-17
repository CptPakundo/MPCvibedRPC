package engine

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus/dbustest"
)

// With mpv's events, a pause shows at once although the next regular check is a minute away.
func TestMpvPauseShowsAtOnce(t *testing.T) {
	m := newFakeMpv(t, map[string]any{
		"path": "/v/Sample Movie (2001).mkv", "filename": "Sample Movie (2001).mkv",
		"pause": false, "time-pos": 60.0, "duration": 1500.0, "speed": 1.0, "idle-active": false,
	})
	e, d, _ := newTestEngine(t, withPort(deadPort(t)), withPipes([]PlayerPipe{{"mpv", m.path}}), func(cfg *core.Config, _ *Options) {
		cfg.PollInterval = 60000
	})
	e.Start()
	// the first check finds mpv and connects to Discord; the next regular one would be a minute later
	eventually(t, "connected and watching", func() bool { return e.Status().Discord == "connected" && m.watchers() == 1 })
	n := len(d.list())

	m.set("pause", true)
	start := time.Now()
	m.event(`{"event":"property-change","id":1,"name":"pause","data":true}`)
	eventually(t, "the pause sent", func() bool { return len(d.list()) > n && e.Status().Paused })
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("the pause took %v to show", d)
	}
	if s := lastDetails(d); !strings.Contains(s, "1:00 / 25:00") {
		t.Errorf("last status: %q", s)
	}
}

// A burst of events (seeking) makes one early check, and early checks are at least nudgeGap apart.
func TestNudgesArePaced(t *testing.T) {
	e := &Engine{wake: make(chan struct{}, 1)}
	wakes := 0
	count := func(d time.Duration) {
		deadline := time.After(d)
		for {
			select {
			case <-e.wake:
				wakes++
			case <-deadline:
				return
			}
		}
	}
	for i := 0; i < 20; i++ {
		e.nudge()
	}
	count(nudgeDelay + 500*time.Millisecond)
	if wakes != 1 {
		t.Fatalf("%d checks after a burst; want 1", wakes)
	}
	e.nudge() // right after: waits for the gap
	count(time.Second)
	if wakes != 1 {
		t.Fatalf("an early check came within the gap (%d)", wakes)
	}
	count(nudgeGap)
	if wakes != 2 {
		t.Fatalf("%d checks; the waiting one should come after the gap", wakes)
	}
}

// fakeKodiEvents is Kodi's web server (as newFakeKodi, with a speed that can change) and its notification port.
type fakeKodiEvents struct {
	web     *httptest.Server
	ln      net.Listener
	speed   atomic.Int32
	mu      sync.Mutex
	clients []net.Conn
}

func newFakeKodiEvents(t *testing.T) *fakeKodiEvents {
	k := &fakeKodiEvents{}
	k.speed.Store(1)
	k.web = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := map[string]string{
			"Player.GetActivePlayers": `[{"playerid":1,"playertype":"internal","type":"video"}]`,
			"Player.GetItem":          `{"item":{"type":"movie","label":"Sample Movie","title":"Sample Movie","year":2020,"file":"/storage/movies/Sample Movie (2020).mkv"}}`,
			"Player.GetProperties":    `{"speed":` + strconv.Itoa(int(k.speed.Load())) + `,"time":{"hours":0,"minutes":10,"seconds":0,"milliseconds":0},"totaltime":{"hours":1,"minutes":30,"seconds":0,"milliseconds":0}}`,
		}[req.Method]
		w.Write([]byte(`{"id":1,"jsonrpc":"2.0","result":` + result + `}`))
	}))
	t.Cleanup(k.web.Close)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	k.ln = ln
	t.Cleanup(func() {
		ln.Close()
		k.mu.Lock()
		for _, c := range k.clients {
			c.Close()
		}
		k.mu.Unlock()
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			k.mu.Lock()
			k.clients = append(k.clients, c)
			k.mu.Unlock()
		}
	}()
	return k
}

func (k *fakeKodiEvents) connected() int { k.mu.Lock(); defer k.mu.Unlock(); return len(k.clients) }

func (k *fakeKodiEvents) notify(method string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, c := range k.clients {
		c.Write([]byte(`{"jsonrpc":"2.0","method":"` + method + `","params":{"data":{"player":{"playerid":1}},"sender":"xbmc"}}`))
	}
}

// Kodi's notifications bring the check forward too.
func TestKodiPauseShowsAtOnce(t *testing.T) {
	k := newFakeKodiEvents(t)
	e, d, _ := newTestEngine(t, withPort(deadPort(t)), withKodi(k.web.URL, "secret"), func(cfg *core.Config, o *Options) {
		cfg.PollInterval = 60000
		o.KodiEvents = k.ln.Addr().String()
	})
	e.Start()
	eventually(t, "connected and listening", func() bool { return e.Status().Discord == "connected" && k.connected() == 1 })
	n := len(d.list())
	k.speed.Store(0)
	start := time.Now()
	k.notify("Player.OnPause")
	eventually(t, "the pause sent", func() bool { return len(d.list()) > n && e.Status().Paused })
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the pause took %v to show", took)
	}
}

// MPRIS players' signals bring the check forward on the systems that read MPRIS; elsewhere the bus is never watched.
func TestMPRISPauseShowsAtOnce(t *testing.T) {
	bus := dbustest.New(t)
	var paused atomic.Bool
	e, d, _ := newTestEngine(t, withPort(deadPort(t)), func(cfg *core.Config, o *Options) {
		cfg.PollInterval = 60000
		o.MPRISBus = bus.Address
		o.MPRIS = func(time.Duration) []*core.Info {
			state := 2
			if paused.Load() {
				state = 1
			}
			in := core.NewInfo("Sample Movie (2020).mkv", "/v/Sample Movie (2020).mkv", "/v", state, 60000, 5400000, 1)
			in.Player = "Celluloid"
			return []*core.Info{in}
		}
	})
	e.Start()
	if !HasMPRIS {
		time.Sleep(500 * time.Millisecond)
		if bus.Watchers() != 0 {
			t.Fatalf("the bus is watched on %s", runtime.GOOS)
		}
		return
	}
	eventually(t, "connected and watching", func() bool { return e.Status().Discord == "connected" && bus.Watchers() == 1 })
	n := len(d.list())
	paused.Store(true)
	start := time.Now()
	bus.Emit(&dbus.Message{Serial: 9, Path: "/org/mpris/MediaPlayer2", Interface: "org.freedesktop.DBus.Properties", Member: "PropertiesChanged",
		Sender: ":1.9", Signature: "sa{sv}as", Body: []any{"org.mpris.MediaPlayer2.Player", map[string]any{"PlaybackStatus": dbus.Variant{Sig: "s", Value: "Paused"}}, []any{}}})
	eventually(t, "the pause sent", func() bool { return len(d.list()) > n && e.Status().Paused })
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the pause took %v to show", took)
	}
}

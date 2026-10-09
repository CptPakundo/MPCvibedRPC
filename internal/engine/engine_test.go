package engine

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

type mpc struct {
	mu              sync.Mutex
	state, pos, dur int
	file, filepath  string
	down            bool
	srv             *httptest.Server
	port            int
}

func newMPC(t *testing.T) *mpc {
	m := &mpc{state: 2, pos: 60000, dur: 1500000, file: "Show.Name.S01E01.720p.mkv", filepath: `C:\TV\Show Name\Season 1\Show.Name.S01E01.720p.mkv`}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.down || r.URL.Path != "/variables.html" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<html><body><p id="file">%s</p><p id="filepath">%s</p><p id="state">%d</p><p id="position">%d</p><p id="duration">%d</p><p id="playbackrate">1</p></body></html>`,
			m.file, strings.ReplaceAll(m.filepath, `\`, `\`), m.state, m.pos, m.dur)
	}))
	u, _ := url.Parse(m.srv.URL)
	m.port, _ = strconv.Atoi(u.Port())
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mpc) set(f func(*mpc)) { m.mu.Lock(); f(m); m.mu.Unlock() }

type disc struct {
	path   string
	mu     sync.Mutex
	acts   []map[string]any // args of each SET_ACTIVITY
	reject bool             // reject activities that carry a "type"
	ln     net.Listener
	conns  []net.Conn
}

func newDisc(t *testing.T) *disc {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Discord uses a Unix socket; the Windows named pipe is covered by the smoke test in CI")
	}
	d := &disc{path: filepath.Join(t.TempDir(), "discord-ipc-0")}
	ln, err := net.Listen("unix", d.path)
	if err != nil {
		t.Skip("no unix sockets")
	}
	d.ln = ln
	t.Cleanup(func() {
		ln.Close()
		d.mu.Lock()
		for _, c := range d.conns {
			c.Close()
		}
		d.mu.Unlock()
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			d.mu.Lock()
			d.conns = append(d.conns, c)
			d.mu.Unlock()
			go d.serve(c)
		}
	}()
	return d
}

func reply(c net.Conn, v any) {
	b, _ := json.Marshal(v)
	h := make([]byte, 8)
	binary.LittleEndian.PutUint32(h, 1)
	binary.LittleEndian.PutUint32(h[4:], uint32(len(b)))
	c.Write(append(h, b...))
}

func (d *disc) serve(c net.Conn) {
	h := make([]byte, 8)
	for {
		if _, err := io.ReadFull(c, h); err != nil {
			return
		}
		op := binary.LittleEndian.Uint32(h)
		body := make([]byte, binary.LittleEndian.Uint32(h[4:]))
		io.ReadFull(c, body)
		var m map[string]any
		json.Unmarshal(body, &m)
		if op == 0 {
			reply(c, map[string]any{"cmd": "DISPATCH", "evt": "READY", "data": map[string]any{}})
			continue
		}
		args, _ := m["args"].(map[string]any)
		act, _ := args["activity"].(map[string]any)
		d.mu.Lock()
		d.acts = append(d.acts, args)
		rej := d.reject && act != nil && act["type"] != nil
		d.mu.Unlock()
		if rej {
			reply(c, map[string]any{"evt": "ERROR", "nonce": m["nonce"], "data": map[string]any{"code": 4000, "message": "bad"}})
		} else {
			reply(c, map[string]any{"evt": nil, "nonce": m["nonce"], "data": nil})
		}
	}
}

func (d *disc) list() []map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]map[string]any(nil), d.acts...)
}

func eventually(t *testing.T, what string, f func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if f() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func setup(t *testing.T) (*Engine, *mpc, *disc, *logs) {
	m, d := newMPC(t), newDisc(t)
	cfg := core.DefaultConfig()
	cfg.Port = m.port
	cfg.PollInterval = 250
	cfg.ShowArtwork = false
	l := &logs{}
	e := New(cfg, Options{Log: l.add, DiscordPaths: []string{d.path}, CacheFile: filepath.Join(t.TempDir(), "c.json")})
	t.Cleanup(e.Stop)
	return e, m, d, l
}

type logs struct {
	mu sync.Mutex
	l  []string
}

func (l *logs) add(lv, msg string) { l.mu.Lock(); l.l = append(l.l, lv+" "+msg); l.mu.Unlock() }
func (l *logs) has(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.l {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestPlayPauseClearStop(t *testing.T) {
	e, m, d, l := setup(t)
	if s := e.Status(); s.Running || s.Discord != "off" {
		t.Fatalf("%+v", s)
	}
	e.Start()
	eventually(t, "activity sent", func() bool { return len(d.list()) >= 1 })
	act := d.list()[0]["activity"].(map[string]any)
	if act["type"] != float64(3) || !strings.Contains(fmt.Sprint(act["details"], act["state"], act["name"]), "Show Name") {
		t.Fatalf("unexpected activity %v", act)
	}
	// the activity reaches Discord a moment before the status shows what is playing
	eventually(t, "now playing published", func() bool { return e.Status().NowPlaying != nil })
	s := e.Status()
	if !s.Running || s.Discord != "connected" || !s.MPC || s.NowPlaying == nil || s.Paused {
		t.Fatalf("%+v", s)
	}
	if !l.has("Connected to Discord") || !l.has("MPC-HC detected") || !l.has("Playing: ") {
		t.Fatal(l.l)
	}

	// pausing sends a fresh update
	n := len(d.list())
	m.set(func(m *mpc) { m.state = 1 })
	eventually(t, "pause update", func() bool { return len(d.list()) > n })
	if !e.Status().Paused {
		t.Fatal("should be paused")
	}

	// closing the file clears the presence
	n = len(d.list())
	m.set(func(m *mpc) { m.state = 0 })
	eventually(t, "clear", func() bool { return len(d.list()) > n })
	last := d.list()[len(d.list())-1]
	if _, has := last["activity"]; has {
		t.Fatalf("expected a clear, got %v", last)
	}
	eventually(t, "nowPlaying nil", func() bool { return e.Status().NowPlaying == nil })

	// MPC disappears
	m.set(func(m *mpc) { m.down = true })
	eventually(t, "mpc down", func() bool { return !e.Status().MPC })

	// play again, then stop: presence is cleared
	m.set(func(m *mpc) { m.down = false; m.state = 2 })
	n = len(d.list())
	eventually(t, "replay", func() bool { return len(d.list()) > n })
	n = len(d.list())
	e.Stop()
	if len(d.list()) != n+1 {
		t.Fatalf("stop should clear once, got %d new", len(d.list())-n)
	}
	if s := e.Status(); s.Running || s.Discord != "off" || s.NowPlaying != nil || s.Since != nil {
		t.Fatalf("%+v", s)
	}
}

func TestSteadyPlaybackDoesNotSpam(t *testing.T) {
	e, m, d, _ := setup(t)
	e.Start()
	eventually(t, "first", func() bool { return len(d.list()) >= 1 })
	// MPC's position advances in real time; the engine must not resend
	start := time.Now()
	for time.Since(start) < 1200*time.Millisecond {
		m.set(func(m *mpc) { m.pos = 60000 + int(time.Since(start).Milliseconds()) })
		time.Sleep(50 * time.Millisecond)
	}
	if n := len(d.list()); n != 1 {
		t.Fatalf("expected 1 update, got %d", n)
	}
	// a seek does
	m.set(func(m *mpc) { m.pos += 600000 })
	eventually(t, "seek update", func() bool { return len(d.list()) >= 2 })
}

func TestBasicModeFallback(t *testing.T) {
	e, _, d, l := setup(t)
	d.reject = true
	e.Start()
	eventually(t, "two sets", func() bool { return len(d.list()) >= 2 })
	second := d.list()[1]["activity"].(map[string]any)
	if second["type"] != nil {
		t.Fatalf("basic activity must not carry a type: %v", second)
	}
	if !l.has("Falling back to basic mode") {
		t.Fatal(l.l)
	}
	eventually(t, "now playing", func() bool { return e.Status().NowPlaying != nil })
}

func TestDiscordLateAndReconnect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-socket fake; see the smoke test")
	}
	m := newMPC(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "discord-ipc-0")
	cfg := core.DefaultConfig()
	cfg.Port, cfg.PollInterval, cfg.ShowArtwork = m.port, 250, false
	l := &logs{}
	e := New(cfg, Options{Log: l.add, DiscordPaths: []string{path}})
	defer e.Stop()
	e.Start()
	eventually(t, "waiting warning", func() bool { return l.has("Could not reach Discord") })
	if s := e.Status(); s.Discord != "waiting" {
		t.Fatalf("%+v", s)
	}
	// Discord starts later
	d := &disc{path: path}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Skip(err)
	}
	d.ln = ln
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			d.mu.Lock()
			d.conns = append(d.conns, c)
			d.mu.Unlock()
			go d.serve(c)
		}
	}()
	eventually(t, "connected + sent", func() bool { return len(d.list()) >= 1 && e.Status().Discord == "connected" })

	// Discord quits: we notice, and resend after it comes back
	d.mu.Lock()
	for _, c := range d.conns {
		c.Close()
	}
	d.conns = nil
	d.mu.Unlock()
	eventually(t, "lost", func() bool { return l.has("Lost connection to Discord") })
	n := len(d.list())
	eventually(t, "resent", func() bool { return len(d.list()) > n })
}

func TestApplySettingsRestarts(t *testing.T) {
	e, _, d, l := setup(t)
	e.Start()
	eventually(t, "sent", func() bool { return len(d.list()) >= 1 })
	cfg := e.Config()
	cfg.ActivityType = "playing"
	e.ApplySettings(cfg)
	eventually(t, "restart resend", func() bool { return len(d.list()) >= 3 }) // clear + new set
	last := d.list()[len(d.list())-1]["activity"].(map[string]any)
	if last["type"] != nil && last["type"] != float64(0) {
		t.Fatalf("playing mode must not be type 3: %v", last)
	}
	if !l.has("Presence stopped") {
		t.Fatal(l.l)
	}
	// a stopped engine stays stopped after ApplySettings
	e.Stop()
	e.ApplySettings(cfg)
	if e.Status().Running {
		t.Fatal("should not have started")
	}
}

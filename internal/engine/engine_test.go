package engine

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

// fakeListener accepts the engine's connections to the fake Discord: a Unix socket, or a named pipe on Windows.
type fakeListener interface {
	Accept() (io.ReadWriteCloser, error)
	Close() error
}

type disc struct {
	path   string
	mu     sync.Mutex
	acts   []map[string]any // args of each SET_ACTIVITY
	reject bool             // reject activities that carry a "type"
	ln     fakeListener
	conns  []io.ReadWriteCloser
}

func newDisc(t *testing.T) *disc {
	d := &disc{path: fakePath(t.TempDir())}
	ln, err := listenFake(d.path)
	if err != nil {
		t.Skip("cannot listen for the fake Discord: " + err.Error())
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

func reply(c io.Writer, v any) {
	b, _ := json.Marshal(v)
	h := make([]byte, 8)
	binary.LittleEndian.PutUint32(h, 1)
	binary.LittleEndian.PutUint32(h[4:], uint32(len(b)))
	c.Write(append(h, b...))
}

func (d *disc) serve(c io.ReadWriteCloser) {
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
	e := New(cfg, Options{Log: l.add, DiscordPaths: []string{d.path}, CacheFile: filepath.Join(t.TempDir(), "c.json"), PauseUnit: 300 * time.Millisecond})
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
	m := newMPC(t)
	path := fakePath(t.TempDir())
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
	ln, err := listenFake(path)
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

// ---- clearing the status after a long pause ----

// isClear reports whether the fake Discord's last message was a clear (no activity).
func isClear(d *disc) bool {
	l := d.list()
	if len(l) == 0 {
		return false
	}
	_, has := l[len(l)-1]["activity"]
	return !has
}

// pauseSetup starts the engine playing with the given pause limit (the test engine's "minute" lasts 300 ms).
func pauseSetup(t *testing.T, minutes int) (*Engine, *mpc, *disc) {
	e, m, d, _ := setup(t)
	cfg := e.Config()
	cfg.PauseClearMinutes = minutes
	e.ApplySettings(cfg)
	e.Start()
	eventually(t, "playing update", func() bool { return len(d.list()) >= 1 })
	return e, m, d
}

func TestPauseClearsAfterTheLimitAndReturnsOnResume(t *testing.T) {
	e, m, d := pauseSetup(t, 1)
	n := len(d.list())
	m.set(func(m *mpc) { m.state = 1 })
	eventually(t, "paused update", func() bool { return len(d.list()) > n })
	if isClear(d) {
		t.Fatal("the pause itself must still show the status")
	}
	eventually(t, "status cleared after the limit", func() bool { return isClear(d) })
	if s := e.Status(); !s.PauseCleared || s.NowPlaying == nil || !s.Paused {
		t.Fatalf("status while cleared: %+v", s)
	}
	// nothing more is sent while nothing changes
	n = len(d.list())
	time.Sleep(900 * time.Millisecond)
	if len(d.list()) != n {
		t.Fatalf("expected silence while paused and cleared, got %d new messages", len(d.list())-n)
	}
	// resuming brings the status back
	m.set(func(m *mpc) { m.state = 2 })
	eventually(t, "status back on resume", func() bool { return len(d.list()) > n && !isClear(d) })
	if s := e.Status(); s.PauseCleared || s.Paused {
		t.Fatalf("status after resume: %+v", s)
	}
}

func TestPauseClearSeekWhilePausedBringsItBack(t *testing.T) {
	e, m, d := pauseSetup(t, 1)
	m.set(func(m *mpc) { m.state = 1 })
	eventually(t, "cleared", func() bool { return isClear(d) && e.Status().PauseCleared })
	n := len(d.list())
	m.set(func(m *mpc) { m.pos += 120000 }) // the user moves around while paused: they are still there
	eventually(t, "status back after a seek", func() bool { return len(d.list()) > n && !isClear(d) })
	if e.Status().PauseCleared {
		t.Fatal("PauseCleared should be false once the status is back")
	}
	// and the wait starts over: it clears again if nothing else happens
	eventually(t, "cleared again", func() bool { return isClear(d) && e.Status().PauseCleared })
}

func TestPauseClearOffByDefault(t *testing.T) {
	e, m, d := pauseSetup(t, 0)
	n := len(d.list())
	m.set(func(m *mpc) { m.state = 1 })
	eventually(t, "paused update", func() bool { return len(d.list()) > n })
	n = len(d.list())
	time.Sleep(1200 * time.Millisecond) // several "minutes" of the shortened unit
	if len(d.list()) != n || isClear(d) || e.Status().PauseCleared {
		t.Fatal("with the limit at 0 a paused video must keep its status")
	}
}

func TestPauseClearLimitRestartsWhenSettingsChange(t *testing.T) {
	e, m, d := pauseSetup(t, 1)
	m.set(func(m *mpc) { m.state = 1 })
	eventually(t, "cleared", func() bool { return isClear(d) })
	cfg := e.Config()
	cfg.PauseClearMinutes = 0 // switched off while hidden: the status returns
	e.ApplySettings(cfg)
	eventually(t, "status back when the setting is turned off", func() bool { return !isClear(d) })
	if e.Status().PauseCleared {
		t.Fatal("PauseCleared should be false after the setting is turned off")
	}
}

func TestClearArtworkCache(t *testing.T) {
	m, d := newMPC(t), newDisc(t)
	cfg := core.DefaultConfig()
	cfg.Port, cfg.PollInterval, cfg.ShowArtwork = m.port, 250, false
	cache := filepath.Join(t.TempDir(), "c.json")
	e := New(cfg, Options{DiscordPaths: []string{d.path}, CacheFile: cache})
	t.Cleanup(e.Stop)

	// stopped: the file on disk is removed
	if err := os.WriteFile(cache, []byte(`{"a":null}`), 0o644); err != nil {
		t.Fatal(err)
	}
	e.ClearArtworkCache()
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("the cache file should be removed while stopped, got %v", err)
	}

	// running: it is removed too, and the current title is sent again
	e.Start()
	eventually(t, "first update", func() bool { return len(d.list()) >= 1 })
	if err := os.WriteFile(cache, []byte(`{"a":null}`), 0o644); err != nil {
		t.Fatal(err)
	}
	n := len(d.list())
	e.ClearArtworkCache()
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("the cache file should be removed while running, got %v", err)
	}
	eventually(t, "the title is refreshed after clearing", func() bool { return len(d.list()) > n })
}

func TestPreviewTracksWhatIsSent(t *testing.T) {
	e, m, d, _ := setup(t)
	if e.Status().Preview != nil {
		t.Fatal("no preview before anything is shown")
	}
	cfg := e.Config()
	cfg.PauseClearMinutes = 1
	e.ApplySettings(cfg)
	e.Start()
	eventually(t, "preview while playing", func() bool { return e.Status().Preview != nil })
	p := e.Status().Preview
	if !p.Watching || !strings.Contains(p.Name+p.Details, "Show Name") || p.End == 0 || p.Start == 0 {
		t.Fatalf("playing preview: %+v", p)
	}

	// pausing updates it (no timestamps, a text bar in the state line)
	n := len(d.list())
	m.set(func(m *mpc) { m.state = 1 })
	eventually(t, "paused update", func() bool { return len(d.list()) > n })
	eventually(t, "paused preview", func() bool { q := e.Status().Preview; return q != nil && q.End == 0 })

	// once the status is taken down because of the long pause, the preview goes too
	eventually(t, "preview hidden with the status", func() bool { return e.Status().PauseCleared && e.Status().Preview == nil })

	// playing again brings it back; closing the file removes it
	m.set(func(m *mpc) { m.state = 2 })
	eventually(t, "preview back", func() bool { return e.Status().Preview != nil })
	m.set(func(m *mpc) { m.state = 0 })
	eventually(t, "preview gone with the file", func() bool { return e.Status().Preview == nil })
}

func TestPreviewShowsTheBasicModeFallback(t *testing.T) {
	e, _, d, _ := setup(t)
	d.reject = true // Discord refuses the Watching format, so the engine falls back
	e.Start()
	eventually(t, "preview in basic mode", func() bool { return e.Status().Preview != nil })
	p := e.Status().Preview
	if p.Watching {
		t.Fatalf("basic mode must not claim to be Watching: %+v", p)
	}
	if !strings.Contains(p.Details, "Show Name") {
		t.Fatalf("the fallback folds the title into the details line: %+v", p)
	}
}

func TestPreviewClearsWhenDiscordGoesAway(t *testing.T) {
	e, _, d, l := setup(t)
	e.Start()
	eventually(t, "preview", func() bool { return e.Status().Preview != nil })
	d.mu.Lock()
	for _, c := range d.conns {
		c.Close()
	}
	d.mu.Unlock()
	eventually(t, "lost", func() bool { return l.has("Lost connection to Discord") })
	// after the reconnect the engine sends again and the preview returns; in between it must not be stale
	eventually(t, "preview back after reconnect", func() bool { return e.Status().Preview != nil })
}
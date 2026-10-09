package engine

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeVLCWeb is VLC's web interface: status.json and playlist.json behind its password.
type fakeVLCWeb struct {
	mu       sync.Mutex
	password string
	state    string // playing | paused | stopped
	rate     float64
	calls    int
	port     int
}

func newFakeVLCWeb(t *testing.T, password string) *fakeVLCWeb {
	f := &fakeVLCWeb{password: password, state: "playing", rate: 1}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		if _, pw, ok := r.BasicAuth(); !ok || pw != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/requests/status.json":
			if f.state == "stopped" {
				fmt.Fprint(w, `{"apiversion":3,"state":"stopped","time":0,"length":0,"position":0,"rate":1,"currentplid":-1}`)
				return
			}
			fmt.Fprintf(w, `{"apiversion":3,"state":%q,"time":60,"length":1500,"position":0.04,"rate":%v,"currentplid":3,
				"information":{"category":{"meta":{"filename":"Sample Movie (2001).mkv","title":"Sample Movie"}}}}`, f.state, f.rate)
		case "/requests/playlist.json":
			fmt.Fprint(w, `{"type":"node","id":"0","children":[{"type":"node","id":"1","name":"Playlist","children":[
				{"type":"leaf","id":"3","name":"Sample Movie","uri":"file:///D:/Videos/Movies/Sample%20Movie%20(2001).mkv","current":"current"}]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	f.port = srv.Listener.Addr().(*net.TCPAddr).Port
	return f
}

func (f *fakeVLCWeb) set(fn func(*fakeVLCWeb)) { f.mu.Lock(); fn(f); f.mu.Unlock() }
func (f *fakeVLCWeb) count() int               { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

// vlcEngine is pipeEngine with VLC's web interface set up.
func vlcEngine(t *testing.T, port int, pipes []PlayerPipe, v *fakeVLCWeb, password string) (*Engine, *disc, *logs) {
	e, d, l := pipeEngine(t, port, pipes)
	e.cfg.VlcPort, e.cfg.VlcPassword = v.port, password
	return e, d, l
}

func TestVLCOverItsWebInterface(t *testing.T) {
	v := newFakeVLCWeb(t, "secret")
	e, d, l := vlcEngine(t, deadPort(t), nil, v, "secret")
	e.Start()
	eventually(t, "VLC shown", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "VLC" && s.MPC })
	if !l.has("VLC detected.") || !l.has("VLC's web interface on port") {
		t.Error("the log should name VLC")
	}
	eventually(t, "activity sent", func() bool { return len(d.list()) > 0 && !isClear(d) })
	act := d.list()[len(d.list())-1]["activity"].(map[string]any)
	if assets, _ := act["assets"].(map[string]any); assets == nil || assets["large_text"] != "VLC media player" {
		t.Errorf("VLC should be named as such: %v", act["assets"])
	}
	if !strings.Contains(*e.Status().NowPlaying, "Sample Movie") {
		t.Errorf("now playing: %q", *e.Status().NowPlaying)
	}

	v.set(func(f *fakeVLCWeb) { f.state = "paused" })
	eventually(t, "paused", func() bool { return e.Status().Paused })

	v.set(func(f *fakeVLCWeb) { f.state = "stopped" })
	eventually(t, "cleared when VLC stops", func() bool { return isClear(d) && e.Status().NowPlaying == nil })
	if s := e.Status(); !s.MPC || s.Player != "VLC" {
		t.Errorf("a stopped VLC is still a reachable player: %+v", s)
	}
}

func TestVLCSpeed(t *testing.T) {
	v := newFakeVLCWeb(t, "pw")
	v.set(func(f *fakeVLCWeb) { f.rate = 2 })
	e, d, _ := vlcEngine(t, deadPort(t), nil, v, "pw")
	e.Start()
	eventually(t, "activity sent", func() bool { return len(d.list()) > 0 && !isClear(d) })
	ts := d.list()[len(d.list())-1]["activity"].(map[string]any)["timestamps"].(map[string]any)
	if span := ts["end"].(float64) - ts["start"].(float64); span < 740000 || span > 760000 {
		t.Errorf("2x speed should give a 750 s bar, got %v ms", span)
	}
}

// VLC is asked last: a playing MPC-HC or mpv wins, an idle one does not hide a playing VLC.
func TestVLCInThePlayerOrder(t *testing.T) {
	web := newMPC(t)
	web.set(func(m *mpc) { m.state = -1 }) // MPC-HC open, nothing playing
	m := newFakeMpv(t, map[string]any{"idle-active": true})
	v := newFakeVLCWeb(t, "pw")
	e, _, _ := vlcEngine(t, web.port, []PlayerPipe{{"mpv", m.path}}, v, "pw")
	e.Start()
	eventually(t, "VLC shown", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "VLC" })

	m.set("path", `D:\Videos\Sample Movie (2001).mkv`)
	m.set("idle-active", false)
	m.set("pause", false)
	eventually(t, "mpv wins", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "mpv" })
	before := v.count()
	time.Sleep(700 * time.Millisecond)
	if v.count() != before {
		t.Error("VLC should not be asked while a player before it is playing")
	}

	web.set(func(m *mpc) { m.state = 2 })
	eventually(t, "MPC-HC wins", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "MPC-HC" })

	web.set(func(m *mpc) { m.state = -1 })
	m.set("idle-active", true)
	v.set(func(f *fakeVLCWeb) { f.state = "stopped" })
	eventually(t, "nothing plays: the first that answers is named", func() bool { s := e.Status(); return s.NowPlaying == nil && s.Player == "MPC-HC" })
}

// Without a password VLC is never asked (that is the default).
func TestVLCNotAskedWithoutPassword(t *testing.T) {
	v := newFakeVLCWeb(t, "pw")
	e, _, l := vlcEngine(t, deadPort(t), nil, v, "")
	e.Start()
	time.Sleep(700 * time.Millisecond)
	if v.count() != 0 || e.Status().MPC {
		t.Errorf("VLC asked %d times", v.count())
	}
	if l.has("VLC") {
		t.Error("the log should not mention VLC")
	}
}

func TestVLCWrongPassword(t *testing.T) {
	v := newFakeVLCWeb(t, "right")
	e, _, l := vlcEngine(t, deadPort(t), nil, v, "wrong")
	e.Start()
	eventually(t, "hint", func() bool { return strings.Contains(e.Status().Hint, "refused the password") })
	time.Sleep(700 * time.Millisecond)
	if s := e.Status(); s.MPC || s.Player != "" {
		t.Errorf("VLC refused us, yet %+v", s)
	}
	if n := countLogs(l, "refused the password"); n != 1 {
		t.Errorf("logged %d times, want once", n)
	}
	cfg := e.Config()
	cfg.VlcPassword = "right"
	e.ApplySettings(cfg)
	eventually(t, "VLC answers with the right password", func() bool { s := e.Status(); return s.Player == "VLC" && s.Hint == "" })
}

func countLogs(l *logs, sub string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.l {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

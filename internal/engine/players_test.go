package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// fakeMpv is an mpv-style IPC endpoint (named pipe on Windows, Unix socket elsewhere) answering get_property.
type fakeMpv struct {
	path   string
	mu     sync.Mutex
	values map[string]any
}

func newFakeMpv(t *testing.T, values map[string]any) *fakeMpv {
	f := &fakeMpv{path: fakePath(t.TempDir()), values: values}
	ln, err := listenFake(f.path)
	if err != nil {
		t.Skip("cannot listen for the fake mpv: " + err.Error())
	}
	var conns []io.ReadWriteCloser
	var cmu sync.Mutex
	t.Cleanup(func() {
		ln.Close()
		cmu.Lock()
		for _, c := range conns {
			c.Close()
		}
		cmu.Unlock()
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			cmu.Lock()
			conns = append(conns, c)
			cmu.Unlock()
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeMpv) set(k string, v any) { f.mu.Lock(); f.values[k] = v; f.mu.Unlock() }

func (f *fakeMpv) serve(c io.ReadWriteCloser) {
	defer c.Close()
	r := bufio.NewReader(c)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			Command []any `json:"command"`
			ID      int   `json:"request_id"`
		}
		if json.Unmarshal(line, &req) != nil || len(req.Command) < 2 {
			continue
		}
		f.mu.Lock()
		v, ok := f.values[fmt.Sprint(req.Command[1])]
		f.mu.Unlock()
		out := map[string]any{"request_id": req.ID, "error": "success", "data": v}
		if !ok {
			out = map[string]any{"request_id": req.ID, "error": "property unavailable"}
		}
		b, _ := json.Marshal(out)
		if _, err := c.Write(append(b, '\n')); err != nil {
			return
		}
	}
}

// deadPort is a local port nothing listens on, so the web interface counts as off.
func deadPort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func pipeEngine(t *testing.T, port int, pipes []PlayerPipe) (*Engine, *disc, *logs) {
	d := newDisc(t)
	cfg := core.DefaultConfig()
	cfg.Port = port
	cfg.PollInterval = 250
	cfg.ShowArtwork = false
	l := &logs{}
	e := New(cfg, Options{Pipes: func(core.Config) []PlayerPipe { return pipes }, Log: l.add, DiscordPaths: []string{d.path},
		CacheFile: filepath.Join(t.TempDir(), "c.json"), PauseUnit: 300 * time.Millisecond})
	t.Cleanup(e.Stop)
	return e, d, l
}

func lastDetails(d *disc) string {
	l := d.list()
	if len(l) == 0 {
		return ""
	}
	act, _ := l[len(l)-1]["activity"].(map[string]any)
	if act == nil {
		return ""
	}
	return fmt.Sprint(act["details"], " | ", act["state"])
}

func TestMpvOverItsPipe(t *testing.T) {
	m := newFakeMpv(t, map[string]any{
		"path": `D:\Videos\Movies\Sample Movie (2001).mkv`, "filename": "Sample Movie (2001).mkv",
		"pause": false, "time-pos": 60.0, "duration": 1500.0, "speed": 1.0, "idle-active": false,
	})
	e, d, l := pipeEngine(t, deadPort(t), []PlayerPipe{{"mpv", m.path}})
	e.Start()
	eventually(t, "mpv shown", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "mpv" && s.MPC })
	if !l.has("mpv detected.") {
		t.Error("the log should name mpv")
	}
	eventually(t, "activity sent", func() bool { return len(d.list()) > 0 && !isClear(d) })
	act := d.list()[len(d.list())-1]["activity"].(map[string]any)
	if assets, _ := act["assets"].(map[string]any); assets == nil || assets["large_text"] != "mpv" {
		t.Errorf("mpv should be named as such, not as Media Player Classic: %v", act["assets"])
	}

	m.set("pause", true)
	eventually(t, "paused", func() bool { return e.Status().Paused })

	m.set("idle-active", true)
	eventually(t, "cleared when mpv goes idle", func() bool { return isClear(d) && e.Status().NowPlaying == nil })
	if !e.Status().MPC {
		t.Error("an idle mpv is still a reachable player")
	}
}

func TestWebInterfaceWinsOverPipes(t *testing.T) {
	web := newMPC(t)
	m := newFakeMpv(t, map[string]any{"path": "/v/other.mkv", "pause": false, "idle-active": false})
	e, _, _ := pipeEngine(t, web.port, []PlayerPipe{{"mpv", m.path}})
	e.Start()
	eventually(t, "MPC-HC shown", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "MPC-HC" })

	web.set(func(m *mpc) { m.down = true }) // MPC-HC closes: mpv is picked up
	eventually(t, "switched to mpv", func() bool { return e.Status().Player == "mpv" })
}

func TestMissingPipesMeanNoPlayer(t *testing.T) {
	e, _, l := pipeEngine(t, deadPort(t), []PlayerPipe{{"MPC-QT", fakePath(t.TempDir()) + "-none"}})
	e.Start()
	time.Sleep(700 * time.Millisecond)
	if s := e.Status(); s.MPC || s.Player != "" || s.NowPlaying != nil {
		t.Errorf("nothing listens, yet %+v", s)
	}
	if l.has("detected") {
		t.Error("nothing should be detected")
	}
}

// MPC-QT imitates MPC-HC's page with quirks: "playbackRate" (meaningless), state 3 while seeking, raw "&".
func TestMpcQtWebPageQuirks(t *testing.T) {
	var mu sync.Mutex
	state := 2
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		st := state
		mu.Unlock()
		fmt.Fprintf(w, `<html><head><title>MPC-HC WebServer - Variables</title></head><body class="page-variables">
       <p id="file">Sample Movie & Friends (2001) 1080p.mkv</p>
       <p id="filepath">C:/Videos/Movies/Sample Movie & Friends (2001) 1080p.mkv</p>
       <p id="filedir">C:/Videos/Movies</p>
       <p id="state">%d</p>
       <p id="position">60000</p>
       <p id="duration">900000</p>
       <p id="playbackRate">1.24611e-306</p>
</body></html>`, st)
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(srv.URL[len("http://127.0.0.1:"):])
	e, d, _ := pipeEngine(t, port, nil)
	e.Start()
	eventually(t, "MPC-QT shown", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "MPC-QT" })
	eventually(t, "activity sent", func() bool { return len(d.list()) > 0 && !isClear(d) })
	act := d.list()[len(d.list())-1]["activity"].(map[string]any)
	ts, _ := act["timestamps"].(map[string]any)
	if ts == nil {
		t.Fatalf("no timestamps: %v", act)
	}
	start, end := ts["start"].(float64), ts["end"].(float64)
	if span := end - start; span < 800000 || span > 1000000 {
		t.Errorf("the progress bar must use normal speed: span %v ms", span)
	}
	if *e.Status().NowPlaying == "" || e.Status().Player != "MPC-QT" {
		t.Error("wrong status")
	}

	n := len(d.list())
	mu.Lock()
	state = 3 // seeking
	mu.Unlock()
	time.Sleep(800 * time.Millisecond)
	if isClear(d) || e.Status().NowPlaying == nil {
		t.Errorf("a seek must not clear the status (%d -> %d activities)", n, len(d.list()))
	}
}
func TestMpcQtPipePreferredOverItsWebPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body class="page-variables"><p id="file">Web.Name.mkv</p><p id="state">2</p><p id="position">1000</p><p id="duration">900000</p><p id="playbackRate">1.24611e-306</p></body></html>`)
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(srv.URL[len("http://127.0.0.1:"):])
	q := newFakeMpv(t, map[string]any{"path": `C:\Videos\Pipe.Name.mkv`, "pause": false, "time-pos": 1.0, "duration": 900.0, "speed": 1.5, "idle-active": false})
	e, d, _ := pipeEngine(t, port, []PlayerPipe{{"MPC-QT", q.path}})
	e.Start()
	eventually(t, "shown from the pipe", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "MPC-QT" })
	eventually(t, "activity sent", func() bool { return len(d.list()) > 0 && !isClear(d) })
	ts := d.list()[len(d.list())-1]["activity"].(map[string]any)["timestamps"].(map[string]any)
	if span := ts["end"].(float64) - ts["start"].(float64); span < 590000 || span > 610000 {
		t.Errorf("1.5x speed from the pipe should give a 600 s bar, got %v ms", span)
	}
}

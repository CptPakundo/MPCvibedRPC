package engine

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
)

func largeText(d *disc) any {
	l := d.list()
	if len(l) == 0 {
		return nil
	}
	act, _ := l[len(l)-1]["activity"].(map[string]any)
	assets, _ := act["assets"].(map[string]any)
	return assets["large_text"]
}

func TestPlayerPipesPerSystem(t *testing.T) {
	cfg := core.DefaultConfig()
	names := func(cfg core.Config) map[string]string {
		m := map[string]string{}
		for _, p := range PlayerPipes(cfg) {
			m[p.Name] = p.Path
		}
		return m
	}
	got := names(cfg)
	if got["MPC-QT"] == "" || got["mpv"] != pipe.Path("mpvsocket") {
		t.Errorf("MPC-QT and mpv are always asked: %v", got)
	}
	if runtime.GOOS == "darwin" {
		if got["IINA"] != pipe.Path("iina-mpvsocket") {
			t.Errorf("macOS asks IINA on its default name: %v", got)
		}
	} else if _, ok := got["IINA"]; ok {
		t.Errorf("IINA is only asked on macOS: %v", got)
	}
	cfg.IinaPipe, cfg.MpvPipe = "", ""
	got = names(cfg)
	if _, ok := got["IINA"]; ok || len(got) != 1 {
		t.Errorf("empty names switch mpv and IINA off: %v", got)
	}
}

func TestIinaNamedAsItself(t *testing.T) {
	m := newFakeMpv(t, map[string]any{
		"path": "/Users/someone/Movies/Sample Movie (2020).mp4", "filename": "Sample Movie (2020).mp4",
		"pause": false, "time-pos": 10.0, "duration": 600.0, "speed": 1.0, "idle-active": false,
	})
	e, d, l := pipeEngine(t, deadPort(t), []PlayerPipe{{"IINA", m.path}})
	e.Start()
	eventually(t, "IINA shown", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "IINA" })
	eventually(t, "activity sent", func() bool { return len(d.list()) > 0 && !isClear(d) })
	if lt := largeText(d); lt != "IINA" {
		t.Errorf("IINA should be named as itself, got %v", lt)
	}
	if !l.has("IINA detected.") {
		t.Error("the log should name IINA")
	}
}

// MPRIS players come after every other player, only on the systems that have D-Bus, and only while the setting is on.
func TestMPRISAskedLast(t *testing.T) {
	web := newMPC(t)
	var asked atomic.Int32
	mprisPlaying := atomic.Bool{}
	mprisPlaying.Store(true)
	e, d, l := pipeEngine(t, web.port, nil)
	e.opts.MPRIS = func(time.Duration) []*core.Info {
		asked.Add(1)
		idle := core.NewInfo("", "", "", -1, 0, 0, 1)
		idle.Player = "VLC"
		state := 2
		if !mprisPlaying.Load() {
			state = -1
		}
		in := core.NewInfo("Sample Show S01E02.mkv", "/home/someone/Videos/Sample Show S01E02.mkv", "/home/someone/Videos", state, 1000, 60000, 1)
		in.Player = "Celluloid"
		return []*core.Info{idle, in}
	}
	e.Start()
	eventually(t, "MPC-HC shown", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "MPC-HC" })
	if asked.Load() != 0 {
		t.Error("MPRIS must not be asked while MPC-HC plays")
	}
	web.set(func(m *mpc) { m.down = true })
	if !HasMPRIS {
		eventually(t, "no player", func() bool { return e.Status().Player == "" })
		time.Sleep(600 * time.Millisecond)
		if asked.Load() != 0 {
			t.Errorf("MPRIS is never asked on %s", runtime.GOOS)
		}
		return
	}
	eventually(t, "Celluloid shown", func() bool { s := e.Status(); return s.NowPlaying != nil && s.Player == "Celluloid" })
	eventually(t, "activity names Celluloid", func() bool { return largeText(d) == "Celluloid" })
	if !l.has("Celluloid detected.") {
		t.Error("the log should name Celluloid")
	}

	// nothing plays: the first player that answers is reported
	mprisPlaying.Store(false)
	eventually(t, "idle VLC reported", func() bool { s := e.Status(); return s.Player == "VLC" && s.NowPlaying == nil })

	// switched off: not asked any more
	cfg := e.Config()
	cfg.Mpris = false
	e.ApplySettings(cfg)
	time.Sleep(300 * time.Millisecond)
	n := asked.Load()
	time.Sleep(700 * time.Millisecond)
	if asked.Load() != n {
		t.Error("MPRIS is asked although it is switched off")
	}
}

// MPCvibedRPC: shows what MPC-HC, MPC-BE, MPC-QT or mpv is playing as a Discord Rich Presence. Runs from the tray; the settings are a
// small local web page opened in an app-style window.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/assets"
	"github.com/CptPakundo/MPCvibedRPC/internal/diag"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/server"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "0.9.7"

const preferredPort = 47654

var (
	background  bool
	afterUpdate bool
)

func main() {
	for _, a := range os.Args[1:] {
		switch a {
		case "--background":
			background = true
		case "--after-update":
			afterUpdate = true
		case "--version":
			fmt.Println(version)
			return
		}
	}
	if v := os.Getenv("MPCRPC_UPDATE_API"); v != "" { // tests and self-hosted mirrors
		updater.APIBase = strings.TrimRight(v, "/")
	}
	defer func() {
		if p := recover(); p != nil {
			crash(fmt.Sprint(p))
			os.Exit(1)
		}
	}()
	if err := run(); err != nil {
		crash(err.Error())
		os.Exit(1)
	}
}

func crash(msg string) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "mpcvibedrpc-crash.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), msg)
		f.Close()
	}
}

type ipcInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	Pid   int    `json:"pid"`
}

// pingExisting finds a running copy of the program (nil when there is none).
func pingExisting(ipcFile string) *ipcInfo {
	raw, err := os.ReadFile(ipcFile)
	if err != nil {
		return nil
	}
	var info ipcInfo
	if json.Unmarshal(raw, &info) != nil || info.Port == 0 {
		return nil
	}
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	res, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", info.Port))
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	var p struct {
		App string `json:"app"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&p) != nil || p.App != "MPCvibedRPC" {
		return nil
	}
	return &info
}

// logger writes to the log file (kept small) and keeps the last lines for the window.
type logger struct {
	mu   sync.Mutex
	f    *os.File
	ring []string
}

func newLogger(path string) *logger {
	if st, err := os.Stat(path); err == nil && st.Size() > 1024*1024 {
		_ = os.Rename(path, path+".old")
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	return &logger{f: f}
}

func (l *logger) log(level, msg string) {
	line := fmt.Sprintf("[%s] %s %s", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), level, msg)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ring = append(l.ring, line)
	if len(l.ring) > 300 {
		l.ring = l.ring[len(l.ring)-300:]
	}
	if l.f != nil {
		fmt.Fprintln(l.f, line)
	}
}

func (l *logger) last(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.ring) < n {
		n = len(l.ring)
	}
	return append([]string{}, l.ring[len(l.ring)-n:]...)
}

func (l *logger) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}

func windowURL(port int, token string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/?t=%s", port, token)
}

func run() error {
	st, err := store.New(store.DataDir())
	if err != nil {
		return err
	}
	ipcFile := st.IPCPath()
	browserProfile := filepath.Join(st.Dir, "window") // profile of the browser that shows the settings window
	firstRun := st.FirstRun()

	// Already running? Ask it to show its window and leave (a background start at login just leaves quietly).
	// The named mutex closes the race of two copies starting at the same moment; after an update the old copy is
	// still on its way out, so wait for it.
	deadline := time.Now().Add(1500 * time.Millisecond)
	if afterUpdate {
		deadline = time.Now().Add(10 * time.Second)
	}
	for !winsys.AcquireSingleInstance() {
		if other := pingExisting(ipcFile); other != nil {
			if !background {
				req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/open", other.Port), strings.NewReader("{}"))
				req.Header.Set("X-Token", other.Token)
				c := &http.Client{Timeout: 3 * time.Second}
				if res, err := c.Do(req); err == nil {
					res.Body.Close()
				}
			}
			return nil
		}
		if time.Now().After(deadline) {
			break // the holder is not answering: carry on rather than lock the user out
		}
		time.Sleep(250 * time.Millisecond)
	}
	if other := pingExisting(ipcFile); other != nil {
		if !background {
			req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/open", other.Port), strings.NewReader("{}"))
			req.Header.Set("X-Token", other.Token)
			c := &http.Client{Timeout: 3 * time.Second}
			if res, err := c.Do(req); err == nil {
				res.Body.Close()
			}
		}
		return nil
	}

	lg := newLogger(st.LogPath())
	defer lg.close()
	log := lg.log

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	canAutoStart := winsys.IsWindows

	msg := fmt.Sprintf("MPCvibedRPC %s starting", version)
	if background {
		msg += " in the background"
	}
	log("INFO", msg+".")
	updater.CleanupOld(exe)
	if !firstRun && !st.App().WelcomeSeen {
		// an install that existed before the welcome card was added has no need of it
		if p, err := jsonx.Parse(`{"app":{"welcomeSeen":true}}`); err == nil {
			_ = st.Update(p.(*jsonx.Obj))
		}
	}
	winsys.CloseWindowBrowser(browserProfile) // a window browser left behind by a crash

	eng := engine.New(st.Config(), engine.Options{Log: log, CacheFile: st.CachePath()})
	token := server.NewToken()

	var (
		mu         sync.Mutex
		lastUpdate *updater.Info
		tray       *winsys.Tray
		srv        *server.Server
		quitOnce   sync.Once
		quitDone   = make(chan struct{})
		notified   string // the newest version the user was told about (once per version per run)
	)
	getUpdate := func() *updater.Info { mu.Lock(); defer mu.Unlock(); return lastUpdate }
	setUpdate := func(u *updater.Info) { mu.Lock(); lastUpdate = u; mu.Unlock() }

	fullState := func() map[string]any {
		var upd any
		if u := getUpdate(); u != nil && u.Newer {
			upd = u
		}
		return map[string]any{
			"version": version, "schema": store.Sections, "values": st.Values(), "app": st.App(), "status": eng.Status(),
			"autoStart": canAutoStart && winsys.GetAutoStart(exe), "canAutoStart": canAutoStart, "dataDir": st.Dir, "update": upd,
		}
	}

	quit := func() {
		quitOnce.Do(func() {
			go func() {
				srv.Close()                               // ends the page's keep-alive connection, so an open window closes right away
				winsys.CloseWindowBrowser(browserProfile) // and no browser is left running for it
				eng.Stop()
				tray.Close()
				_ = os.Remove(ipcFile)
				log("INFO", "Quit.")
				close(quitDone)
			}()
		})
	}

	checkUpdates := func(manual bool) (*updater.Info, error) {
		a := st.App()
		if !manual && !(a.CheckUpdates && a.UpdateRepo != "") {
			return nil, nil
		}
		r, err := updater.Check(nil, a.UpdateRepo, version)
		if err != nil {
			return nil, err
		}
		if r.Newer {
			setUpdate(r)
			log("INFO", fmt.Sprintf("Update available: %s (you have %s).", r.Latest, version))
			if !manual {
				mu.Lock()
				first := notified != r.Latest
				notified = r.Latest
				t := tray
				mu.Unlock()
				if first && t != nil {
					if t.Balloon("MPCvibedRPC "+r.Latest+" is available", "You have "+version+". Click here, then open the Updates tab to install it.") {
						log("INFO", "Showed an update notice next to the tray icon.")
					} else {
						log("WARN", "Windows did not show the update notice.")
					}
				}
			}
		} else {
			setUpdate(nil)
		}
		return r, nil
	}

	writeIPC := func(port int) {
		b, _ := json.Marshal(ipcInfo{Port: port, Token: token, Pid: os.Getpid()})
		_ = os.WriteFile(ipcFile, b, 0o644)
	}

	openWindow := func() { winsys.OpenWindow(windowURL(srv.Port(), token), browserProfile) }

	handlers := map[string]server.Handler{
		"state":  {Get: true, Fn: func(*jsonx.Obj) (any, error) { return fullState(), nil }},
		"status": {Get: true, Fn: func(*jsonx.Obj) (any, error) { return eng.Status(), nil }},
		"log":    {Get: true, Fn: func(*jsonx.Obj) (any, error) { return map[string]any{"lines": lg.last(200)}, nil }},
		"diagnostics": {Get: true, Fn: func(*jsonx.Obj) (any, error) {
			return map[string]any{"text": diag.Report(diag.Input{
				Version: version, System: winsys.SystemInfo(), Players: winsys.RunningPlayerNames(),
				Config: eng.Config(), App: st.App(), Status: eng.Status(), Log: lg.last(100),
			})}, nil
		}},
		"start":       {Fn: func(*jsonx.Obj) (any, error) { eng.Start(); return eng.Status(), nil }},
		"stop":        {Fn: func(*jsonx.Obj) (any, error) { eng.Stop(); return eng.Status(), nil }},
		"open":        {Fn: func(*jsonx.Obj) (any, error) { openWindow(); return nil, nil }},
		"quit":        {Fn: func(*jsonx.Obj) (any, error) { time.AfterFunc(100*time.Millisecond, quit); return nil, nil }},
		"open-folder": {Fn: func(*jsonx.Obj) (any, error) { winsys.OpenFolder(st.Dir); return nil, nil }},
		"clear-cache": {Fn: func(*jsonx.Obj) (any, error) {
			eng.ClearArtworkCache()
			log("INFO", "Cover cache cleared.")
			return nil, nil
		}},
		"save": {Fn: func(body *jsonx.Obj) (any, error) {
			before := st.App()
			if err := st.Update(body); err != nil {
				return nil, err
			}
			eng.ApplySettings(st.Config())
			after := st.App()
			if pa, ok := body.M["app"].(*jsonx.Obj); ok && canAutoStart {
				if _, has := pa.M["autoStart"]; has {
					winsys.SetAutoStart(after.AutoStart, exe)
				}
			}
			if after.UpdateRepo != before.UpdateRepo {
				setUpdate(nil)
			}
			log("INFO", "Settings saved.")
			return fullState(), nil
		}},
		"reset": {Fn: func(*jsonx.Obj) (any, error) {
			if err := st.Reset(); err != nil {
				return nil, err
			}
			eng.ApplySettings(st.Config())
			if canAutoStart {
				winsys.SetAutoStart(st.App().AutoStart, exe)
			}
			setUpdate(nil)
			log("INFO", "Settings restored to their defaults.")
			return fullState(), nil
		}},
		"mpc-web": {Fn: func(body *jsonx.Obj) (any, error) {
			closeMpc, _ := body.M["closeMpc"].(bool)
			cfg := eng.Config()
			r := winsys.EnableMpcWebInterface(cfg.Port, closeMpc, cfg.MpvPipe)
			if r.MpvPipe != "" && r.MpvPipe != cfg.MpvPipe {
				// mpv was already set up with a name of the user's own: look for that one
				b, _ := json.Marshal(map[string]any{"settings": map[string]any{"mpvPipe": r.MpvPipe}})
				if patch, err := jsonx.Parse(string(b)); err == nil && st.Update(patch.(*jsonx.Obj)) == nil {
					eng.ApplySettings(st.Config())
					log("INFO", "Looking for mpv under the name it is set up with: "+r.MpvPipe+".")
				}
			}
			return r, nil
		}},
		"update-check": {Fn: func(*jsonx.Obj) (any, error) {
			r, err := checkUpdates(true)
			if err != nil {
				return nil, err
			}
			return r, nil
		}},
		"update-install": {Fn: func(*jsonx.Obj) (any, error) {
			info := getUpdate()
			if info == nil {
				var err error
				if info, err = checkUpdates(true); err != nil {
					return nil, err
				}
			}
			if info == nil || !info.Newer {
				return nil, fmt.Errorf("You already have the latest version.")
			}
			log("INFO", fmt.Sprintf("Downloading %s...", info.Latest))
			next, err := updater.Download(nil, info, exe)
			if err != nil {
				return nil, err
			}
			wasRunning := eng.Status().Running
			eng.Stop()
			log("INFO", "Update downloaded; restarting.")
			_ = os.Remove(ipcFile)
			// the new copy is started right away (it waits for this one to let go); we leave a moment later so the
			// settings window still gets its answer
			err = updater.SwapAndRestart(exe, next, nil, func() {
				time.AfterFunc(400*time.Millisecond, func() {
					winsys.CloseWindowBrowser(browserProfile)
					tray.Close()
					lg.close()
					os.Exit(0)
				})
			})
			if err != nil {
				log("ERROR", "Update failed: "+err.Error())
				writeIPC(srv.Port())
				if wasRunning {
					eng.Start()
				}
				return nil, err
			}
			return nil, nil
		}},
	}

	srv = server.New(server.Options{Token: token, Page: assets.UI, Icon: assets.Icon, Handlers: handlers, Version: version})
	port, err := srv.Listen(preferredPort)
	if err != nil {
		return err
	}
	writeIPC(port)
	log("INFO", fmt.Sprintf("Settings available at http://127.0.0.1:%d/", port))

	t := winsys.StartTray(winsys.TrayOptions{
		Icon: assets.Icon, Dir: st.Dir, Log: log,
		Tooltip: func() string {
			s := eng.Status()
			t := "MPCvibedRPC - "
			switch {
			case !s.Running:
				t += "stopped"
			case s.NowPlaying != nil:
				t += *s.NowPlaying
			case s.Discord == "connected" || s.Discord == "standby":
				t += "ready"
			default:
				t += "waiting for Discord"
			}
			if r := []rune(t); len(r) > 62 {
				t = string(r[:62])
			}
			return t
		},
		Running: func() bool { return eng.Status().Running },
		OnOpen:  openWindow,
		OnToggle: func() {
			if eng.Status().Running {
				eng.Stop()
			} else {
				eng.Start()
			}
		},
		OnQuit: quit,
	})
	mu.Lock()
	tray = t
	mu.Unlock()

	// The registry is the source of truth for "start with Windows"; keep its path pointing at this copy of the program.
	if canAutoStart && st.App().AutoStart {
		go winsys.SetAutoStart(true, exe)
	}
	if st.App().StartPresence {
		eng.Start()
	}
	if firstRun || (!background && st.App().OpenWindow) {
		openWindow()
	}

	time.AfterFunc(20*time.Second, func() {
		if _, err := checkUpdates(false); err != nil {
			log("WARN", "Update check failed: "+err.Error())
		}
	})
	go func() {
		for range time.Tick(24 * time.Hour) {
			_, _ = checkUpdates(false)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		quit()
		<-quitDone
	case <-quitDone:
	}
	srv.Close()
	return nil
}

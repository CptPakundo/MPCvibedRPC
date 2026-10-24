package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/assets"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/server"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// app is the running program: the settings, the engine, the local server the window talks to, and the tray.
type app struct {
	st             *store.Store
	eng            *engine.Engine
	lg             *logger
	log            func(level, msg string)
	exe            string // this program, where the autostart entry and updates point
	ipcFile        string
	browserProfile string // the profile of the browser that shows the settings window
	token          string // the window's key to the local API (new each launch)
	srv            *server.Server

	plex     *plexAccount
	stremio  *stremioAccount
	jellyfin *jellyfinAccount
	upd      updates
	windows  atomic.Int32 // settings windows open now: presence never rests while one is

	mu   sync.Mutex
	tray *winsys.Tray

	quitOnce sync.Once
	quitDone chan struct{}
}

func run() error {
	st, err := store.New(store.DataDir())
	if err != nil {
		return err
	}
	ipcFile := st.IPCPath()
	firstRun := st.FirstRun()

	// Already running? Ask it to show its window and leave (a background start at login just leaves quietly).
	// The named mutex closes the race of two copies starting at the same moment; after an update the old copy is
	// still on its way out, so wait for it.
	deadline := time.Now().Add(1500 * time.Millisecond)
	if afterUpdate {
		deadline = time.Now().Add(10 * time.Second)
	}
	for !winsys.AcquireSingleInstance(st.Dir) {
		if other := pingExisting(ipcFile); other != nil {
			handOver(other)
			return nil
		}
		if time.Now().After(deadline) {
			break // the holder is not answering: carry on rather than lock the user out
		}
		time.Sleep(250 * time.Millisecond)
	}
	if other := pingExisting(ipcFile); other != nil {
		handOver(other)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	lg := newLogger(st.LogPath())
	defer lg.close()
	a := &app{st: st, lg: lg, log: lg.log, exe: exe, ipcFile: ipcFile, browserProfile: filepath.Join(st.Dir, "window"),
		token: server.NewToken(), quitDone: make(chan struct{})}

	msg := fmt.Sprintf("MPCvibedRPC %s starting", version)
	// An automatic update leaves a marker for the copy it starts: that copy starts quietly in the tray and says so once.
	// A start without --after-update that finds one is a start by hand after that copy never got going: the marker is
	// stale and goes, so a later update by hand is not taken for an automatic one.
	autoFrom := takeAutoMarker(st.Dir)
	if !afterUpdate {
		autoFrom = ""
	}
	if autoFrom != "" {
		msg += " after updating itself from " + autoFrom
		background = true
	}
	if background {
		msg += " in the background"
	}
	a.log("INFO", msg+".")
	updater.CleanupOld(exe)
	if !firstRun && !st.App().WelcomeSeen {
		// an install that existed before the welcome card was added has no need of it
		if p, err := jsonx.Parse(`{"app":{"welcomeSeen":true}}`); err == nil {
			_ = st.Update(p.(*jsonx.Obj))
		}
	}
	winsys.CloseWindowBrowser(a.browserProfile) // a window browser left behind by a crash

	a.eng = engine.New(st.Config(), engine.Options{Log: a.log, CacheFile: st.CachePath(), Version: version, StatsFile: st.StatsPath(),
		MayRest: func() bool { return a.windows.Load() == 0 }, OnRest: winsys.TrimMemory})
	a.plex = &plexAccount{st: st, eng: a.eng, log: a.log}
	// renews a 7-day Plex token (plexJwt) in time; nothing otherwise
	engine.Go(a.log, "The Plex sign-in renewal", func() { a.plex.keepFresh(context.Background(), 0) })
	a.stremio = &stremioAccount{st: st, eng: a.eng, log: a.log}
	a.jellyfin = &jellyfinAccount{st: st, eng: a.eng, log: a.log}

	a.srv = server.New(server.Options{Token: a.token, Page: assets.UI, CSS: assets.CSS, JS: assets.JS, Icon: assets.Icon, Handlers: a.handlers(), Version: version,
		OnWindow: func(open bool) {
			if open {
				a.windows.Add(1)
				a.eng.Wake() // stop resting at once, so the window shows the player
			} else {
				a.windows.Add(-1)
			}
		}})
	port, err := a.srv.Listen(preferredPort)
	if err != nil {
		return err
	}
	a.writeIPC(port)
	a.log("INFO", fmt.Sprintf("Settings available at http://127.0.0.1:%d/", port))

	t := a.startTray()
	a.mu.Lock()
	a.tray = t
	a.mu.Unlock()
	if autoFrom != "" && t != nil {
		if !t.Balloon("MPCvibedRPC updated to "+version, "Installed by itself (you had "+autoFrom+"). Click here to see what's new.") {
			a.log("INFO", "No notice of the automatic update was shown (no tray notifications here).")
		}
	}

	// The registry is the source of truth for "start with Windows"; keep its path pointing at this copy of the program.
	if st.App().AutoStart {
		engine.Go(a.log, "The run-at-login entry", func() { winsys.SetAutoStart(true, exe) })
	}
	if st.App().StartPresence {
		a.eng.Start()
	}
	if firstRun || (!background && st.App().OpenWindow) {
		a.openWindow()
	}
	a.scheduleUpdateChecks()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		a.quit()
		<-a.quitDone
	case <-a.quitDone:
	}
	a.srv.Close()
	return nil
}

func (a *app) getTray() *winsys.Tray {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tray
}

func (a *app) writeIPC(port int) {
	b, _ := json.Marshal(ipcInfo{Port: port, Token: a.token, Pid: os.Getpid()})
	_ = os.WriteFile(a.ipcFile, b, 0o600)
}

// ipcRemoveWait is how long removeIPC keeps trying.
var ipcRemoveWait = time.Second

// removeIPC deletes ipc.json. On Windows a file another process has open for reading cannot be deleted, so it is
// tried again every 50 ms for up to ipcRemoveWait; a file that is already gone counts as removed.
func removeIPC(path string) error {
	deadline := time.Now().Add(ipcRemoveWait)
	for {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (a *app) openWindow() { winsys.OpenWindow(windowURL(a.srv.Port(), a.token), a.browserProfile) }

// quit shuts the program down once (the window's Quit, the tray's Quit, or a signal).
func (a *app) quit() {
	a.quitOnce.Do(func() {
		go func() {
			a.srv.Close() // ends the page's keep-alive connection, so an open window closes right away
			// the port and token are gone with the server: a new copy must not find them while this one winds down
			_ = removeIPC(a.ipcFile)
			winsys.CloseWindowBrowser(a.browserProfile) // and no browser is left running for it
			a.plex.stopWaiting()
			a.stremio.stopWaiting()
			a.jellyfin.stopWaiting()
			a.eng.Stop()
			a.getTray().Close()
			a.log("INFO", "Quit.")
			close(a.quitDone)
		}()
	})
}

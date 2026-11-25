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

	"github.com/CptPakundo/MPRP/internal/assets"
	"github.com/CptPakundo/MPRP/internal/engine"
	"github.com/CptPakundo/MPRP/internal/jsonx"
	"github.com/CptPakundo/MPRP/internal/notify"
	"github.com/CptPakundo/MPRP/internal/server"
	"github.com/CptPakundo/MPRP/internal/store"
	"github.com/CptPakundo/MPRP/internal/updater"
	"github.com/CptPakundo/MPRP/internal/winsys"
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

	plex      *plexAccount
	stremio   *stremioAccount
	jellyfin  *jellyfinAccount
	upd       updates
	windows   atomic.Int32   // settings windows open now: presence never rests while one is
	appWindow atomic.Bool    // the last window opened is the program's own app window, not a tab of the user's browser
	inTray    atomic.Bool    // the program closed its window itself (minimised to the tray): its going is not the X
	watching  atomic.Bool    // watchMinimise runs
	opened    atomic.Int64   // windows asked for so far: one asked for after a window went means it was not the X
	winHandle atomic.Uintptr // the settings window as watchMinimise last found it (Windows; 0: not known)
	winSys    *windowSys     // nil: the system's window functions (tests stand in for a window)
	busyUntil atomic.Int64   // until then (Unix nanoseconds) no update installs itself: see settle

	mu       sync.Mutex
	tray     *winsys.Tray
	notifier *notify.Notifier // Linux: the desktop notification service (made at the first notification)

	quitOnce sync.Once
	quitting atomic.Bool // quit has begun: an install that fails now must not bring the server's file or the engine back
	quitDone chan struct{}
}

func run() error {
	dir, other, moved := chooseDataDir(store.DataDir(), store.FormerDataDir(), afterUpdate || updateFailed, pingExisting)
	if other != nil {
		handOver(other) // a copy of the former version runs: it is the one to show
		return nil
	}
	st, err := store.New(dir)
	if err != nil {
		return err
	}
	ipcFile := st.IPCPath()
	firstRun := st.FirstRun()

	// Already running? Ask it to show its window and leave (a background start at login just leaves quietly).
	// The named mutex closes the race of two copies starting at the same moment. Its holder may not answer yet: it is
	// still starting (ipc.json is written once its server listens; a start at login can take seconds), or, after an
	// update, the old copy is still on its way out. So wait for it.
	deadline := time.Now().Add(10 * time.Second)
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
	st.Log = lg.log
	a := &app{st: st, lg: lg, log: lg.log, exe: exe, ipcFile: ipcFile, browserProfile: filepath.Join(st.Dir, "window"),
		token: server.NewToken(), quitDone: make(chan struct{})}
	a.busy()

	msg := fmt.Sprintf("MPRP %s starting", version)
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
	if moved != "" {
		a.log("INFO", moved)
	}
	failedUpdate := ""
	if updateFailed {
		if failedUpdate = failedVersion(st.Dir); failedUpdate != "" {
			a.log("WARN", "The update to "+failedUpdate+" did not start, so "+version+" is back. That version is not installed by itself again; a newer one will be.")
		}
	}
	if afterUpdate {
		// The copy that started this one keeps the old program file until this one has shown that it runs, to put it
		// back otherwise. So the leftovers go a little later, once that copy is gone.
		engine.Go(a.log, "The clean-up after the update", func() {
			time.Sleep(proofWait + 30*time.Second)
			updater.CleanupOld(exe)
			_ = os.Remove(filepath.Join(st.Dir, failedMarker))
		})
	} else {
		updater.CleanupOld(exe)
	}
	if !firstRun && !st.App().WelcomeSeen {
		// an install that existed before the welcome card was added has no need of it
		if p, err := jsonx.Parse(`{"app":{"welcomeSeen":true}}`); err == nil {
			_ = st.Update(p.(*jsonx.Obj))
		}
	}
	winsys.CloseWindowBrowser(a.browserProfile) // a window browser left behind by a crash
	if n, err := st.MigrateMatchFixes(); err != nil {
		a.log("WARN", "Could not save the Fix a match lines as fixes ("+err.Error()+"); they still act as before.")
	} else if n > 0 {
		a.log("INFO", fmt.Sprintf("Fix a match: the artworkAliases and artworkOverrides lines became fixes (titles: %d). The old lines stay in the settings file.", n))
	}

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
				a.windowOpened()
			} else {
				a.windowClosed()
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
	if failedUpdate != "" {
		a.notify("MPRP could not update to "+failedUpdate, "The new version did not start, so "+version+" is back.")
	}
	if autoFrom != "" {
		if !a.notify("MPRP updated to "+version, "Installed by itself (you had "+autoFrom+"). Click here to see what's new.") {
			a.log("INFO", "No notice of the automatic update was shown (no notifications here).")
		}
	}

	// The registry is the source of truth for "start with Windows"; keep its path pointing at this copy of the program.
	// An entry of the program's former name is carried over first (it does nothing when there is none).
	engine.Go(a.log, "The run-at-login entry", func() {
		if note := winsys.MigrateAutoStart(exe); note != "" {
			a.log("INFO", note)
		}
		if st.App().AutoStart {
			winsys.RefreshAutoStart(exe)
		}
	})
	if st.App().StartPresence {
		a.eng.Start()
	}
	// An update installed from the window comes back with its window (and what's new in it), whatever "Show this
	// window when I open the app" says: the window the user pressed Update in must not just vanish. An update that
	// installed itself set background above.
	if firstRun || (!background && (st.App().OpenWindow || afterUpdate)) {
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
	// in one step: a second start that reads the file meanwhile must not find half of it and take it for "none running"
	tmp := a.ipcFile + ".tmp"
	if os.WriteFile(tmp, b, 0o600) != nil || os.Rename(tmp, a.ipcFile) != nil {
		_ = os.Remove(tmp)
		_ = os.WriteFile(a.ipcFile, b, 0o600)
	}
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

func (a *app) openWindow() {
	a.busy()
	// a window that is open already comes to the front: two windows would each keep their own copy of the values
	if a.windows.Load() > 0 && winsys.FocusWindow(a.browserProfile) {
		return
	}
	a.opened.Add(1)
	a.winHandle.Store(0)
	a.inTray.Store(false)
	mode := winsys.OpenWindow(windowURL(a.srv.Port(), a.srv.NewCode()), a.browserProfile)
	a.appWindow.Store(mode == "app")
	if mode == "app" {
		// the window is a browser's: its taskbar button gets the program's name, icon and command (Windows)
		engine.Go(a.log, "Naming the window", func() { winsys.NameWindow(a.browserProfile) })
	}
}

// quit shuts the program down once (the window's Quit, the tray's Quit, or a signal).
func (a *app) quit() {
	a.quitOnce.Do(func() {
		a.quitting.Store(true)
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
			a.getNotifier().Close()
			a.log("INFO", "Quit.")
			close(a.quitDone)
		}()
	})
}

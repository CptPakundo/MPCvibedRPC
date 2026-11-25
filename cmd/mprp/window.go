package main

import (
	"runtime"
	"time"

	"github.com/CptPakundo/MPRP/internal/winsys"
)

// The settings window's X quits the program, and minimising it sends it to the tray (minimizeToTray, Windows).
// Both apply only to the program's own app window (not to a tab of the user's browser) and not on macOS, where the
// window closes to the menu bar as Mac apps do.

// windowSys is what the window code asks of the system, and its waits. Tests give an app one of their own (app.winSys).
type windowSys struct {
	find  func(profileDir string) uintptr // the settings window's handle (Windows; 0: not found)
	state func(h uintptr) (exists, minimised bool)
	close func(h uintptr) bool // asks the window to close; false: the request could not be delivered
	quit  func(a *app)         // what the X does
	goos  string

	// goneWait is how long the program waits after the last window's connection ended before it takes that as the
	// window's X: a reload or a broken connection (the page reconnects within about a second) comes back in time.
	goneWait time.Duration
	// goneCheck is how long a window that still stands after its page went is watched for going (Windows): the X
	// destroys the window, a reload keeps it.
	goneCheck, goneEvery time.Duration
	// minimiseEvery is how often an open window is checked for being minimised; notFoundEvery when it was not found.
	minimiseEvery, notFoundEvery time.Duration
	// closeFailedAfter: a window the program closed (minimised to the tray) that still stands this long after, and is
	// no longer minimised, did not close (a page dialog, a refused close): its X quits again.
	closeFailedAfter time.Duration
}

var systemWindow = windowSys{
	find: winsys.SettingsWindow, state: winsys.WindowState, close: winsys.CloseWindow, quit: (*app).quit, goos: runtime.GOOS,
	goneWait: 3 * time.Second, goneCheck: time.Second, goneEvery: 25 * time.Millisecond,
	minimiseEvery: 250 * time.Millisecond, notFoundEvery: 2 * time.Second, closeFailedAfter: 2 * time.Second,
}

func (a *app) win() *windowSys {
	if a.winSys != nil {
		return a.winSys
	}
	return &systemWindow
}

// windowOpened counts a window's page that connected.
func (a *app) windowOpened() {
	if a.windows.Add(1) == 1 && winsys.IsWindows && a.appWindow.Load() && a.watching.CompareAndSwap(false, true) {
		go a.watchMinimise()
	}
	a.eng.Wake() // stop resting at once, so the window shows the player
}

// windowClosed counts a window's page that went. When it was the last, the window may have been closed with its X:
// then the program quits. On Windows that is known at once: the window itself is gone (a reload keeps it).
// Otherwise, and when the window still stands after goneCheck, the program quits once no page is back within
// goneWait (see windowSys). A window asked for in the meantime (still loading) keeps it.
func (a *app) windowClosed() {
	if a.windows.Add(-1) > 0 {
		return
	}
	// as things were when it went: a window opened again from the tray meanwhile clears inTray
	inTray, opened, h := a.inTray.Load(), a.opened.Load(), a.winHandle.Load()
	w := a.win()
	quitIfClosed := func() {
		if a.opened.Load() == opened && closeQuits(a.windows.Load(), a.appWindow.Load(), inTray, a.quitting.Load(), w.goos) {
			a.log("INFO", "The settings window was closed: quitting.")
			w.quit(a)
		}
	}
	if h == 0 {
		time.AfterFunc(w.goneWait, quitIfClosed)
		return
	}
	go func() {
		for end := time.Now().Add(w.goneCheck); time.Now().Before(end); time.Sleep(w.goneEvery) {
			if exists, _ := w.state(h); !exists {
				quitIfClosed()
				return
			}
		}
		time.AfterFunc(w.goneWait, quitIfClosed) // the window still stands: a reload, or a page that lost its connection
	}()
}

// closeQuits decides whether the settings window having gone means the user closed it: none is open, it was the
// program's app window, the program did not send it to the tray itself and is not quitting already, and not macOS.
func closeQuits(windows int32, appWindow, inTray, quitting bool, goos string) bool {
	return windows == 0 && appWindow && !inTray && !quitting && goos != "darwin"
}

// watchMinimise runs while a window is open (Windows): a window that gets minimised is closed instead when
// minimizeToTray is on, so it lives on in the tray icon. One runs at a time (a.watching).
func (a *app) watchMinimise() {
	w := a.win()
	var h uintptr
	wasMin, missed := false, false
	var closedAt time.Time // when the program closed the window (minimised to the tray)
	for {
		wait := w.minimiseEvery
		if missed {
			wait = w.notFoundEvery
		}
		time.Sleep(wait)
		if a.windows.Load() == 0 || a.quitting.Load() {
			a.watching.Store(false)
			// a window that opened just now found this watcher still running: carry on for it
			if a.windows.Load() == 0 || a.quitting.Load() || !a.watching.CompareAndSwap(false, true) {
				return
			}
			h, wasMin = 0, false
		}
		if h == 0 {
			if h = w.find(a.browserProfile); h == 0 {
				missed = true
				continue
			}
			missed = false
			a.winHandle.Store(h) // for windowClosed
		}
		exists, min := w.state(h)
		if !exists {
			h, wasMin = 0, false
			closedAt = time.Time{}
			a.winHandle.Store(0)
			continue
		}
		if min && !wasMin && a.st.App().MinimizeToTray {
			a.inTray.Store(true) // its going is not the X: the program stays
			if w.close(h) {
				closedAt = time.Now()
				a.log("INFO", "The settings window was minimized to the tray.")
			} else {
				a.inTray.Store(false)
				a.log("WARN", "The settings window could not be closed to the tray; it stays minimized.")
			}
		}
		// a close that did not happen (the window stands again, restored) must not keep the X from quitting later
		if !closedAt.IsZero() && !min && time.Since(closedAt) >= w.closeFailedAfter && a.inTray.CompareAndSwap(true, false) {
			closedAt = time.Time{}
			a.log("INFO", "The settings window did not close to the tray; its X quits again.")
		}
		wasMin = min
	}
}

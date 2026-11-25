package main

import (
	"runtime"
	"time"

	"github.com/CptPakundo/MPRP/internal/winsys"
)

// The settings window's X quits the program, and minimising it sends it to the tray (minimizeToTray, Windows).
// Both apply only to the program's own app window (not to a tab of the user's browser) and not on macOS, where the
// window closes to the menu bar as Mac apps do.

var (
	// windowGoneWait is how long the program waits after the last window's connection ended before it takes that as
	// the window's X: a reload or a broken connection (the page reconnects within about a second) comes back in time.
	windowGoneWait = 3 * time.Second
	// goneCheck is how long a window that still stands after its page went is watched for going (Windows): the X
	// destroys the window, a reload keeps it.
	goneCheck = time.Second
	goneEvery = 25 * time.Millisecond
	// minimiseEvery is how often an open window is checked for being minimised; notFoundEvery when it was not found.
	minimiseEvery = 250 * time.Millisecond
	notFoundEvery = 2 * time.Second
)

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
// windowGoneWait. A window asked for in the meantime (still loading) keeps it.
func (a *app) windowClosed() {
	if a.windows.Add(-1) > 0 {
		return
	}
	// as things were when it went: a window opened again from the tray meanwhile clears inTray
	inTray, opened, h := a.inTray.Load(), a.opened.Load(), a.winHandle.Load()
	quitIfClosed := func() {
		if a.opened.Load() == opened && closeQuits(a.windows.Load(), a.appWindow.Load(), inTray, a.quitting.Load(), runtime.GOOS) {
			a.log("INFO", "The settings window was closed: quitting.")
			a.quit()
		}
	}
	if h == 0 {
		time.AfterFunc(windowGoneWait, quitIfClosed)
		return
	}
	go func() {
		for end := time.Now().Add(goneCheck); time.Now().Before(end); time.Sleep(goneEvery) {
			if exists, _ := winsys.WindowState(h); !exists {
				quitIfClosed()
				return
			}
		}
		time.AfterFunc(windowGoneWait, quitIfClosed) // the window still stands: a reload, or a page that lost its connection
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
	var h uintptr
	wasMin, missed := false, false
	for {
		wait := minimiseEvery
		if missed {
			wait = notFoundEvery
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
			if h = winsys.SettingsWindow(a.browserProfile); h == 0 {
				missed = true
				continue
			}
			missed = false
			a.winHandle.Store(h) // for windowClosed
		}
		exists, min := winsys.WindowState(h)
		if !exists {
			h, wasMin = 0, false
			a.winHandle.Store(0)
			continue
		}
		if min && !wasMin && a.st.App().MinimizeToTray {
			a.inTray.Store(true) // its going is not the X: the program stays
			winsys.CloseWindow(h)
			a.log("INFO", "The settings window was minimized to the tray.")
		}
		wasMin = min
	}
}

package main

import (
	"runtime"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// The settings window's X quits the program, and minimising it sends it to the tray (minimizeToTray, Windows).
// Both apply only to the program's own app window (not to a tab of the user's browser) and not on macOS, where the
// window closes to the menu bar as Mac apps do.

var (
	// windowGoneWait is how long the program waits after the last window's connection ended before it takes that as
	// the window's X: a reload or a broken connection (the page reconnects within about a second) comes back in time.
	windowGoneWait = 3 * time.Second
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

// windowClosed counts a window's page that went. When it was the last, and it stays the last for windowGoneWait,
// the window was closed with its X: the program quits. A window asked for in the meantime (still loading) keeps it.
func (a *app) windowClosed() {
	if a.windows.Add(-1) > 0 {
		return
	}
	// as things were when it went: a window opened again from the tray meanwhile clears inTray
	inTray, opened := a.inTray.Load(), a.opened.Load()
	time.AfterFunc(windowGoneWait, func() {
		if a.opened.Load() == opened && closeQuits(a.windows.Load(), a.appWindow.Load(), inTray, a.quitting.Load(), runtime.GOOS) {
			a.log("INFO", "The settings window was closed: quitting.")
			a.quit()
		}
	})
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
	wasMin := false
	for {
		wait := minimiseEvery
		if h == 0 {
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
				continue
			}
		}
		exists, min := winsys.WindowState(h)
		if !exists {
			h, wasMin = 0, false
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

package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The settings window's going quits the program only when the user closed the program's own window.
func TestCloseQuits(t *testing.T) {
	for _, c := range []struct {
		name                        string
		windows                     int32
		appWindow, inTray, quitting bool
		goos                        string
		want                        bool
	}{
		{"closed with its X", 0, true, false, false, "windows", true},
		{"Linux app window", 0, true, false, false, "linux", true},
		{"a window is open again (a reload)", 1, true, false, false, "windows", false},
		{"a tab of the user's browser", 0, false, false, false, "windows", false},
		{"minimised to the tray", 0, true, true, false, "windows", false},
		{"quitting already", 0, true, false, true, "windows", false},
		{"macOS closes to the menu bar", 0, true, false, false, "darwin", false},
	} {
		if got := closeQuits(c.windows, c.appWindow, c.inTray, c.quitting, c.goos); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// fakeWindow stands in for the settings window: whether it exists and is minimised, and whether a close is refused.
type fakeWindow struct {
	mu          sync.Mutex
	exists, min bool
	refuse      bool // a close is delivered but the window stays (a page dialog)
	undelivered bool // a close cannot even be delivered
	closes      int
}

func (w *fakeWindow) set(f func(w *fakeWindow)) { w.mu.Lock(); f(w); w.mu.Unlock() }

func (w *fakeWindow) closed() int { w.mu.Lock(); defer w.mu.Unlock(); return w.closes }

// The short waits of the tests.
const (
	testGoneWait    = 300 * time.Millisecond
	testGoneCheck   = 150 * time.Millisecond
	testCloseFailed = 60 * time.Millisecond
)

// fakeSys is the system's window functions with w in place of the window, short waits, and an X that counts its
// quits (a counted quit marks its app as quitting, so nothing real runs).
func fakeSys(w *fakeWindow) (sys *windowSys, quits func() int32) {
	n := &atomic.Int32{}
	sys = &windowSys{
		find: func(string) uintptr {
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.exists {
				return 7
			}
			return 0
		},
		state: func(h uintptr) (bool, bool) {
			w.mu.Lock()
			defer w.mu.Unlock()
			return h == 7 && w.exists, w.exists && w.min
		},
		close: func(uintptr) bool {
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.undelivered {
				return false
			}
			w.closes++
			if !w.refuse {
				w.exists = false
			}
			return true
		},
		quit:     func(a *app) { n.Add(1); a.quitting.Store(true) },
		goos:     "windows",
		goneWait: testGoneWait, goneCheck: testGoneCheck, goneEvery: 5 * time.Millisecond,
		minimiseEvery: 5 * time.Millisecond, notFoundEvery: 5 * time.Millisecond, closeFailedAfter: testCloseFailed,
	}
	return sys, n.Load
}

// windowApp is a program with its app window open (one page connected); h is the window as watchMinimise found it.
func windowApp(t *testing.T, sys *windowSys, h uintptr) *app {
	a, _ := autoApp(t)
	a.winSys = sys
	a.appWindow.Store(true)
	a.windows.Store(1)
	a.winHandle.Store(h)
	t.Cleanup(func() { a.quitting.Store(true); a.windows.Store(0) }) // no late timer quits; a watcher ends
	return a
}

// The order of the two signs of the X: the page's /api/alive connection drops, and (Windows) the window itself goes.
func TestWindowXOrder(t *testing.T) {
	w := &fakeWindow{exists: true}
	sys, quits := fakeSys(w)
	after := func(d time.Duration) int32 { time.Sleep(d); return quits() }
	drop := func(a *app) { a.windowClosed() } // the page's connection ends (it counts the page out)

	// The window is already gone when its connection drops: quit at once, without waiting for a reload.
	w.set(func(w *fakeWindow) { w.exists = false })
	drop(windowApp(t, sys, 7))
	if n := after(100 * time.Millisecond); n != 1 {
		t.Fatalf("window gone first: %d quits within 100 ms, want 1", n)
	}

	// The connection drops first and the window goes a moment later.
	w.set(func(w *fakeWindow) { w.exists = true })
	drop(windowApp(t, sys, 7))
	time.Sleep(40 * time.Millisecond)
	w.set(func(w *fakeWindow) { w.exists = false })
	if n := after(80 * time.Millisecond); n != 2 {
		t.Fatalf("window gone after its page: %d quits, want 2 (before the reload wait)", n)
	}

	// A reload: the window stands and its page comes back within the wait.
	w.set(func(w *fakeWindow) { w.exists = true })
	a := windowApp(t, sys, 7)
	drop(a)
	time.Sleep(50 * time.Millisecond)
	a.windows.Add(1)
	if n := after(testGoneCheck + testGoneWait + 100*time.Millisecond); n != 2 {
		t.Fatalf("a reload quit the program (%d quits)", n)
	}

	// The window stands but its page never comes back: quit after the window check and the reload wait.
	drop(windowApp(t, sys, 7))
	if n := after(testGoneCheck + testGoneWait/2); n != 2 {
		t.Fatalf("quit before the reload wait was over (%d quits)", n)
	}
	if n := after(testGoneWait); n != 3 {
		t.Fatalf("a page that never came back: %d quits, want 3", n)
	}

	// The window is not known (Linux, or not found yet): only the reload wait decides.
	drop(windowApp(t, sys, 0))
	if n := after(testGoneWait / 2); n != 3 {
		t.Fatalf("no window known: quit before the reload wait (%d quits)", n)
	}
	if n := after(testGoneWait); n != 4 {
		t.Fatalf("no window known: %d quits, want 4", n)
	}
	a = windowApp(t, sys, 0)
	drop(a)
	a.windows.Add(1) // reloaded
	if n := after(testGoneWait + 100*time.Millisecond); n != 4 {
		t.Fatalf("no window known, page back: %d quits, want 4", n)
	}

	// A window asked for meanwhile (from the tray, still loading) keeps the program.
	a = windowApp(t, sys, 0)
	drop(a)
	a.opened.Add(1)
	if n := after(testGoneWait + 100*time.Millisecond); n != 4 {
		t.Fatalf("a window asked for meanwhile: %d quits, want 4", n)
	}

	// The program closed the window itself (minimised to the tray): not the X.
	w.set(func(w *fakeWindow) { w.exists = false })
	a = windowApp(t, sys, 7)
	a.inTray.Store(true)
	drop(a)
	if n := after(testGoneWait + 100*time.Millisecond); n != 4 {
		t.Fatalf("minimised to the tray: %d quits, want 4", n)
	}
}

// Minimising sends the window to the tray; a close that did not happen must not keep a later X from quitting.
func TestMinimiseToTray(t *testing.T) {
	w := &fakeWindow{exists: true}
	sys, quits := fakeSys(w)
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for end := time.Now().Add(2 * time.Second); !cond(); time.Sleep(5 * time.Millisecond) {
			if time.Now().After(end) {
				t.Fatal("timed out waiting for " + what)
			}
		}
	}
	start := func() *app { // a window open, watched for being minimised
		a := windowApp(t, sys, 0)
		setApp(t, a.st, `{"minimizeToTray":true}`)
		a.watching.Store(true)
		go a.watchMinimise()
		waitFor("the window to be found", func() bool { return a.winHandle.Load() == 7 })
		return a
	}

	// It closes: the page's going is not the X.
	a := start()
	w.set(func(w *fakeWindow) { w.min = true })
	waitFor("the close", func() bool { return w.closed() == 1 })
	if !a.inTray.Load() {
		t.Fatal("inTray not set")
	}
	a.windowClosed()
	time.Sleep(testGoneWait + 100*time.Millisecond)
	if quits() != 0 {
		t.Fatal("minimising to the tray quit the program")
	}

	// The close is refused (the window stays, and the user brings it back): its X quits again.
	w.set(func(w *fakeWindow) { w.exists, w.min, w.refuse, w.closes = true, false, true, 0 })
	a = start()
	w.set(func(w *fakeWindow) { w.min = true })
	waitFor("the close", func() bool { return w.closed() == 1 })
	time.Sleep(3 * testCloseFailed)
	if !a.inTray.Load() {
		t.Fatal("still minimised (it may be closing slowly): inTray must stay")
	}
	w.set(func(w *fakeWindow) { w.min = false }) // restored by the user
	waitFor("inTray to clear", func() bool { return !a.inTray.Load() })
	w.set(func(w *fakeWindow) { w.exists = false }) // the user's X
	a.windowClosed()
	waitFor("the quit", func() bool { return quits() == 1 })

	// The close cannot be delivered at all: the window stays minimised and inTray is not left set.
	w.set(func(w *fakeWindow) { w.exists, w.min, w.refuse, w.undelivered, w.closes = true, false, false, true, 0 })
	a = start()
	w.set(func(w *fakeWindow) { w.min = true })
	time.Sleep(50 * time.Millisecond)
	if a.inTray.Load() {
		t.Fatal("inTray left set although the close never went out")
	}
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// updates is what the program knows about newer versions.
type updates struct {
	mu         sync.Mutex
	last       *updater.Info // the last check's answer
	notified   string        // the newest version the user was told about (once per version per run)
	installing atomic.Bool   // an update is being downloaded or swapped in
	auto       atomic.Bool   // an automatic install waits for a quiet moment
}

func (u *updates) get() *updater.Info  { u.mu.Lock(); defer u.mu.Unlock(); return u.last }
func (u *updates) set(i *updater.Info) { u.mu.Lock(); u.last = i; u.mu.Unlock() }

// newer is the last check's answer when it found a newer version, else nil.
func (u *updates) newer() *updater.Info {
	if i := u.get(); i != nil && i.Newer {
		return i
	}
	return nil
}

// checkUpdates asks GitHub for a newer version: when the window asks (manual), or in the background while
// "Check for updates automatically" is on. A version found in the background is announced next to the tray icon once.
func (a *app) checkUpdates(manual bool) (*updater.Info, error) {
	s := a.st.App()
	if !manual && !(s.CheckUpdates && s.UpdateRepo != "") {
		return nil, nil
	}
	r, err := updater.Check(nil, s.UpdateRepo, version)
	if err != nil {
		return nil, err
	}
	if !r.Newer {
		a.upd.set(nil)
		return r, nil
	}
	a.upd.set(r)
	a.log("INFO", fmt.Sprintf("Update available: %s (you have %s).", r.Latest, version))
	if a.autoInstall() || manual {
		return r, nil // installed by itself at a quiet moment: it says so afterwards instead
	}
	a.upd.mu.Lock()
	first := a.upd.notified != r.Latest
	a.upd.notified = r.Latest
	a.upd.mu.Unlock()
	t := a.getTray()
	if first && t != nil && winsys.IsWindows { // the macOS menu bar icon has no notifications
		if t.Balloon("MPCvibedRPC "+r.Latest+" is available", "You have "+version+". Click here to see what's new and install it.") {
			a.log("INFO", "Showed an update notice next to the tray icon.")
		} else {
			a.log("WARN", "Windows did not show the update notice.")
		}
	}
	return r, nil
}

// autoWait is how long an automatic install waits before looking again for a quiet moment.
var autoWait = 2 * time.Minute

// quietMoment: nothing is shown on Discord (nothing plays, or a pause was cleared) and no settings window is open,
// so restarting into a new version interrupts nobody.
func quietMoment(windows int32, s engine.Status) bool {
	return windows == 0 && (s.NowPlaying == nil || s.PauseCleared)
}

// autoInstall installs a found update by itself, at the next quiet moment, when "Install updates automatically" is
// on (with checks on, and where the program can replace itself: Windows and Linux). It reports whether it will. It
// waits in the background, looking again every autoWait; a failed install is left to the next check.
func (a *app) autoInstall() bool {
	enabled := func() bool {
		s := a.st.App()
		i := a.upd.newer()
		return s.CheckUpdates && s.AutoUpdate && i != nil && i.CanInstall
	}
	if !enabled() {
		return false
	}
	if !a.upd.auto.CompareAndSwap(false, true) {
		return true // already waiting
	}
	engine.Go(a.log, "The automatic update", func() {
		defer a.upd.auto.Store(false)
		for !quietMoment(a.windows.Load(), a.eng.Status()) {
			time.Sleep(autoWait)
			if !enabled() {
				return // switched off meanwhile, or installed by hand
			}
		}
		i := a.upd.newer()
		if i == nil {
			return // gone meanwhile: a check found nothing newer, or the settings were restored
		}
		latest := i.Latest
		a.log("INFO", "Installing "+latest+" by itself (Install updates automatically is on).")
		marker := filepath.Join(a.st.Dir, autoMarker)
		_ = os.WriteFile(marker, []byte(version), 0o644)
		if err := a.installUpdate(); err != nil {
			_ = os.Remove(marker)
			a.log("WARN", "The automatic update to "+latest+" failed: "+err.Error()+" The next check tries again.")
		}
	})
	return true
}

// autoMarker is the file an automatic update leaves for the new copy: it then starts in the background (no window)
// and says once that it was updated. It holds the version it was updated from.
const autoMarker = "auto-updated"

// takeAutoMarker reads and removes the marker; it returns the version updated from, or "".
func takeAutoMarker(dir string) string {
	p := filepath.Join(dir, autoMarker)
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	_ = os.Remove(p)
	return strings.TrimSpace(string(b))
}

// scheduleUpdateChecks looks for a new version shortly after starting (an open window shows it as soon as it is
// known), then every six hours. A copy started at login may have no network yet, so a failed first check is tried
// again a minute later.
func (a *app) scheduleUpdateChecks() {
	time.AfterFunc(5*time.Second, func() {
		if _, err := a.checkUpdates(false); err != nil {
			time.AfterFunc(time.Minute, func() {
				if _, err := a.checkUpdates(false); err != nil {
					a.log("WARN", "Update check failed: "+err.Error())
				}
			})
		}
	})
	engine.Go(a.log, "The update check", func() {
		for range time.Tick(6 * time.Hour) {
			_, _ = a.checkUpdates(false)
		}
	})
}

// installUpdate downloads the newer version, swaps it in and restarts into it.
func (a *app) installUpdate() error {
	if !a.upd.installing.CompareAndSwap(false, true) {
		return errors.New("An update is already being installed.")
	}
	failed := true
	defer func() {
		if failed {
			a.upd.installing.Store(false)
		}
	}()
	info := a.upd.get()
	if info == nil {
		var err error
		if info, err = a.checkUpdates(true); err != nil {
			return err
		}
	}
	if info == nil || !info.Newer {
		return errors.New("You already have the latest version.")
	}
	a.log("INFO", fmt.Sprintf("Downloading %s...", info.Latest))
	next, err := updater.Download(nil, info, a.exe)
	if err != nil {
		return err
	}
	wasRunning := a.eng.Status().Running
	a.eng.Stop()
	a.log("INFO", "Update downloaded; restarting.")
	_ = removeIPC(a.ipcFile)
	// the new copy is started right away (it waits for this one to let go); we leave a moment later so the settings
	// window still gets its answer
	err = updater.SwapAndRestart(a.exe, next, nil, func() {
		time.AfterFunc(400*time.Millisecond, func() {
			winsys.CloseWindowBrowser(a.browserProfile)
			a.getTray().Close()
			a.lg.close()
			os.Exit(0)
		})
	})
	if err != nil {
		a.log("ERROR", "Update failed: "+err.Error())
		a.writeIPC(a.srv.Port())
		if wasRunning {
			a.eng.Start()
		}
		return err
	}
	failed = false // the new copy takes over: further installs stay refused until this one exits
	return nil
}

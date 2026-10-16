package main

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// updates is what the program knows about newer versions.
type updates struct {
	mu         sync.Mutex
	last       *updater.Info // the last check's answer
	notified   string        // the newest version the user was told about (once per version per run)
	installing atomic.Bool   // an update is being downloaded or swapped in
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
	if manual {
		return r, nil
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

// scheduleUpdateChecks looks for a new version shortly after starting (an open window shows it as soon as it is
// known), then daily. A copy started at login may have no network yet, so a failed first check is tried again a
// minute later.
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
	go func() {
		for range time.Tick(24 * time.Hour) {
			_, _ = a.checkUpdates(false)
		}
	}()
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
	_ = os.Remove(a.ipcFile)
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

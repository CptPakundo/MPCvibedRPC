package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/proc"
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
		// a version that was installed and did not start is not tried by itself again
		return s.CheckUpdates && s.AutoUpdate && i != nil && i.CanInstall && i.Latest != failedVersion(a.st.Dir)
	}
	if !enabled() {
		return false
	}
	if !a.upd.auto.CompareAndSwap(false, true) {
		return true // already waiting
	}
	quiet := func() bool { return quietMoment(a.windows.Load(), a.eng.Status()) }
	engine.Go(a.log, "The automatic update", func() {
		defer a.upd.auto.Store(false)
		for {
			for !quiet() {
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
			err := a.installUpdate(quiet)
			if err == nil {
				return
			}
			_ = os.Remove(marker)
			if errors.Is(err, errNotQuiet) {
				a.log("INFO", "Something started while "+latest+" was downloaded: the update waits for the next quiet moment.")
				continue
			}
			a.log("WARN", "The automatic update to "+latest+" failed: "+err.Error()+" The next check tries again.")
			return
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
	engine.Go(a.log, "The first update check", func() {
		time.Sleep(5 * time.Second)
		if _, err := a.checkUpdates(false); err != nil {
			time.Sleep(time.Minute)
			if _, err := a.checkUpdates(false); err != nil {
				a.log("WARN", "Update check failed: "+err.Error())
			}
		}
	})
	engine.Go(a.log, "The update check", func() {
		for range time.Tick(6 * time.Hour) {
			_, _ = a.checkUpdates(false)
		}
	})
}

// errNotQuiet is installUpdate's answer when the moment stopped being quiet while the update was downloaded.
var errNotQuiet = errors.New("Something is being shown now.")

// installUpdate downloads the newer version, swaps it in and restarts into it. An update that installs itself passes
// quiet, which is asked again once the download is there (it can take minutes on a slow line): a status that went up
// meanwhile, or a window that was opened, is not interrupted (errNotQuiet). An install by hand passes nil.
func (a *app) installUpdate(quiet func() bool) error {
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
	if quiet != nil && !quiet() {
		_ = os.Remove(next)
		return errNotQuiet
	}
	wasRunning := a.eng.Status().Running
	a.eng.Stop()
	a.log("INFO", "Update downloaded; restarting.")
	_ = removeIPC(a.ipcFile)
	// goOn puts this copy back to work when the new one cannot take over
	goOn := func() {
		if !a.quitting.Load() { // a quit that began meanwhile has closed the server and stopped the engine for good
			a.writeIPC(a.srv.Port())
			if wasRunning {
				a.eng.Start()
			}
		}
	}
	if err := updater.Swap(a.exe, next); err != nil {
		a.log("ERROR", "Update failed: "+err.Error())
		goOn()
		return err
	}
	child, err := startCopy(a.exe, "--after-update")
	if err != nil {
		if rerr := updater.Restore(a.exe); rerr != nil {
			a.log("ERROR", "The old version could not be put back: "+rerr.Error())
		}
		err = fmt.Errorf("The new version could not be started: %v", err)
		a.log("ERROR", "Update failed: "+err.Error())
		goOn()
		return err
	}
	failed = false // the new copy takes over: further installs stay refused until this one exits
	engine.Go(a.log, "The hand-over to the new version", func() { a.handOverTo(child, info.Latest, quiet != nil) })
	return nil
}

// startCopy starts the program file exe, detached, and gives its process. A variable: tests start a stand-in.
var startCopy = func(exe string, args ...string) (*os.Process, error) {
	c := proc.Detach(exec.Command(exe, args...))
	if err := c.Start(); err != nil {
		return nil, err
	}
	return c.Process, nil
}

// exitProgram ends this copy (a variable: tests look at the code instead).
var exitProgram = os.Exit

var (
	// handOverDelay is how long the old copy still serves after the new one was started, so the settings window gets
	// the answer to its install request.
	handOverDelay = 400 * time.Millisecond
	// proofWait is how long the new copy has to show that it runs. It starts in a second or two; a virus scanner that
	// reads a program file it has not seen before can add several.
	proofWait = 30 * time.Second
)

// failedMarker is the file in the data folder that names a version that was installed and did not start. That
// version is not installed by itself again (a newer one is); an update that starts removes the file.
const failedMarker = "update-failed"

// failedVersion is the version failedMarker names, or "".
func failedVersion(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, failedMarker))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// handOverTo is the old copy's last task after an update: it gets out of the new copy's way, then waits until that
// copy has shown that it runs (it wrote ipc.json, which it does once its server listens and its tray icon is up) and
// only then leaves. A new version that ends at once, or never gets that far, would otherwise leave the user with a
// program that does not start any more, at this start and at every login. In that case the old program file is put
// back and started again, and the version is remembered so it is not installed by itself a second time.
func (a *app) handOverTo(child *os.Process, latest string, auto bool) {
	time.Sleep(handOverDelay)
	if a.srv != nil {
		a.srv.Close() // the window closes; the port is free for the new copy
	}
	winsys.CloseWindowBrowser(a.browserProfile)
	a.getTray().Close()
	winsys.ReleaseSingleInstance() // the new copy waits for this
	leave := func(code int) {
		if a.lg != nil {
			a.lg.close()
		}
		exitProgram(code)
	}
	gone := make(chan struct{})
	go func() { _, _ = child.Wait(); close(gone) }()
	if copyCameUp(a.ipcFile, child.Pid, gone, proofWait) {
		leave(0)
		return
	}
	a.log("ERROR", "The new version "+latest+" did not start. Going back to "+version+".")
	_ = child.Kill() // it may hang instead of having ended; its file can only be moved once it is gone
	select {
	case <-gone:
	case <-time.After(5 * time.Second):
	}
	if err := updater.Restore(a.exe); err != nil {
		a.log("ERROR", "The old version could not be put back: "+err.Error())
		leave(1)
		return
	}
	_ = os.WriteFile(filepath.Join(a.st.Dir, failedMarker), []byte(latest), 0o644)
	_ = os.Remove(filepath.Join(a.st.Dir, autoMarker))
	args := []string{"--update-failed"}
	if auto {
		args = append(args, "--background") // it updated itself in the background, so it comes back there
	}
	if _, err := startCopy(a.exe, args...); err != nil {
		a.log("ERROR", "The old version is back in place but could not be started: "+err.Error())
		leave(1)
		return
	}
	leave(0)
}

// copyCameUp waits until the copy with process id pid has written ipc.json (true), or has ended or let limit pass
// without doing so (false).
func copyCameUp(ipcFile string, pid int, gone <-chan struct{}, limit time.Duration) bool {
	deadline := time.After(limit)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	up := func() bool {
		raw, err := os.ReadFile(ipcFile)
		var i ipcInfo
		return err == nil && json.Unmarshal(raw, &i) == nil && i.Pid == pid && i.Port != 0
	}
	for {
		if up() {
			return true
		}
		select {
		case <-gone:
			return up() // ended: it cannot come up any more
		case <-deadline:
			return false
		case <-tick.C:
		}
	}
}

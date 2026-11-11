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
	next       time.Time     // when the next check by itself is due (wall clock; zero: now)
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
// "Check for updates automatically" is on. A version found in the background is offered by an open settings window
// at once (its page asks for the state every second); with no window open, and when it will not install itself, it is
// announced next to the tray icon once.
func (a *app) checkUpdates(manual bool) (*updater.Info, error) {
	s := a.st.App()
	if !manual && !(s.CheckUpdates && s.UpdateRepo != "") {
		return nil, nil
	}
	r, err := updater.Check(nil, s.UpdateRepo, version)
	a.upd.checked(time.Now(), err == nil)
	if err != nil {
		return nil, err
	}
	if !r.Newer {
		a.upd.set(nil)
		return r, nil
	}
	a.upd.set(r)
	a.log("INFO", fmt.Sprintf("Update available: %s (you have %s).", r.Latest, version))
	if a.autoInstall() || manual || a.windows.Load() > 0 {
		// installed by itself at a quiet moment (it says so afterwards instead), or the window says it
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

// autoWait is how long an automatic install waits before looking again for a quiet moment.
var autoWait = 2 * time.Minute

// quietMoment: nothing is shown on Discord (nothing plays, or a pause was cleared) and no settings window is open,
// so restarting into a new version interrupts nobody.
func quietMoment(windows int32, s engine.Status) bool {
	return windows == 0 && (s.NowPlaying == nil || s.PauseCleared)
}

// settle is how long after the program started, or a settings window was asked for, no update installs itself. At the
// start the players have not been asked yet, so nothing seems to play; and a window that is opening does not count as
// open until its page has loaded. Without it an update found by the check at the start would restart the program
// under a window that is just coming up, or under a video.
var settle = 10 * time.Second

// busy makes the next settle no quiet moment.
func (a *app) busy() { a.busyUntil.Store(time.Now().Add(settle).UnixNano()) }

// settleLeft is how much of that time is left (zero or less: none).
func (a *app) settleLeft() time.Duration { return time.Until(time.Unix(0, a.busyUntil.Load())) }

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
	quiet := func() bool { return a.settleLeft() <= 0 && quietMoment(a.windows.Load(), a.eng.Status()) }
	engine.Go(a.log, "The automatic update", func() {
		defer a.upd.auto.Store(false)
		for {
			for !quiet() {
				wait := autoWait
				if left := a.settleLeft(); left > 0 && left < wait {
					wait = left // the start is over sooner than the next look would be
				}
				time.Sleep(wait)
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

var (
	// checkEvery is how often the program looks for a newer version while it runs. One small request: GitHub answers
	// "unchanged" to all but the first after a release.
	checkEvery = 6 * time.Hour
	// checkRetry is how long a check that failed (no network) waits for its next try.
	checkRetry = 30 * time.Minute
	// checkLook is how often the program looks at the clock for that. It reads the wall clock rather than counting
	// six hours itself: a timer does not count the same on every system while the computer sleeps, and a laptop that
	// sleeps most of the day would otherwise go days without a check.
	checkLook = 10 * time.Minute
	// firstRetries are the waits before the check at the start is tried again: a copy started at login may have no
	// network yet.
	firstRetries = []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute}
)

// checked books a check that went out at now: the next one by itself is due checkEvery later, or checkRetry later
// when it failed.
func (u *updates) checked(now time.Time, ok bool) {
	wait := checkEvery
	if !ok {
		wait = checkRetry
	}
	u.mu.Lock()
	u.next = now.Round(0).Add(wait) // Round(0): the wall clock alone
	u.mu.Unlock()
}

// due says whether a check by itself is due at now: none went out yet, its time has come, or the clock was set far
// back (the time booked is then further away than a wait can be; the two hours leave the end of summer time alone).
func (u *updates) due(now time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	now = now.Round(0)
	return !now.Before(u.next) || u.next.Sub(now) > checkEvery+2*time.Hour
}

// scheduleUpdateChecks looks for a new version as the program starts (an open window offers it as soon as it is
// known), then every six hours while it runs. The check at the start is tried again a few times when it fails.
func (a *app) scheduleUpdateChecks() {
	engine.Go(a.log, "The update check", func() {
		if err := tryCheck(func() error { _, err := a.checkUpdates(false); return err }, firstRetries, time.Sleep); err != nil {
			a.log("WARN", "Update check failed: "+err.Error())
		}
		for range time.Tick(checkLook) {
			if a.upd.due(time.Now()) {
				_, _ = a.checkUpdates(false)
			}
		}
	})
}

// tryCheck runs check, and again after each of waits while it fails; it gives the last error.
func tryCheck(check func() error, waits []time.Duration, sleep func(time.Duration)) error {
	err := check()
	for _, w := range waits {
		if err == nil {
			break
		}
		sleep(w)
		err = check()
	}
	return err
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

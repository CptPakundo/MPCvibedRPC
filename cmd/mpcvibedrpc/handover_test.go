package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
)

// TestCopyStandIn is not a test of its own: started with MPCRPC_TEST_COPY set, the test program plays a new copy of
// the program after an update. "up" writes ipc.json as a copy that came up does, "exit" ends at once, "hang" neither
// ends nor comes up.
func TestCopyStandIn(t *testing.T) {
	switch os.Getenv("MPCRPC_TEST_COPY") {
	case "exit":
		os.Exit(3)
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "up":
		os.WriteFile(os.Getenv("MPCRPC_TEST_IPC"), []byte(fmt.Sprintf(`{"port":1,"token":"x","pid":%d}`, os.Getpid())), 0o600)
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

// standIn starts the test program as a new copy in the given mode.
func standIn(t *testing.T, mode, ipcFile string) *os.Process {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=^TestCopyStandIn$")
	c.Env = append(os.Environ(), "MPCRPC_TEST_COPY="+mode, "MPCRPC_TEST_IPC="+ipcFile)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Process.Kill() })
	return c.Process
}

// handOverApp is an app after a swap: the new program file is in place, the old one is kept as .old.
func handOverApp(t *testing.T) (a *app, logs func() string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "home"))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "app.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	os.WriteFile(exe+".next", []byte("new"), 0o755)
	if err := updater.Swap(exe, exe+".next"); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var lines []string
	lg := func(l, m string) { mu.Lock(); lines = append(lines, l+" "+m); mu.Unlock() }
	a = &app{st: st, log: lg, exe: exe, ipcFile: st.IPCPath()}
	return a, func() string { mu.Lock(); defer mu.Unlock(); return strings.Join(lines, "\n") }
}

// runHandOver runs handOverTo with the program's exit and its restart caught: it gives the exit code and the arguments
// the old program file was started again with (nil: it was not).
func runHandOver(t *testing.T, a *app, child *os.Process, auto bool, wait time.Duration) (code int, restarted []string) {
	t.Helper()
	defer func(s func(string, ...string) (*os.Process, error), e func(int), p, d time.Duration) {
		startCopy, exitProgram, proofWait, handOverDelay = s, e, p, d
	}(startCopy, exitProgram, proofWait, handOverDelay)
	proofWait, handOverDelay = wait, 0
	code = -1
	exitProgram = func(c int) { code = c }
	startCopy = func(exe string, args ...string) (*os.Process, error) {
		if exe != a.exe {
			t.Errorf("started %s, want the program's own file", exe)
		}
		restarted = append([]string{}, args...)
		return os.FindProcess(os.Getpid())
	}
	a.handOverTo(child, "9.9.9", auto)
	return code, restarted
}

func fileText(path string) string { b, _ := os.ReadFile(path); return string(b) }

// The new copy came up: the old one leaves, and nothing is put back.
func TestHandOverLeavesOnceTheNewCopyIsUp(t *testing.T) {
	a, logs := handOverApp(t)
	code, restarted := runHandOver(t, a, standIn(t, "up", a.ipcFile), false, 20*time.Second)
	if code != 0 || restarted != nil {
		t.Fatalf("exit code %d, restarted with %v\n%s", code, restarted, logs())
	}
	if fileText(a.exe) != "new" || failedVersion(a.st.Dir) != "" {
		t.Errorf("the new version must stay: program file %q, failed marker %q", fileText(a.exe), failedVersion(a.st.Dir))
	}
}

// The new copy ends at once: the old program file is back and started again, and the version is remembered.
func TestHandOverPutsTheOldVersionBackWhenTheNewOneEnds(t *testing.T) {
	a, logs := handOverApp(t)
	os.WriteFile(filepath.Join(a.st.Dir, autoMarker), []byte("0.9.0"), 0o644)
	code, restarted := runHandOver(t, a, standIn(t, "exit", a.ipcFile), true, 20*time.Second)
	if code != 0 || strings.Join(restarted, " ") != "--update-failed --background" {
		t.Fatalf("exit code %d, restarted with %v\n%s", code, restarted, logs())
	}
	if fileText(a.exe) != "old" || fileText(a.exe+".new") != "new" {
		t.Errorf("the old program file must be back: %q, set aside: %q", fileText(a.exe), fileText(a.exe+".new"))
	}
	if failedVersion(a.st.Dir) != "9.9.9" || !strings.Contains(logs(), "9.9.9 did not start") {
		t.Errorf("failed marker %q\n%s", failedVersion(a.st.Dir), logs())
	}
	if _, err := os.Stat(filepath.Join(a.st.Dir, autoMarker)); err == nil {
		t.Error("the marker of the automatic update must go: the copy that comes back did not update itself")
	}
}

// The new copy neither ends nor comes up: after the wait it is ended and the old version is back. An update by hand
// comes back with its window (no --background).
func TestHandOverPutsTheOldVersionBackWhenTheNewOneHangs(t *testing.T) {
	a, logs := handOverApp(t)
	code, restarted := runHandOver(t, a, standIn(t, "hang", a.ipcFile), false, 400*time.Millisecond)
	if code != 0 || strings.Join(restarted, " ") != "--update-failed" {
		t.Fatalf("exit code %d, restarted with %v\n%s", code, restarted, logs())
	}
	if fileText(a.exe) != "old" || failedVersion(a.st.Dir) != "9.9.9" {
		t.Errorf("program file %q, failed marker %q", fileText(a.exe), failedVersion(a.st.Dir))
	}
}

// A version that did not start is not installed by itself again; a newer one is.
func TestAutoInstallSkipsAVersionThatDidNotStart(t *testing.T) {
	a, _ := autoApp(t)
	os.WriteFile(filepath.Join(a.st.Dir, failedMarker), []byte("9.9.9\n"), 0o644)
	a.upd.set(&updater.Info{Newer: true, CanInstall: true, Latest: "9.9.9"})
	if a.autoInstall() {
		t.Fatal("9.9.9 did not start the last time: it must not install itself again")
	}
	defer func(w time.Duration) { autoWait = w }(autoWait)
	autoWait = 10 * time.Millisecond
	a.upd.set(&updater.Info{Newer: true, CanInstall: true, Latest: "9.9.10"})
	if !a.autoInstall() {
		t.Fatal("a newer version must be tried")
	}
	for i := 0; a.upd.auto.Load(); i++ { // it fails (the release has no file) and gives up; wait for that
		if i > 300 {
			t.Fatal("did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
)

// An automatic update only restarts the program when that interrupts nobody: nothing shown on Discord (a cleared
// pause counts as nothing) and no settings window open.
func TestQuietMoment(t *testing.T) {
	title := "Sample Show - S01E02"
	for _, c := range []struct {
		windows int32
		s       engine.Status
		want    bool
	}{
		{0, engine.Status{}, true},
		{0, engine.Status{NowPlaying: &title}, false},
		{0, engine.Status{NowPlaying: &title, Paused: true}, false},
		{0, engine.Status{NowPlaying: &title, Paused: true, PauseCleared: true}, true},
		{1, engine.Status{}, false},
	} {
		if got := quietMoment(c.windows, c.s); got != c.want {
			t.Errorf("windows %d, %+v: %v, want %v", c.windows, c.s, got, c.want)
		}
	}
}

// The marker an automatic update leaves is read once.
func TestAutoMarker(t *testing.T) {
	dir := t.TempDir()
	if from := takeAutoMarker(dir); from != "" {
		t.Fatalf("no marker: %q", from)
	}
	os.WriteFile(filepath.Join(dir, autoMarker), []byte("0.9.1\n"), 0o644)
	if from := takeAutoMarker(dir); from != "0.9.1" {
		t.Fatalf("marker: %q", from)
	}
	if from := takeAutoMarker(dir); from != "" {
		t.Fatalf("read twice: %q", from)
	}
}

func autoApp(t *testing.T) (*app, func() string) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var logs []string
	lg := func(l, m string) { mu.Lock(); logs = append(logs, l+" "+m); mu.Unlock() }
	a := &app{st: st, log: lg, eng: engine.New(st.Config(), engine.Options{Log: lg})}
	return a, func() string { mu.Lock(); defer mu.Unlock(); return strings.Join(logs, "\n") }
}

func setApp(t *testing.T, st *store.Store, js string) {
	t.Helper()
	p, _ := jsonx.Parse(`{"app":` + js + `}`)
	if err := st.Update(p.(*jsonx.Obj)); err != nil {
		t.Fatal(err)
	}
}

// Installing by itself needs both switches (on by default) and an update the program can install (not on macOS);
// it waits while a window is open, and gives up when switched off meanwhile.
func TestAutoInstallWaitsAndStops(t *testing.T) {
	defer func(w time.Duration) { autoWait = w }(autoWait)
	autoWait = 10 * time.Millisecond
	a, _ := autoApp(t)
	if a.autoInstall() {
		t.Fatal("no update known: nothing to install")
	}
	a.upd.set(&updater.Info{Newer: true, Latest: "9.9.9"}) // a release the program can't install (macOS)
	if a.autoInstall() {
		t.Fatal("not installable")
	}
	a.upd.set(&updater.Info{Newer: true, CanInstall: true, Latest: "9.9.9"})
	setApp(t, a.st, `{"autoUpdate":false}`)
	if a.autoInstall() {
		t.Fatal("switched off")
	}
	setApp(t, a.st, `{"autoUpdate":true,"checkUpdates":false}`)
	if a.autoInstall() {
		t.Fatal("checks off")
	}
	setApp(t, a.st, `{"checkUpdates":true}`)
	a.windows.Store(1) // a settings window is open: not now
	if !a.autoInstall() || !a.upd.auto.Load() {
		t.Fatal("should wait for a quiet moment")
	}
	if !a.autoInstall() {
		t.Fatal("a second check while waiting says it is in hand")
	}
	time.Sleep(50 * time.Millisecond)
	if a.upd.installing.Load() {
		t.Fatal("installed while the window was open")
	}
	setApp(t, a.st, `{"autoUpdate":false}`)
	for i := 0; a.upd.auto.Load(); i++ {
		if i > 200 {
			t.Fatal("still waiting after it was switched off")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(a.st.Dir, autoMarker)); err == nil {
		t.Fatal("a marker was left without an install")
	}
}

// At a quiet moment it installs; a failed install (here: the release has no file) leaves no marker behind and is
// left to the next check.
func TestAutoInstallAtQuietMoment(t *testing.T) {
	defer func(w time.Duration) { autoWait = w }(autoWait)
	autoWait = 10 * time.Millisecond
	a, logs := autoApp(t)
	a.upd.set(&updater.Info{Newer: true, CanInstall: true, Latest: "9.9.9"})
	if !a.autoInstall() {
		t.Fatal("should install")
	}
	for i := 0; a.upd.auto.Load(); i++ {
		if i > 300 {
			t.Fatal("did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	l := logs()
	if !strings.Contains(l, "Installing 9.9.9 by itself") || !strings.Contains(l, "The automatic update to 9.9.9 failed") {
		t.Fatal(l)
	}
	if _, err := os.Stat(filepath.Join(a.st.Dir, autoMarker)); err == nil {
		t.Fatal("marker left after a failed install")
	}
}

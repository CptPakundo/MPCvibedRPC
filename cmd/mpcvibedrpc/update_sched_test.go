package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
)

// The check by itself follows the wall clock: due at once when none went out yet, six hours after one that
// answered, half an hour after one that failed, and at once when the clock was set far back.
func TestCheckIsDueByTheWallClock(t *testing.T) {
	var u updates
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	if !u.due(now) {
		t.Fatal("no check went out yet: due")
	}
	u.checked(now, true)
	for _, c := range []struct {
		after time.Duration
		want  bool
	}{
		{time.Minute, false},
		{checkEvery - time.Minute, false},
		{checkEvery, true},
		{48 * time.Hour, true},            // the computer slept for two days
		{-time.Hour, false},               // summer time ends, or a small correction
		{-(checkEvery + time.Hour), true}, // the clock was set back by more than a wait
	} {
		if got := u.due(now.Add(c.after)); got != c.want {
			t.Errorf("%v after a check that answered: due %v, want %v", c.after, got, c.want)
		}
	}
	u.checked(now, false)
	if u.due(now.Add(checkRetry-time.Minute)) || !u.due(now.Add(checkRetry)) {
		t.Fatal("a check that failed is tried again after checkRetry, not sooner")
	}
}

// The check at the start is tried again after each wait while it fails, and no more once it answers.
func TestFirstCheckIsTriedAgain(t *testing.T) {
	waits := []time.Duration{time.Second, time.Minute}
	for _, c := range []struct {
		fails     int // how many checks fail before one answers
		wantCalls int
		wantSlept time.Duration
		wantErr   bool
	}{
		{0, 1, 0, false},
		{1, 2, time.Second, false},
		{2, 3, time.Second + time.Minute, false},
		{9, 3, time.Second + time.Minute, true},
	} {
		calls, slept := 0, time.Duration(0)
		err := tryCheck(func() error {
			calls++
			if calls <= c.fails {
				return errors.New("no network")
			}
			return nil
		}, waits, func(d time.Duration) { slept += d })
		if calls != c.wantCalls || slept != c.wantSlept || (err != nil) != c.wantErr {
			t.Errorf("%d failing: %d checks, slept %v, error %v", c.fails, calls, slept, err)
		}
	}
}

// An update found as the program starts does not install itself in the first moments: the players have not been
// asked yet and a window may be opening. It installs once that time is over (here it then fails: the release has
// no file).
func TestAutoInstallWaitsForTheStartToSettle(t *testing.T) {
	defer func(w, s time.Duration) { autoWait, settle = w, s }(autoWait, settle)
	autoWait, settle = time.Hour, 300*time.Millisecond // the look after the start must not wait for autoWait
	a, logs := autoApp(t)
	a.busy()
	a.upd.set(&updater.Info{Newer: true, CanInstall: true, Latest: "9.9.9"})
	if !a.autoInstall() {
		t.Fatal("should wait and install")
	}
	time.Sleep(100 * time.Millisecond)
	if a.upd.installing.Load() || logs() != "" {
		t.Fatalf("installed while the start was settling: %s", logs())
	}
	for i := 0; a.upd.auto.Load(); i++ {
		if i > 300 {
			t.Fatal("did not install after the start had settled")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if l := logs(); !strings.Contains(l, "Installing 9.9.9 by itself") {
		t.Fatal(l)
	}
}

// A window that was just asked for counts as open before its page has loaded.
func TestOpeningAWindowIsNoQuietMoment(t *testing.T) {
	defer func(s time.Duration) { settle = s }(settle)
	settle = time.Hour
	a, _ := autoApp(t)
	if a.settleLeft() > 0 {
		t.Fatal("nothing asked for a window yet")
	}
	a.busy() // what openWindow does first
	if left := a.settleLeft(); left < 59*time.Minute {
		t.Fatalf("settle left %v", left)
	}
}

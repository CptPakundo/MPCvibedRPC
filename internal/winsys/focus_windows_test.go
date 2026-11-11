//go:build windows

package winsys

import "testing"

func TestPickWindow(t *testing.T) {
	pids := map[uint32]bool{10: true, 11: true}
	wins := []topWindow{
		{Handle: 1, Pid: 99, Visible: true, Titled: true},  // somebody else's browser
		{Handle: 2, Pid: 10, Visible: false, Titled: true}, // hidden helper
		{Handle: 3, Pid: 10, Visible: true, Titled: false}, // no title
		{Handle: 4, Pid: 11, Visible: true, Titled: true},  // the window
		{Handle: 5, Pid: 10, Visible: true, Titled: true},
	}
	w, ok := pickWindow(wins, pids)
	if !ok || w.Handle != 4 {
		t.Fatalf("got %+v %v, want window 4", w, ok)
	}
	if _, ok := pickWindow(wins[:3], pids); ok {
		t.Fatal("hidden and untitled windows must not be taken")
	}
	if _, ok := pickWindow(nil, pids); ok {
		t.Fatal("no windows, no result")
	}
	w, ok = pickWindow([]topWindow{{Handle: 7, Pid: 10, Visible: true, Titled: true, Minimised: true}}, pids)
	if !ok || !w.Minimised {
		t.Fatalf("a minimised window is still found and reported as minimised: %+v %v", w, ok)
	}
}

// The enumeration runs for real (it only reads): the callback must work with Windows' calling convention, and a
// folder no browser uses as its profile has no window to bring forward.
func TestEnumeratingWindowsWorks(t *testing.T) {
	enumMu.Lock()
	enumFound = nil
	r, _, _ := pEnumWindows.Call(enumWindowsCallback, 0)
	n := len(enumFound)
	enumFound = nil
	enumMu.Unlock()
	if r == 0 {
		t.Fatal("EnumWindows failed")
	}
	t.Logf("%d top-level windows", n)
	if FocusWindow(t.TempDir()) {
		t.Error("no browser uses a new folder as its profile: there is nothing to bring forward")
	}
}

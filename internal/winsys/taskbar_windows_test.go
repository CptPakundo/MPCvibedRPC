package winsys

import "testing"

func TestWindowIdentity(t *testing.T) {
	const exe = `C:\Programs\Sample\MPRP.exe`
	id, name, command, icon := windowIdentity(exe, "")
	if id != "MPRP" || name != "MPRP" || command != `"`+exe+`"` || icon != exe+",0" {
		t.Errorf("the installed copy: id %q, name %q, command %q, icon %q", id, name, command, icon)
	}
	// a copy with a data folder of its own: another button group, and a pinned button starts it with that folder
	id2, name2, command2, _ := windowIdentity(exe, `D:\Portable\data\`)
	if id2 == id || id2 != runNameFor(`D:\Portable\data`) {
		t.Errorf("a copy with its own folder has the ID %q (the installed one: %q)", id2, id)
	}
	if name2 != "MPRP" || command2 != `"`+exe+`" --home "D:\Portable\data"` {
		t.Errorf("a copy with its own folder: name %q, command %q", name2, command2)
	}
}

// A window that is not there is not named, and nothing is waited for without a profile.
func TestNameWindowWithoutAWindow(t *testing.T) {
	if NameWindow("") {
		t.Error("a window was named without a profile")
	}
	if setWindowStrings(0, []windowString{{propAppID, "Sample"}}) {
		t.Error("properties were set on no window")
	}
}

// Only events about the window itself count: shown or retitled, the icon is set again; destroyed, the keeping ends.
func TestIconEvent(t *testing.T) {
	const w = uintptr(0x1234)
	for _, c := range []struct {
		event        uint32
		hwnd         uintptr
		obj, child   int32
		reapply, end bool
		why          string
	}{
		{eventObjectShow, w, objIDWindow, childIDSelf, true, false, "shown"},
		{eventObjectNameChange, w, objIDWindow, childIDSelf, true, false, "retitled"},
		{eventObjectDestroy, w, objIDWindow, childIDSelf, false, true, "destroyed"},
		{eventObjectShow, 0x9999, objIDWindow, childIDSelf, false, false, "another window of the browser"},
		{eventObjectDestroy, 0x9999, objIDWindow, childIDSelf, false, false, "another window of the browser goes"},
		{eventObjectNameChange, w, -4, childIDSelf, false, false, "its contents (OBJID_CLIENT)"},
		{eventObjectShow, w, objIDWindow, 3, false, false, "a child element"},
		{0x800B, w, objIDWindow, childIDSelf, false, false, "moved (not asked for)"},
	} {
		if r, e := iconEvent(c.event, c.hwnd, w, c.obj, c.child); r != c.reapply || e != c.end {
			t.Errorf("%s: reapply %v end %v, want %v %v", c.why, r, e, c.reapply, c.end)
		}
	}
}

// One window is looked after at a time: a new one, or StopWindowIcon, ends the one before; the hook's callback
// reaches the current one, with idObject as Windows passes it (a LONG in a pointer-sized argument).
func TestIconKeeperLifecycle(t *testing.T) {
	type fake struct{ applied, ended int }
	keeper := func(hwnd uintptr, f *fake) *iconKeeper {
		return &iconKeeper{hwnd: hwnd, apply: func(h uintptr) {
			if h == hwnd {
				f.applied++
			}
		}, end: func() { f.ended++ }}
	}
	StopWindowIcon()
	gen := iconGen.Load()
	var f1, f2 fake
	k1, k2 := keeper(0x10, &f1), keeper(0x20, &f2)
	if !startKeeper(k1, gen) {
		t.Fatal("the first window was not looked after")
	}
	onWinEvent(0, eventObjectShow, 0x10, 0, 0, 0, 0)
	onWinEvent(0, eventObjectNameChange, 0x10, uintptr(0xFFFFFFFC), 0, 0, 0) // OBJID_CLIENT: not the window
	if f1.applied != 1 || f1.ended != 0 {
		t.Errorf("first window: %+v", f1)
	}
	if !startKeeper(k2, gen) || f1.ended != 1 {
		t.Errorf("a new window did not end the one before: %+v", f1)
	}
	onWinEvent(0, eventObjectNameChange, 0x10, 0, 0, 0, 0) // the first window's events no longer count
	onWinEvent(0, eventObjectNameChange, 0x20, 0, 0, 0, 0)
	if f1.applied != 1 || f2.applied != 1 {
		t.Errorf("events went to %+v %+v", f1, f2)
	}
	onWinEvent(0, eventObjectDestroy, 0x20, 0, 0, 0, 0)
	onWinEvent(0, eventObjectDestroy, 0x20, 0, 0, 0, 0)
	if f2.ended != 1 {
		t.Errorf("the destroyed window's keeping ended %d times, want once", f2.ended)
	}
	endKeeper(k2)
	var f3 fake
	k3 := keeper(0x30, &f3)
	if !startKeeper(k3, gen) {
		t.Fatal("a third window was not looked after")
	}
	StopWindowIcon()
	if f3.ended != 1 {
		t.Error("StopWindowIcon did not end the keeping")
	}
	if startKeeper(keeper(0x40, &fake{}), gen) {
		t.Error("a window found before StopWindowIcon was looked after")
	}
	onWinEvent(0, eventObjectShow, 0x30, 0, 0, 0, 0)
	if f3.applied != 0 {
		t.Error("an event came through after StopWindowIcon")
	}
}

// A redrawn taskbar button (the browser set an icon of its own: no WinEvent tells that) has the window's icon checked;
// a destroyed window ends the keeping. The shell window's procedure passes them to the current window.
func TestShellEvent(t *testing.T) {
	const w = uintptr(0x1234)
	for _, c := range []struct {
		code, hwnd uintptr
		check, end bool
		why        string
	}{
		{hshellRedraw, w, true, false, "redrawn"},
		{hshellRedraw | 0x8000, w, true, false, "flashing (HSHELL_FLASH)"},
		{hshellWindowDestroyed, w, false, true, "destroyed"},
		{hshellRedraw, 0x9999, false, false, "another window"},
		{1, w, false, false, "created (not asked for)"},
	} {
		if ch, e := shellEvent(c.code, c.hwnd, w); ch != c.check || e != c.end {
			t.Errorf("%s: check %v end %v, want %v %v", c.why, ch, e, c.check, c.end)
		}
	}
	StopWindowIcon()
	applied, ended := 0, 0
	k := &iconKeeper{hwnd: w, apply: func(uintptr) { applied++ }, end: func() { ended++ }}
	if !startKeeper(k, iconGen.Load()) {
		t.Fatal("not looked after")
	}
	defer StopWindowIcon()
	m := uint32(shellHookMessage())
	if m == 0 {
		t.Fatal("no SHELLHOOK message")
	}
	shellWindowProc(0, m, hshellRedraw, w)
	shellWindowProc(0, m, hshellWindowDestroyed, w)
	if applied != 1 || ended != 1 {
		t.Errorf("applied %d, ended %d; want 1 and 1", applied, ended)
	}
}

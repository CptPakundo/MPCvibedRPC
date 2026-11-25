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

// Only events about a window itself count: shown or retitled, it is checked; destroyed, it is forgotten.
func TestIconEvent(t *testing.T) {
	for _, c := range []struct {
		event      uint32
		obj, child int32
		want       keepAction
		why        string
	}{
		{eventObjectShow, objIDWindow, childIDSelf, keepCheck, "shown"},
		{eventObjectNameChange, objIDWindow, childIDSelf, keepCheck, "retitled"},
		{eventObjectDestroy, objIDWindow, childIDSelf, keepForget, "destroyed"},
		{eventObjectNameChange, -4, childIDSelf, keepNothing, "its contents (OBJID_CLIENT)"},
		{eventObjectShow, objIDWindow, 3, keepNothing, "a child element"},
		{eventObjectDestroy, objIDWindow, 3, keepNothing, "a child element goes"},
		{0x800B, objIDWindow, childIDSelf, keepNothing, "moved (not asked for)"},
	} {
		if got := iconEvent(c.event, c.obj, c.child); got != c.want {
			t.Errorf("%s: %v, want %v", c.why, got, c.want)
		}
	}
}

// A window that got a taskbar button, or whose button was redrawn (the browser set an icon of its own: no WinEvent
// tells that), is checked; a destroyed one is forgotten.
func TestShellEvent(t *testing.T) {
	for _, c := range []struct {
		code uintptr
		want keepAction
		why  string
	}{
		{hshellRedraw, keepCheck, "redrawn"},
		{hshellRedraw | 0x8000, keepCheck, "flashing (HSHELL_FLASH)"},
		{hshellWindowCreated, keepCheck, "created"},
		{hshellWindowDestroyed, keepForget, "destroyed"},
		{4, keepNothing, "activated (not asked for)"},
	} {
		if got := shellEvent(c.code); got != c.want {
			t.Errorf("%s: %v, want %v", c.why, got, c.want)
		}
	}
}

// Only windows with a taskbar button are named: not the browser's bubbles, menus, hidden helpers or child windows.
func TestTaskbarWindow(t *testing.T) {
	for _, c := range []struct {
		w    topWindow
		want bool
		why  string
	}{
		{topWindow{Visible: true}, true, "a plain window"},
		{topWindow{}, false, "hidden"},
		{topWindow{Visible: true, Owned: true}, false, "owned (a bubble)"},
		{topWindow{Visible: true, Tool: true}, false, "a tool window (a menu)"},
		{topWindow{Visible: true, Owned: true, AppWindow: true}, true, "owned, but asks for a button"},
		{topWindow{Visible: true, Child: true}, false, "a child window"},
	} {
		if got := taskbarWindow(c.w); got != c.want {
			t.Errorf("%s: %v, want %v", c.why, got, c.want)
		}
	}
	// a bubble over the settings window is not taken for it, nor is another program's window above both
	pids := map[uint32]bool{10: true}
	wins := []topWindow{
		{Handle: 1, Pid: 99, Visible: true, Titled: true},
		{Handle: 2, Pid: 10, Visible: true, Titled: true, Owned: true},
		{Handle: 3, Pid: 10, Visible: true},
		{Handle: 4, Pid: 10, Visible: true, Titled: true},
	}
	if w, ok := pickTaskbarWindow(wins, pids); !ok || w.Handle != 4 {
		t.Errorf("picked %+v %v, want the window 4", w, ok)
	}
	if _, ok := pickTaskbarWindow(wins[:3], pids); ok {
		t.Error("a window was picked where the browser has only a bubble and an untitled window")
	}
}

// fakeKeeper is a keeping of the browser process 10 whose windows are described by wins.
type fakeKeeper struct {
	wins           map[uintptr]topWindow
	named, checked map[uintptr]int
	ended          int
}

func newFakeKeeper(first uintptr) (*fakeKeeper, *iconKeeper) {
	f := &fakeKeeper{wins: map[uintptr]topWindow{}, named: map[uintptr]int{}, checked: map[uintptr]int{}}
	k := &iconKeeper{pid: 10,
		inspect: func(h uintptr) topWindow { return f.wins[h] },
		name:    func(h uintptr) { f.named[h]++ },
		check:   func(h uintptr) { f.checked[h]++ },
		end:     func() { f.ended++ },
		named:   map[uintptr]bool{first: true}}
	f.wins[first] = topWindow{Handle: first, Pid: 10, Visible: true, Titled: true}
	return f, k
}

// A new window of the browser process (the browser replaced the first one) is named once and then has its icon kept;
// a window of another process, and the browser's bubble, are left alone. The first window's going does not end the
// keeping, and a handle given to a window again is named again.
func TestIconKeeperAdoptsNewWindows(t *testing.T) {
	f, k := newFakeKeeper(0x10)
	f.wins[0x20] = topWindow{Handle: 0x20, Pid: 10, Visible: true}
	f.wins[0x30] = topWindow{Handle: 0x30, Pid: 99, Visible: true, Titled: true}
	f.wins[0x40] = topWindow{Handle: 0x40, Pid: 10, Visible: true, Owned: true}

	k.shell(hshellRedraw, 0x10) // the first window: named already, its icon is checked
	if f.named[0x10] != 0 || f.checked[0x10] != 1 {
		t.Errorf("the first window: named %d, checked %d", f.named[0x10], f.checked[0x10])
	}
	k.event(eventObjectDestroy, 0x10, objIDWindow, childIDSelf)
	delete(f.wins, 0x10)
	k.shell(hshellWindowCreated, 0x20) // its replacement
	k.event(eventObjectShow, 0x20, objIDWindow, childIDSelf)
	k.event(eventObjectNameChange, 0x20, objIDWindow, childIDSelf)
	if f.named[0x20] != 1 || f.checked[0x20] != 2 {
		t.Errorf("the new window: named %d (want 1), checked %d (want 2)", f.named[0x20], f.checked[0x20])
	}
	if !k.adopted {
		t.Error("taking on a window did not start a while of looking")
	}
	k.shell(hshellWindowCreated, 0x30)
	k.shell(hshellRedraw, 0x30)
	k.event(eventObjectShow, 0x40, objIDWindow, childIDSelf)
	if f.named[0x30]+f.checked[0x30]+f.named[0x40]+f.checked[0x40] != 0 {
		t.Errorf("another program's window or a bubble was touched: %v %v", f.named, f.checked)
	}
	if f.ended != 0 {
		t.Error("the first window's going ended the keeping")
	}
	// the handle 0x20 goes and comes back for another window of the browser
	k.shell(hshellWindowDestroyed, 0x20)
	k.shell(hshellWindowCreated, 0x20)
	if f.named[0x20] != 2 {
		t.Errorf("a reused handle was named %d times, want 2", f.named[0x20])
	}
	// a look at all the windows (the hooks' first moments): only the browser's taskbar windows
	f2, k2 := newFakeKeeper(0x10)
	k2.sweep([]topWindow{f.wins[0x30], f.wins[0x40], {Handle: 0x50, Pid: 10, Visible: true}, {Handle: 0x10, Pid: 10, Visible: true}, {Handle: 0x60, Pid: 10}})
	if f2.named[0x50] != 1 || f2.checked[0x10] != 1 || len(f2.named) != 1 || len(f2.checked) != 1 {
		t.Errorf("the look named %v and checked %v", f2.named, f2.checked)
	}
}

// When the browser's end cannot be waited for, the keeping ends once the process has no window left.
func TestIconKeeperEndsWithTheLastWindow(t *testing.T) {
	f, k := newFakeKeeper(0x10)
	left := []topWindow{{Handle: 0x10, Pid: 10}, {Handle: 0x77, Pid: 99}}
	k.alive = func() bool { return hasWindow(left, 10) }
	k.shell(hshellWindowDestroyed, 0x55) // a window never named: nothing is looked up
	k.shell(hshellWindowDestroyed, 0x10)
	if f.ended != 0 {
		t.Error("ended while the process still has a window")
	}
	k.named[0x10] = true
	left = left[1:]
	k.event(eventObjectDestroy, 0x10, objIDWindow, childIDSelf)
	if f.ended != 1 {
		t.Error("did not end with the process's last window")
	}
}

// One keeping at a time: a new one, or StopWindowIcon, ends the one before; the hook's callback and the shell window's
// procedure reach the current one, with idObject as Windows passes it (a LONG in a pointer-sized argument).
func TestIconKeeperLifecycle(t *testing.T) {
	StopWindowIcon()
	gen := iconGen.Load()
	f1, k1 := newFakeKeeper(0x10)
	f2, k2 := newFakeKeeper(0x20)
	if !startKeeper(k1, gen) {
		t.Fatal("the first keeping did not start")
	}
	onWinEvent(0, eventObjectShow, 0x10, 0, 0, 0, 0)
	onWinEvent(0, eventObjectNameChange, 0x10, uintptr(0xFFFFFFFC), 0, 0, 0) // OBJID_CLIENT: not the window
	if f1.checked[0x10] != 1 || f1.ended != 0 {
		t.Errorf("first keeping: %+v", f1)
	}
	if !startKeeper(k2, gen) || f1.ended != 1 {
		t.Errorf("a new keeping did not end the one before: %+v", f1)
	}
	onWinEvent(0, eventObjectNameChange, 0x20, 0, 0, 0, 0)
	if f1.checked[0x10] != 1 || f2.checked[0x20] != 1 {
		t.Errorf("events went to %+v %+v", f1, f2)
	}
	m := uint32(shellHookMessage())
	if m == 0 {
		t.Fatal("no SHELLHOOK message")
	}
	shellWindowProc(0, m, hshellRedraw, 0x20)
	if f2.checked[0x20] != 2 {
		t.Errorf("the shell hook's redraw did not reach the keeping: %+v", f2)
	}
	k2.stop()
	k2.stop()
	if f2.ended != 1 {
		t.Errorf("the keeping ended %d times, want once", f2.ended)
	}
	endKeeper(k2)
	f3, k3 := newFakeKeeper(0x30)
	if !startKeeper(k3, gen) {
		t.Fatal("a third keeping did not start")
	}
	StopWindowIcon()
	if f3.ended != 1 {
		t.Error("StopWindowIcon did not end the keeping")
	}
	if _, k4 := newFakeKeeper(0x40); startKeeper(k4, gen) {
		t.Error("a window found before StopWindowIcon was looked after")
	}
	onWinEvent(0, eventObjectShow, 0x30, 0, 0, 0, 0)
	if f3.checked[0x30] != 0 {
		t.Error("an event came through after StopWindowIcon")
	}
}

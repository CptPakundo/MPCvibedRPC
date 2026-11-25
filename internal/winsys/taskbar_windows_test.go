package winsys

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPRP/internal/assets"
)


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

// Nothing is kept without a profile, and no properties are set on no window.
func TestNameWindowWithoutAWindow(t *testing.T) {
	if NameWindow("", nil) {
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
}

// All windows are looked at often in the first seconds after a window was asked for, less often until it appears,
// and for a while after one was taken on; a window that does not appear is reported after 30 s, once, and then only
// listened for; otherwise the keeping only waits for events.
func TestKeepWait(t *testing.T) {
	now := time.Now()
	var zero time.Time
	for _, c := range []struct {
		asked, settleEnd time.Time
		late             bool
		wait             time.Duration
		overdue          bool
		why              string
	}{
		{zero, zero, false, -1, false, "nothing awaited"},
		{now.Add(-time.Second), zero, false, nameWindowFastEvery, false, "just asked for"},
		{now.Add(-5 * time.Second), zero, false, nameWindowEvery, false, "asked for a while ago"},
		{now.Add(-nameWindowWarn), zero, false, 0, true, "not there after 30 s"},
		{now.Add(-nameWindowWarn), zero, true, -1, false, "not there after 30 s, reported: only listened for"},
		{zero, now.Add(5 * time.Second), false, keepSettleEvery, false, "just taken on"},
		{zero, now.Add(100 * time.Millisecond), false, 100 * time.Millisecond, false, "the settling ends sooner"},
		{zero, now.Add(-time.Second), false, -1, false, "settled"},
		{now.Add(-time.Second), now.Add(5 * time.Second), false, nameWindowFastEvery, false, "the shorter of both"},
	} {
		w, o := keepWait(now, c.asked, c.settleEnd, c.late)
		if w != c.wait || o != c.overdue {
			t.Errorf("%s: wait %v overdue %v, want %v %v", c.why, w, o, c.wait, c.overdue)
		}
	}
}

// The keeping listens while a window is taken on, for 2 minutes after one was asked for, and for 30 s after the last
// one went; otherwise it rests.
func TestKeepIdle(t *testing.T) {
	now := time.Now()
	var zero time.Time
	for _, c := range []struct {
		named       int
		asked, gone time.Time
		idle        bool
		until       time.Duration
		why         string
	}{
		{0, zero, zero, true, 0, "nothing to listen for"},
		{1, zero, zero, false, -1, "a window is taken on"},
		{1, now.Add(-time.Hour), now.Add(-time.Hour), false, -1, "a window is taken on, whatever else"},
		{0, now.Add(-time.Minute), zero, false, keepAskFor - time.Minute, "a window was asked for a minute ago"},
		{0, now.Add(-keepAskFor), zero, true, 0, "a window asked for did not come"},
		{0, zero, now.Add(-10 * time.Second), false, keepLinger - 10*time.Second, "the window went 10 s ago"},
		{0, zero, now.Add(-keepLinger), true, 0, "the window went 30 s ago"},
		{0, now.Add(-time.Minute), now.Add(-keepLinger), false, keepAskFor - time.Minute, "the later of both"},
	} {
		idle, until := keepIdle(now, c.named, c.asked, c.gone)
		if idle != c.idle || until != c.until {
			t.Errorf("%s: idle %v until %v, want %v %v", c.why, idle, until, c.idle, c.until)
		}
	}
}

// fakeKeeper is a keeping whose windows are described by wins; the processes in profile use the window profile.
type fakeKeeper struct {
	wins             map[uintptr]topWindow
	profile          map[uint32]bool
	looked           map[uint32]int // how often each process was looked up
	named, checked   map[uintptr]int
	hooked, unhooked map[uint32]int
	hasIcon          map[uintptr]bool
	noIcons, refuse  bool
	listening        bool
	listens          int
	deaf             bool // the shell hook cannot be registered
	logs             []string
	ended            int
}

func newFakeKeeper() (*fakeKeeper, *iconKeeper) {
	f := &fakeKeeper{wins: map[uintptr]topWindow{}, profile: map[uint32]bool{10: true, 11: true}, looked: map[uint32]int{},
		named: map[uintptr]int{}, checked: map[uintptr]int{}, hooked: map[uint32]int{}, unhooked: map[uint32]int{},
		hasIcon: map[uintptr]bool{}}
	k := &iconKeeper{self: 1,
		visible: func(h uintptr) bool { return f.wins[h].Visible },
		inspect: func(h uintptr) topWindow { return f.wins[h] },
		ours:    func(pid uint32) bool { f.looked[pid]++; return f.profile[pid] },
		name:    func(h uintptr) bool { f.named[h]++; return !f.refuse },
		check: func(h uintptr) iconState {
			f.checked[h]++
			if f.hasIcon[h] {
				return iconsOurs
			}
			return iconsSet
		},
		listen: func() bool {
			f.listens++
			f.listening = !f.deaf
			return !f.deaf
		},
		unlisten: func() { f.listening = false },
		hook: func(pid uint32) []uintptr {
			f.hooked[pid]++
			return []uintptr{uintptr(pid)}
		},
		unhook:      func(hooks []uintptr) { f.unhooked[uint32(hooks[0])]++ },
		iconsLoaded: func() bool { return !f.noIcons },
		log:         func(level, msg string) { f.logs = append(f.logs, level+" "+msg) },
		end:         func() { f.ended++ }}
	return f, k
}

func (f *fakeKeeper) add(h uintptr, w topWindow) {
	w.Handle = h
	f.wins[h] = w
}

// A taskbar window of a process that uses the profile is named once, whenever it appears and whichever process it
// is in (the browser replaced its process, or the window came late); then its icon is kept. Another program's window,
// the browser's bubble and its hidden windows are left alone, and whose a window is is looked up only once.
func TestIconKeeperAdoptsTheProfilesWindows(t *testing.T) {
	f, k := newFakeKeeper()
	f.add(0x10, topWindow{Pid: 10, Visible: true, Titled: true})
	f.add(0x30, topWindow{Pid: 99, Visible: true, Titled: true})
	f.add(0x40, topWindow{Pid: 10, Visible: true, Owned: true})
	f.add(0x50, topWindow{Pid: 10})

	k.shell(hshellWindowCreated, 0x10)
	k.event(eventObjectShow, 0x10, objIDWindow, childIDSelf)
	k.shell(hshellRedraw, 0x10)
	if f.named[0x10] != 1 || f.checked[0x10] != 2 || f.hooked[10] != 1 {
		t.Errorf("the window: named %d (want 1), checked %d (want 2), its process hooked %d (want 1)", f.named[0x10], f.checked[0x10], f.hooked[10])
	}
	if k.settleEnd.IsZero() {
		t.Error("taking on a window did not start a while of looking")
	}
	for i := 0; i < 3; i++ {
		k.shell(hshellRedraw, 0x30)
	}
	k.shell(hshellWindowCreated, 0x40)
	k.sweep([]uintptr{0x30, 0x40, 0x50})
	if f.named[0x30]+f.checked[0x30]+f.named[0x40]+f.checked[0x40]+f.named[0x50]+f.checked[0x50] != 0 {
		t.Errorf("another program's window, a bubble or a hidden window was touched: %v %v", f.named, f.checked)
	}
	if f.looked[99] != 1 || f.looked[10] != 1 {
		t.Errorf("processes looked up %v, want each once", f.looked)
	}
	// the browser's process ends and a new one shows the window: taken on as well
	k.shell(hshellWindowDestroyed, 0x10)
	delete(f.wins, 0x10)
	if f.unhooked[10] != 1 {
		t.Error("the process's hooks stayed after its last window went")
	}
	f.add(0x20, topWindow{Pid: 11, Visible: true})
	k.sweep([]uintptr{0x30, 0x20})
	if f.named[0x20] != 1 || f.hooked[11] != 1 {
		t.Errorf("the window of the new process: named %d, hooked %d", f.named[0x20], f.hooked[11])
	}
	// a handle given to another program's window is looked up again
	k.shell(hshellWindowDestroyed, 0x20)
	f.add(0x20, topWindow{Pid: 99, Visible: true})
	k.shell(hshellWindowCreated, 0x20)
	if f.named[0x20] != 1 || f.looked[99] != 2 {
		t.Errorf("a reused handle: named %d (want 1), process 99 looked up %d (want 2)", f.named[0x20], f.looked[99])
	}
	// the program's own windows are never touched
	f.add(0x60, topWindow{Pid: 1, Visible: true})
	k.shell(hshellWindowCreated, 0x60)
	if f.named[0x60] != 0 || f.looked[1] != 0 {
		t.Error("the program's own window was looked at")
	}
	if f.ended != 0 {
		t.Error("the keeping ended with a window")
	}
}

// What the log gets: the window's icon confirmed once, each failure once, a window that does not appear.
func TestIconKeeperLog(t *testing.T) {
	f, k := newFakeKeeper()
	// a window that goes while it is named refuses its name too: that is no failure
	f.add(0x05, topWindow{Pid: 10, Visible: true})
	k.name = func(h uintptr) bool { f.wins[h] = topWindow{}; return false }
	k.shell(hshellWindowCreated, 0x05)
	k.name = func(h uintptr) bool { f.named[h]++; return !f.refuse }
	f.add(0x10, topWindow{Pid: 10, Visible: true})
	f.add(0x20, topWindow{Pid: 10, Visible: true})
	k.windowAsked(time.Now())
	f.noIcons, f.refuse = true, true
	k.shell(hshellWindowCreated, 0x10)
	k.shell(hshellWindowCreated, 0x20)
	if !k.asked.IsZero() {
		t.Error("a window taken on still counts as awaited")
	}
	k.shell(hshellRedraw, 0x10) // no icon yet: nothing to report
	f.hasIcon[0x10], f.hasIcon[0x20] = true, true
	k.shell(hshellRedraw, 0x10)
	k.shell(hshellRedraw, 0x20)
	k.windowAsked(time.Now())
	k.overdue()
	got := strings.Join(f.logs, "\n")
	want := strings.Join([]string{
		"WARN The settings window keeps the browser's icon: the program's icon could not be loaded.",
		"WARN The settings window's taskbar button could not be given the program's name (its property store refused it).",
		"INFO The settings window got the program's taskbar icon.",
		"WARN The settings window was not found within 30 s; it gets the program's taskbar icon if it appears in the next minutes.",
	}, "\n")
	if got != want {
		t.Errorf("log:\n%s\nwant:\n%s", got, want)
	}
	if _, overdue := keepWait(time.Now().Add(nameWindowWarn), k.asked, time.Time{}, k.late); overdue || k.asked.IsZero() {
		t.Error("an overdue window is reported again, or no longer listened for")
	}
}

// The shell hook is registered when a window is asked for and stays while a window is taken on; once none is left
// and none is coming it is deregistered, and with it what the keeping knew (its WinEvent hooks, whose windows were
// whose); a new ask registers it again. A hook that cannot be registered is reported once.
func TestIconKeeperListensOnlyWhenNeeded(t *testing.T) {
	f, k := newFakeKeeper()
	f.add(0x10, topWindow{Pid: 10, Visible: true})
	f.add(0x30, topWindow{Pid: 99, Visible: true})
	k.windowAsked(time.Now())
	if !f.listening || !k.armed.Load() {
		t.Fatal("not listening after a window was asked for")
	}
	k.shell(hshellWindowCreated, 0x10)
	k.shell(hshellWindowCreated, 0x30)
	if idle, _ := keepIdle(time.Now(), len(k.named), k.asked, k.gone); idle {
		t.Error("idle with a window taken on")
	}
	k.shell(hshellWindowDestroyed, 0x10)
	if k.gone.IsZero() {
		t.Fatal("the last window's going was not noted")
	}
	if idle, until := keepIdle(time.Now(), len(k.named), k.asked, k.gone); idle || until <= 0 || until > keepLinger {
		t.Errorf("right after the last window went: idle %v, until %v (want a while of listening)", idle, until)
	}
	if idle, _ := keepIdle(time.Now().Add(keepLinger), len(k.named), k.asked, k.gone); !idle {
		t.Error("still listening 30 s after the last window went")
	}
	k.setListening(false) // as the loop does then
	if f.listening || k.armed.Load() || f.unhooked[10] != 1 || len(k.verdict) != 0 {
		t.Errorf("after resting: listening %v, unhooked %v, windows remembered %v", f.listening, f.unhooked, k.verdict)
	}
	k.windowAsked(time.Now())
	if !f.listening || f.listens != 2 {
		t.Errorf("a new ask did not listen again (listening %v, %d registrations)", f.listening, f.listens)
	}
	k.shell(hshellWindowCreated, 0x30)
	if f.looked[99] != 2 {
		t.Error("whose a window is was not looked up again after resting")
	}

	f2, k2 := newFakeKeeper()
	f2.deaf = true
	k2.windowAsked(time.Now())
	k2.windowAsked(time.Now())
	if k2.armed.Load() || len(f2.logs) != 1 || !strings.Contains(f2.logs[0], "shell hook could not be set up") {
		t.Errorf("a hook that cannot be registered: armed %v, log %q", k2.armed.Load(), f2.logs)
	}
}

// A window whose browser puts its own icon back every time (or answers for it) is corrected at most 20 times within
// 10 s, then left alone and reported once; another window is still corrected, and the count starts anew after 10 s.
func TestIconKeeperStopsCorrectingAWindowThatFightsBack(t *testing.T) {
	f, k := newFakeKeeper()
	f.add(0x10, topWindow{Pid: 10, Visible: true})
	f.add(0x20, topWindow{Pid: 10, Visible: true})
	k.shell(hshellWindowCreated, 0x10)
	k.shell(hshellWindowCreated, 0x20)
	for i := 0; i < 100; i++ {
		k.shell(hshellRedraw, 0x10) // every correction redraws the button: the next notification
	}
	if f.checked[0x10] != keepFixesMax+1 {
		t.Errorf("corrected %d times, want %d", f.checked[0x10], keepFixesMax+1)
	}
	if len(f.logs) != 1 || !strings.Contains(f.logs[0], "put it back more than 20 times in 10 s") {
		t.Errorf("log %q, want one line", f.logs)
	}
	k.shell(hshellRedraw, 0x20)
	if f.checked[0x20] != 1 {
		t.Error("another window is no longer corrected")
	}
	c := fixCount{}
	start := time.Now()
	for i := 0; i < keepFixesMax; i++ {
		if c.add(start) {
			t.Fatal("too many before the limit")
		}
	}
	if !c.add(start.Add(time.Second)) {
		t.Error("the 21st correction within 10 s is not too many")
	}
	if c.add(start.Add(keepFixesPer + 2*time.Second)) {
		t.Error("the count did not start anew after 10 s")
	}
	// a window that goes and whose handle comes back is corrected again
	k.shell(hshellWindowDestroyed, 0x10)
	k.shell(hshellWindowCreated, 0x10)
	k.shell(hshellRedraw, 0x10)
	if f.checked[0x10] != keepFixesMax+2 {
		t.Errorf("a new window under the same handle: checked %d, want %d", f.checked[0x10], keepFixesMax+2)
	}
}

// One keeping at a time; StopWindowIcon ends it, and a keeping that starts after it is not taken. The hook's
// callback and the shell window's procedure reach the current one, with idObject as Windows passes it (a LONG in a
// pointer-sized argument).
func TestIconKeeperLifecycle(t *testing.T) {
	StopWindowIcon()
	gen := iconGen.Load()
	f1, k1 := newFakeKeeper()
	f1.add(0x10, topWindow{Pid: 10, Visible: true})
	f2, k2 := newFakeKeeper()
	f2.add(0x10, topWindow{Pid: 10, Visible: true})
	if !startKeeper(k1, gen) {
		t.Fatal("the first keeping did not start")
	}
	onWinEvent(0, eventObjectShow, 0x10, 0, 0, 0, 0)
	onWinEvent(0, eventObjectNameChange, 0x10, uintptr(0xFFFFFFFC), 0, 0, 0) // OBJID_CLIENT: not the window
	if f1.named[0x10] != 1 || f1.checked[0x10] != 0 {
		t.Errorf("first keeping: named %d checked %d", f1.named[0x10], f1.checked[0x10])
	}
	if !startKeeper(k2, gen) || f1.ended != 1 {
		t.Error("a new keeping did not end the one before")
	}
	m := uint32(shellHookMessage())
	if m == 0 {
		t.Fatal("no SHELLHOOK message")
	}
	shellWindowProc(0, m, hshellRedraw, 0x10)
	if f1.named[0x10] != 1 || f2.named[0x10] != 1 {
		t.Errorf("the shell hook's redraw went to %v %v", f1.named, f2.named)
	}
	k2.stop()
	k2.stop()
	if f2.ended != 1 {
		t.Errorf("the keeping ended %d times, want once", f2.ended)
	}
	endKeeper(k2)
	f3, k3 := newFakeKeeper()
	if !startKeeper(k3, gen) {
		t.Fatal("a third keeping did not start")
	}
	StopWindowIcon()
	if f3.ended != 1 {
		t.Error("StopWindowIcon did not end the keeping")
	}
	if _, k4 := newFakeKeeper(); startKeeper(k4, gen) {
		t.Error("a keeping started after StopWindowIcon")
	}
	f3.add(0x30, topWindow{Pid: 10, Visible: true})
	onWinEvent(0, eventObjectShow, 0x30, 0, 0, 0, 0)
	if f3.named[0x30] != 0 {
		t.Error("an event came through after StopWindowIcon")
	}
}

// The window's icons come from the icon built into the program, at the size asked for (or the next larger one).
func TestIcoImage(t *testing.T) {
	for _, c := range []struct{ size, want int }{{16, 16}, {32, 32}, {64, 64}, {36, 40}, {20, 24}, {96, 64}} {
		img := icoImage(assets.Icon, c.size)
		if img == nil {
			t.Fatalf("%d px: no image", c.size)
		}
		if w := imageWidth(img); w != c.want {
			t.Errorf("%d px: the %d px image, want %d", c.size, w, c.want)
		}
	}
	for _, bad := range [][]byte{nil, []byte("not an icon"), {0, 0, 1, 0, 1, 0}} {
		if icoImage(bad, 32) != nil {
			t.Errorf("an image from %q", bad)
		}
	}
	loadWindowIcons("") // from the built-in icon alone
	if !haveWindowIcons() {
		t.Error("the program's icons could not be made from the built-in icon")
	}
}

// imageWidth is the width of an .ico image: a PNG's from its header, a bitmap's from its BITMAPINFOHEADER.
func imageWidth(img []byte) int {
	if bytes.HasPrefix(img, []byte("\x89PNG")) {
		return int(binary.BigEndian.Uint32(img[16:]))
	}
	return int(binary.LittleEndian.Uint32(img[4:]))
}

package winsys

import (
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// The settings window is a browser window, so Windows takes its taskbar button for the browser's: the button's menu
// names the browser, and "Pin to taskbar" would pin the browser. A window can carry an identity of its own (the
// System.AppUserModel properties: a name, an icon and the command that starts it again), and Windows accepts them
// from another process, so the program gives its window its own.
var (
	// shell32 is declared in tray_windows.go
	ole32                        = syscall.NewLazyDLL("ole32.dll")
	pSHGetPropertyStoreForWindow = shell32.NewProc("SHGetPropertyStoreForWindow")
	pCoInitializeEx              = ole32.NewProc("CoInitializeEx")
	pCoUninitialize              = ole32.NewProc("CoUninitialize")

	iidPropertyStore = syscall.GUID{Data1: 0x886D8EEB, Data2: 0x8CF2, Data3: 0x4446, Data4: [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99}}
	appUserModel     = syscall.GUID{Data1: 0x9F4C2855, Data2: 0x9F79, Data3: 0x4B39, Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3}}
)

// The System.AppUserModel properties of a window, by their number in appUserModel.
const (
	propRelaunchCommand     = 2
	propRelaunchIcon        = 3
	propRelaunchDisplayName = 4
	propAppID               = 5
)

const (
	vtLPWSTR = 31 // a PROPVARIANT that holds a UTF-16 string

	// the methods of IPropertyStore, by their place in its table
	storeRelease  = 2
	storeSetValue = 6
	storeCommit   = 7

	nameWindowFor       = 15 * time.Second       // how long a window that was just opened is looked for
	nameWindowFast      = 3 * time.Second        // in its first seconds often, so its button has the program's icon from the start:
	nameWindowFastEvery = 30 * time.Millisecond  // every so often
	nameWindowEvery     = 250 * time.Millisecond // and less often after that

	// for a while after the keeping starts, and after it takes on a new window, all the browser's windows are looked
	// at now and then too, in case the browser changed one before the hooks were in place or an event was missed
	keepSettle      = 10 * time.Second
	keepSettleEvery = 250 * time.Millisecond
)

// propertyKey is Windows' PROPERTYKEY, propVariant its PROPVARIANT for a string, propertyStore an IPropertyStore.
type (
	propertyKey struct {
		fmtid syscall.GUID
		pid   uint32
	}
	propVariant struct {
		vt  uint16
		_   [6]byte
		str *uint16
		_   uintptr
	}
	propertyStore struct{ methods *[8]uintptr }
)

// windowIdentity is what the settings window's taskbar button is given: the ID its buttons are grouped by (one per
// data folder, as for run-at-login), the name its menu shows, and the command and icon of a pinned button. A copy with
// a data folder of its own is started again with that folder.
func windowIdentity(exe, home string) (id, name, command, icon string) {
	command = `"` + exe + `"`
	if home = strings.TrimRight(strings.TrimSpace(home), `\/`); home != "" {
		command += ` --home "` + home + `"` // no trailing backslash: it would escape the closing quote
	}
	return runNameFor(home), runNameBase, command, exe + ",0"
}

// NameWindow gives the settings window that was just opened with profileDir the program's own taskbar identity and
// icon. The window takes a moment to appear, so it is looked for until it is there (often at first, at most
// nameWindowFor); the caller runs this on a goroutine of its own. The browser gives its window its own icon again now
// and then (when the page's icon loads, when the title changes), and it may replace the window with a new one, so once
// found every window of that browser process keeps the program's identity and icon for as long as the process lives
// (keepWindowIcons). It reports whether a window was named. Only the newest window is looked after: a later
// NameWindow, or StopWindowIcon, ends the search and the keeping of the one before.
func NameWindow(profileDir string) bool {
	exe, err := os.Executable()
	if err != nil || profileDir == "" {
		return false
	}
	StopWindowIcon()
	gen := iconGen.Load()
	loadWindowIcons(exe) // read now, so they are set the moment the window is there
	id, name, command, icon := windowIdentity(exe, os.Getenv("MPRP_HOME"))
	// the ID first: the other three belong to it
	strs := []windowString{{propAppID, id}, {propRelaunchCommand, command}, {propRelaunchDisplayName, name}, {propRelaunchIcon, icon}}
	start := time.Now()
	for {
		if iconGen.Load() != gen {
			return false
		}
		if w, ok := findWindowWith(profileDir, pickTaskbarWindow); ok {
			named := nameWindow(w.Handle, strs)
			go keepWindowIcons(w.Pid, w.Handle, strs, gen)
			return named
		}
		since := time.Since(start)
		if since > nameWindowFor {
			return false
		}
		if since < nameWindowFast {
			time.Sleep(nameWindowFastEvery)
		} else {
			time.Sleep(nameWindowEvery)
		}
	}
}

// nameWindow gives one window the program's identity and icons; false when the identity was not taken.
func nameWindow(hwnd uintptr, strs []windowString) bool {
	applyWindowIcons(hwnd)
	named := setWindowStrings(hwnd, strs)
	applyWindowIcons(hwnd) // the new ID can give the window a button of its own, drawn with its icon now
	return named
}

// taskbarWindow says whether a window gets a taskbar button (Windows' own rule): a visible top-level window that is
// not a tool window and has no owner, unless it asks for a button. The browser's bubbles and menus have none.
func taskbarWindow(w topWindow) bool {
	return w.Visible && !w.Child && (w.AppWindow || (!w.Owned && !w.Tool))
}

// pickTaskbarWindow takes the first window (from the top of the stacking order) of one of the pids that has a title
// and a taskbar button: the settings window, not a bubble the browser shows over it.
func pickTaskbarWindow(wins []topWindow, pids map[uint32]bool) (topWindow, bool) {
	for _, w := range wins {
		if pids[w.Pid] && w.Titled && taskbarWindow(w) {
			return w, true
		}
	}
	return topWindow{}, false
}

var (
	pSetWinEventHook   = user32.NewProc("SetWinEventHook")
	pUnhookWinEvent    = user32.NewProc("UnhookWinEvent")
	pPeekMessage       = user32.NewProc("PeekMessageW")
	pPostThreadMessage = user32.NewProc("PostThreadMessageW")
	pGetCurrentThread  = kernel32.NewProc("GetCurrentThreadId")
	pMsgWaitForObjects = user32.NewProc("MsgWaitForMultipleObjects")

	pRegisterShellHook   = user32.NewProc("RegisterShellHookWindow")
	pDeregisterShellHook = user32.NewProc("DeregisterShellHookWindow")
	pSendMessageTimeout  = user32.NewProc("SendMessageTimeoutW")

	// made once: callbacks are never freed (onWinEvent and the shell window's procedure find the keeping they are for
	// in iconKeeperNow)
	winEventCallback = sync.OnceValue(func() uintptr { return syscall.NewCallback(onWinEvent) })
	// the message the shell hook's notifications come as
	shellHookMessage = sync.OnceValue(func() uintptr {
		name, _ := syscall.UTF16PtrFromString("SHELLHOOK")
		m, _, _ := pRegisterWindowMessage.Call(uintptr(unsafe.Pointer(name)))
		return m
	})
	// the class of the hidden window that gets them (nil: it could not be registered)
	shellWindowClass = sync.OnceValue(func() *uint16 {
		name, _ := syscall.UTF16PtrFromString(runNameBase + "-icon-keeper")
		inst, _, _ := pGetModuleHandle.Call(0)
		wc := wndClassEx{lpfnWndProc: syscall.NewCallback(shellWindowProc), hInstance: inst, lpszClassName: name}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		if r, _, _ := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			return nil
		}
		return name
	})

	keeperMu      sync.Mutex
	iconKeeperNow *iconKeeper  // the keeping now (nil: none)
	iconGen       atomic.Int64 // counts StopWindowIcon: a search or a keeping from before it ends
)

// WinEvents (SetWinEventHook) and what they are for.
const (
	eventObjectDestroy    = 0x8001
	eventObjectShow       = 0x8002
	eventObjectNameChange = 0x800C
	objIDWindow           = 0      // idObject of an event about the window itself
	childIDSelf           = 0      // and its idChild
	winEventOutOfContext  = 0x0000 // the events come to this process's hook thread, through its message loop
	winEventSkipOwn       = 0x0002 // and none of this process's own
	pmNoRemove            = 0x0000
	pmRemove              = 0x0001
	wmQuit                = 0x0012

	// shell hook notifications (RegisterShellHookWindow), in the low 15 bits of wParam
	hshellWindowCreated   = 1
	hshellWindowDestroyed = 2
	hshellRedraw          = 6 // a taskbar button is drawn again: its window's title or icon changed
	wmGetIcon             = 0x007F
	smtoAbortIfHung       = 0x0002
	wsPopup               = 0x80000000

	// waiting for the browser process to end while the hook thread's messages are handled
	synchronize = 0x00100000
	qsAllInput  = 0x04FF
	waitTimeout = 0x102
	waitFailed  = 0xFFFFFFFF
	infinite    = 0xFFFFFFFF
)

// keepAction is what an event means for the keeping.
type keepAction int

const (
	keepNothing keepAction = iota
	keepCheck              // a window may be new, or may have another icon now: take it on, or check its icon
	keepForget             // a window is gone (its handle may later be given to another)
)

// shellEvent says what a shell hook notification means: a window that got a taskbar button, or whose button was
// redrawn (as it is when the browser gives the window an icon of its own, which no WinEvent tells), is checked; a
// destroyed one is forgotten. It is about any window of any program: keepIcons checks whose it is.
func shellEvent(code uintptr) keepAction {
	switch code & 0x7fff {
	case hshellWindowCreated, hshellRedraw:
		return keepCheck
	case hshellWindowDestroyed:
		return keepForget
	}
	return keepNothing
}

// iconEvent says what a WinEvent of the browser process means: a window that was shown, or whose title changed
// (after which the browser sets its own icon again), is checked; a destroyed one is forgotten. Only events about a
// window itself count, not about its contents.
func iconEvent(event uint32, idObject, idChild int32) keepAction {
	if idObject != objIDWindow || idChild != childIDSelf {
		return keepNothing
	}
	switch event {
	case eventObjectShow, eventObjectNameChange:
		return keepCheck
	case eventObjectDestroy:
		return keepForget
	}
	return keepNothing
}

// adoptWindow decides for a window that an event or a look is about: name it (a taskbar window of the browser
// process pid that has not been named yet) or check its icon (one named before). Windows of other processes, and the
// browser's bubbles, menus and hidden helper windows, are left alone.
func adoptWindow(w topWindow, pid uint32, named bool) (name, check bool) {
	if pid == 0 || w.Pid != pid || !taskbarWindow(w) {
		return false, false
	}
	return !named, named
}

// iconKeeper keeps the program's identity and icon on every taskbar window of one browser process: name gives a new
// window both, check sets the icon again where the window has another one (checkWindowIcons), inspect reads a window
// (describeWindow), end stops the hook thread (once, through stop). alive, when set, tells whether the process still
// has a window (when its end cannot be waited for). named and adopted belong to the hook thread.
type iconKeeper struct {
	pid     uint32
	inspect func(hwnd uintptr) topWindow
	name    func(hwnd uintptr)
	check   func(hwnd uintptr)
	alive   func() bool
	end     func()
	once    sync.Once
	named   map[uintptr]bool // the windows given the identity
	adopted bool             // a window was named since the hook thread last looked
}

// event handles a WinEvent.
func (k *iconKeeper) event(event uint32, hwnd uintptr, idObject, idChild int32) {
	k.act(iconEvent(event, idObject, idChild), hwnd)
}

// shell handles a shell hook notification.
func (k *iconKeeper) shell(code, hwnd uintptr) { k.act(shellEvent(code), hwnd) }

func (k *iconKeeper) act(a keepAction, hwnd uintptr) {
	switch a {
	case keepCheck:
		k.consider(k.inspect(hwnd))
	case keepForget:
		was := k.named[hwnd]
		delete(k.named, hwnd)
		if was && k.alive != nil && !k.alive() {
			k.stop()
		}
	}
}

// consider names the window w or checks its icon, as adoptWindow decides.
func (k *iconKeeper) consider(w topWindow) {
	name, check := adoptWindow(w, k.pid, k.named[w.Handle])
	switch {
	case name:
		if k.named == nil {
			k.named = map[uintptr]bool{}
		}
		k.named[w.Handle] = true
		k.adopted = true
		k.name(w.Handle)
	case check:
		k.check(w.Handle)
	}
}

// sweep considers every window in wins.
func (k *iconKeeper) sweep(wins []topWindow) {
	for _, w := range wins {
		k.consider(w)
	}
}

func (k *iconKeeper) stop() { k.once.Do(k.end) }

// hasWindow says whether one of wins belongs to the process pid.
func hasWindow(wins []topWindow, pid uint32) bool {
	for _, w := range wins {
		if w.Pid == pid {
			return true
		}
	}
	return false
}

// startKeeper makes k the keeping now, ending the one before; false (and nothing changed) when StopWindowIcon was
// called since gen was read.
func startKeeper(k *iconKeeper, gen int64) bool {
	keeperMu.Lock()
	if iconGen.Load() != gen {
		keeperMu.Unlock()
		return false
	}
	old := iconKeeperNow
	iconKeeperNow = k
	keeperMu.Unlock()
	if old != nil {
		old.stop()
	}
	return true
}

// endKeeper forgets k, if it is still the keeping now.
func endKeeper(k *iconKeeper) {
	keeperMu.Lock()
	if iconKeeperNow == k {
		iconKeeperNow = nil
	}
	keeperMu.Unlock()
}

// StopWindowIcon stops looking after the settings window's icon (the program quits, or another window opens), and
// ends a NameWindow that is still looking for its window.
func StopWindowIcon() {
	keeperMu.Lock()
	iconGen.Add(1)
	old := iconKeeperNow
	iconKeeperNow = nil
	keeperMu.Unlock()
	if old != nil {
		old.stop()
	}
}

// currentKeeper is the keeping now (nil: none).
func currentKeeper() *iconKeeper {
	keeperMu.Lock()
	defer keeperMu.Unlock()
	return iconKeeperNow
}

// onWinEvent is the WinEvent callback (WINEVENTPROC); it runs on the hook thread.
func onWinEvent(_, event, hwnd, idObject, idChild, _, _ uintptr) uintptr {
	if k := currentKeeper(); k != nil {
		k.event(uint32(event), hwnd, int32(idObject), int32(idChild))
	}
	return 0
}

// shellWindowProc is the hidden window's procedure: it passes the shell hook's notifications on.
func shellWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if m := shellHookMessage(); m != 0 && uintptr(message) == m {
		if k := currentKeeper(); k != nil {
			k.shell(wParam, lParam)
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

// keepWindowIcons keeps the program's identity (strs) and icon on every taskbar window of the browser process pid,
// first among them the window first (named already), until that process ends or StopWindowIcon is called. A window
// the browser opens, or replaces the first one with, is named once; the icon is set again whenever the browser has
// put its own back. It listens to that process's WinEvents (out of context: nothing goes into the browser) and to the
// shell hook (a hidden window of its own), whose notifications both come through a message loop on this goroutine's
// own thread, which also waits for the process to end.
func keepWindowIcons(pid uint32, first uintptr, strs []windowString, gen int64) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var m msg
	pPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmNoRemove) // makes the thread's message queue, for the quit
	tid, _, _ := pGetCurrentThread.Call()
	k := &iconKeeper{pid: pid, inspect: describeWindow, check: checkWindowIcons,
		name:  func(h uintptr) { nameWindow(h, strs) },
		end:   func() { pPostThreadMessage.Call(tid, wmQuit, 0, 0) },
		named: map[uintptr]bool{first: true}}
	var proc syscall.Handle
	if h, err := syscall.OpenProcess(synchronize, false, pid); err == nil {
		proc = h
		defer syscall.CloseHandle(h)
	} else {
		k.alive = func() bool { return hasWindow(topWindows(), pid) } // the keeping ends with the last window instead
	}
	if !startKeeper(k, gen) {
		return
	}
	defer endKeeper(k)
	var hooks []uintptr
	for _, r := range [][2]uintptr{{eventObjectDestroy, eventObjectShow}, {eventObjectNameChange, eventObjectNameChange}} {
		if hk, _, _ := pSetWinEventHook.Call(r[0], r[1], 0, winEventCallback(), uintptr(pid), 0, winEventOutOfContext|winEventSkipOwn); hk != 0 {
			hooks = append(hooks, hk)
		}
	}
	defer func() {
		for _, hk := range hooks {
			pUnhookWinEvent.Call(hk)
		}
	}()
	shellHooked := false
	if cls := shellWindowClass(); cls != nil && shellHookMessage() != 0 {
		inst, _, _ := pGetModuleHandle.Call(0)
		if w, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), 0, wsPopup, 0, 0, 0, 0, 0, 0, inst, 0); w != 0 {
			defer pDestroyWindow.Call(w)
			if r, _, _ := pRegisterShellHook.Call(w); r != 0 {
				shellHooked = true
				defer pDeregisterShellHook.Call(w)
			}
		}
	}
	if len(hooks) == 0 && !shellHooked {
		return
	}
	// a window the browser opened, or an icon it set, before the hooks were in place
	k.sweep(topWindows())
	if k.alive != nil && !k.alive() {
		return
	}
	settleUntil := time.Now().Add(keepSettle)
	procs, n := [1]syscall.Handle{proc}, uintptr(0)
	if proc != 0 {
		n = 1
	}
	for {
		if k.adopted {
			k.adopted = false
			settleUntil = time.Now().Add(keepSettle)
		}
		wait := uintptr(infinite)
		if left := time.Until(settleUntil); left > 0 {
			wait = uintptr(min(left, keepSettleEvery) / time.Millisecond)
		}
		r, _, _ := pMsgWaitForObjects.Call(n, uintptr(unsafe.Pointer(&procs[0])), 0, wait, qsAllInput)
		switch {
		case r == waitFailed:
			return
		case r < n: // the browser has ended
			return
		case r == waitTimeout:
			k.sweep(topWindows())
		default: // messages: the hooks' notifications, or the quit (k.stop)
			for {
				if got, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove); got == 0 {
					break
				}
				if m.message == wmQuit {
					return
				}
				pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
				pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
			}
		}
	}
}

// windowString is one System.AppUserModel property and its value.
type windowString struct {
	prop  uint32
	value string
}

// setWindowStrings writes the properties to the window's property store, in the order given; false when the store
// cannot be had or a value is refused.
func setWindowStrings(hwnd uintptr, values []windowString) bool {
	runtime.LockOSThread() // COM is set up per thread
	defer runtime.UnlockOSThread()
	if hr, _, _ := pCoInitializeEx.Call(0, 2); int32(hr) >= 0 { // COINIT_APARTMENTTHREADED; S_FALSE: already set up
		defer pCoUninitialize.Call()
	}
	var store *propertyStore
	hr, _, _ := pSHGetPropertyStoreForWindow.Call(hwnd, uintptr(unsafe.Pointer(&iidPropertyStore)), uintptr(unsafe.Pointer(&store)))
	if int32(hr) < 0 || store == nil {
		return false
	}
	self := uintptr(unsafe.Pointer(store))
	defer syscall.SyscallN(store.methods[storeRelease], self)
	ok := true
	for _, v := range values {
		s, err := syscall.UTF16PtrFromString(v.value)
		if err != nil {
			return false
		}
		key := propertyKey{fmtid: appUserModel, pid: v.prop}
		val := propVariant{vt: vtLPWSTR, str: s} // the store copies the string
		if hr, _, _ := syscall.SyscallN(store.methods[storeSetValue], self, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&val))); int32(hr) < 0 {
			ok = false
		}
	}
	if hr, _, _ := syscall.SyscallN(store.methods[storeCommit], self); int32(hr) < 0 {
		ok = false
	}
	return ok
}

var (
	pPrivateExtractIcons = user32.NewProc("PrivateExtractIconsW")

	// the program's icons for the window, read once (loadWindowIcons): a window only keeps their handles, so they are
	// kept for the program's life
	windowIconMu sync.Mutex
	windowIcons  [2]uintptr
)

const (
	wmSetIcon = 0x0080
	smCxIcon  = 11 // the width of a large icon on this screen; smCxSmIcon (tray_windows.go) is the small one's
)

// loadWindowIcons reads the program file's own icon in the two sizes Windows asks a window for, each at exactly that
// size (the taskbar button and Alt+Tab use the large one, the title bar the small one), once. An icon handle is valid
// in every process of the session, so the browser's window can use one this program holds. A program file without an
// icon (a plain development build) gives none, and the browser's stays.
func loadWindowIcons(exe string) {
	path, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return
	}
	windowIconMu.Lock()
	defer windowIconMu.Unlock()
	for i, metric := range []uintptr{smCxSmIcon, smCxIcon} {
		if windowIcons[i] != 0 {
			continue
		}
		size, _, _ := pGetSystemMetrics.Call(metric)
		var icon uintptr
		if n, _, _ := pPrivateExtractIcons.Call(uintptr(unsafe.Pointer(path)), 0, size, size, uintptr(unsafe.Pointer(&icon)), 0, 1, 0); int32(n) >= 1 && icon != 0 {
			windowIcons[i] = icon
		}
	}
}

// checkWindowIcons gives the window the icons loadWindowIcons read again where it has others. (Setting them when it
// has them already would make the taskbar button redraw, which the shell hook reports: it would never end.)
func checkWindowIcons(hwnd uintptr) {
	windowIconMu.Lock()
	want := windowIcons
	windowIconMu.Unlock()
	for i, icon := range want {
		var has uintptr
		r, _, _ := pSendMessageTimeout.Call(hwnd, wmGetIcon, uintptr(i), 0, smtoAbortIfHung, 200, uintptr(unsafe.Pointer(&has)))
		if icon != 0 && r != 0 && has != icon {
			applyWindowIcons(hwnd)
			return
		}
	}
}

// applyWindowIcons gives the window the icons loadWindowIcons read.
func applyWindowIcons(hwnd uintptr) {
	windowIconMu.Lock()
	defer windowIconMu.Unlock()
	for i, icon := range windowIcons { // i is also WM_SETICON's ICON_SMALL (0) and ICON_BIG (1)
		if icon != 0 {
			pPostMessage.Call(hwnd, wmSetIcon, uintptr(i), icon)
		}
	}
}

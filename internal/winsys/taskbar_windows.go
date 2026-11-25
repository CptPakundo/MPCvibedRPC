package winsys

import (
	"encoding/binary"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/CptPakundo/MPRP/internal/assets"
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

	// After a window was asked for, all windows are looked at often in its first seconds (so its button has the
	// program's icon from the start, should an event be missed) and less often after that; a window that has not
	// appeared after nameWindowWarn is reported (and still taken on when it does appear).
	nameWindowFast      = 3 * time.Second
	nameWindowFastEvery = 30 * time.Millisecond
	nameWindowEvery     = 250 * time.Millisecond
	nameWindowWarn      = 30 * time.Second

	// for a while after a window was taken on, all windows are looked at now and then too, in case the browser
	// changed its icon and an event was missed
	keepSettle      = 10 * time.Second
	keepSettleEvery = 250 * time.Millisecond

	keepForgetAll = 4096 // more windows remembered than this: forget them all (they are looked up again)
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

// NameWindow is called when a settings window was just opened with profileDir: its taskbar button gets the program's
// own identity and icon. It starts the keeping (keepWindowIcons) the first time and tells it a window is coming; the
// keeping then gives every taskbar window of a browser process that uses profileDir the identity and icon the moment
// it appears, whenever that is and whichever process it is in, and keeps the program's icon on it (the browser puts
// its own back now and then), until StopWindowIcon. log gets a line when the window first has the program's icon,
// when something fails, and when the keeping ends. It reports whether the keeping runs.
func NameWindow(profileDir string, log func(level, msg string)) bool {
	exe, err := os.Executable()
	if err != nil || profileDir == "" {
		return false
	}
	if log == nil {
		log = func(string, string) {}
	}
	loadWindowIcons(exe) // read now, so they are set the moment the window is there
	k := ensureKeeper(profileDir, exe, log)
	if k == nil {
		return false
	}
	r, _, _ := pPostThreadMessage.Call(k.tid, wmWindowAsked, 0, 0)
	return r != 0
}

// nameWindow gives one window the program's identity and icons; false when the identity was not taken.
func nameWindow(hwnd uintptr, strs []windowString) bool {
	applyWindowIcons(hwnd)
	named := setWindowStrings(hwnd, strs) // the ID first: the other three belong to it
	applyWindowIcons(hwnd)                // the new ID can give the window a button of its own, drawn with its icon now
	return named
}

// taskbarWindow says whether a window gets a taskbar button (Windows' own rule): a visible top-level window that is
// not a tool window and has no owner, unless it asks for a button. The browser's bubbles and menus have none.
func taskbarWindow(w topWindow) bool {
	return w.Visible && !w.Child && (w.AppWindow || (!w.Owned && !w.Tool))
}

// processUsesProfile says whether the process pid was started with the browser profile profileDir.
func processUsesProfile(pid uint32, profileDir string) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	return usesProfile(commandLine(h), profileDir)
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
	ensureMu      sync.Mutex   // one ensureKeeper at a time
	iconKeeperNow *iconKeeper  // the keeping now (nil: none)
	iconGen       atomic.Int64 // counts StopWindowIcon: a keeping that starts after one is not taken
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
	wmWindowAsked         = 0x8001 // WM_APP+1, to the keeping's thread: NameWindow was called

	// shell hook notifications (RegisterShellHookWindow), in the low 15 bits of wParam
	hshellWindowCreated   = 1
	hshellWindowDestroyed = 2
	hshellRedraw          = 6 // a taskbar button is drawn again: its window's title or icon changed
	wmGetIcon             = 0x007F
	smtoAbortIfHung       = 0x0002
	wsPopup               = 0x80000000

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
// destroyed one is forgotten. It is about any window of any program: the keeping checks whose it is.
func shellEvent(code uintptr) keepAction {
	switch code & 0x7fff {
	case hshellWindowCreated, hshellRedraw:
		return keepCheck
	case hshellWindowDestroyed:
		return keepForget
	}
	return keepNothing
}

// iconEvent says what a WinEvent of a browser process with a window taken on means: a window that was shown, or whose
// title changed (after which the browser sets its own icon again), is checked; a destroyed one is forgotten. Only
// events about a window itself count, not about its contents.
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

// keepWait says how long the keeping may wait for an event before it looks at all windows again: often in the first
// seconds after a window was asked for (at asked; zero: none), less often until it appears, and for a while after a
// window was taken on (until settleEnd); otherwise until an event comes (negative). overdue: the window asked for has
// not appeared within nameWindowWarn.
func keepWait(now, asked, settleEnd time.Time) (wait time.Duration, overdue bool) {
	wait = -1
	if !asked.IsZero() {
		switch since := now.Sub(asked); {
		case since >= nameWindowWarn:
			return 0, true
		case since < nameWindowFast:
			wait = nameWindowFastEvery
		default:
			wait = nameWindowEvery
		}
	}
	if left := settleEnd.Sub(now); left > 0 {
		if s := min(left, keepSettleEvery); wait < 0 || s < wait {
			wait = s
		}
	}
	return wait, false
}

// iconKeeper gives the program's identity and icon to every taskbar window of a browser process that uses the
// window profile, and keeps the icon on it. Its functions are Windows' (tests give fakes): visible and inspect read a
// window, ours tells whether a process uses the profile, name gives a window the identity and icons (false: the
// identity was refused), check sets the icons again where the window has others (true: it had the program's),
// iconsLoaded tells whether the program's icons could be read, hook and unhook follow one process's WinEvents, end
// stops the hook thread (once, through stop). Everything else belongs to the hook thread.
type iconKeeper struct {
	profile     string
	tid         uintptr
	self        uint32
	visible     func(hwnd uintptr) bool
	inspect     func(hwnd uintptr) topWindow
	ours        func(pid uint32) bool
	name        func(hwnd uintptr) bool
	check       func(hwnd uintptr) bool
	iconsLoaded func() bool
	hook        func(pid uint32) []uintptr
	unhook      func(hooks []uintptr)
	log         func(level, msg string)
	end         func()
	once        sync.Once

	verdict   map[uintptr]bool     // taskbar windows looked up: whether their process uses the profile
	named     map[uintptr]uint32   // the windows given the identity, and their process
	hooks     map[uint32][]uintptr // the WinEvent hooks of the processes with a window taken on
	asked     time.Time            // when a window was asked for that has not appeared yet (zero: none)
	settleEnd time.Time            // until when all windows are looked at now and then
	confirmed bool                 // a window was seen with the program's icon (reported once)
	warned    map[string]bool      // failures reported (once each)
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
		if ours, seen := k.verdict[hwnd]; seen && !ours {
			return // another program's window: the cheap way out, as most events are about those
		}
		if !k.visible(hwnd) {
			return
		}
		k.consider(k.inspect(hwnd))
	case keepForget:
		k.forget(hwnd)
	}
}

// consider takes the window w on (a taskbar window of a process that uses the profile, not named yet) or checks its
// icon (one named before). Other programs' windows, and the browser's bubbles, menus and hidden windows, are left
// alone; whose a window is is looked up once.
func (k *iconKeeper) consider(w topWindow) {
	if w.Pid == 0 || w.Pid == k.self || !taskbarWindow(w) {
		return
	}
	if k.verdict == nil || len(k.verdict) > keepForgetAll {
		k.verdict = map[uintptr]bool{}
	}
	ours, seen := k.verdict[w.Handle]
	if !seen {
		ours = k.ours(w.Pid)
		k.verdict[w.Handle] = ours
	}
	if !ours {
		return
	}
	if _, done := k.named[w.Handle]; done {
		if k.check(w.Handle) && !k.confirmed {
			k.confirmed = true
			k.log("INFO", "The settings window got the program's taskbar icon.")
		}
		return
	}
	k.adopt(w)
}

// adopt gives the window w the identity and icons, and follows its process's events.
func (k *iconKeeper) adopt(w topWindow) {
	if k.named == nil {
		k.named = map[uintptr]uint32{}
	}
	k.named[w.Handle] = w.Pid
	if !k.iconsLoaded() {
		k.warn("icons", "The settings window keeps the browser's icon: the program's icon could not be loaded.")
	}
	if !k.name(w.Handle) && k.visible(w.Handle) { // (a window that went meanwhile refuses it too)
		k.warn("store", "The settings window's taskbar button could not be given the program's name (its property store refused it).")
	}
	if k.hooks == nil {
		k.hooks = map[uint32][]uintptr{}
	}
	if _, ok := k.hooks[w.Pid]; !ok {
		k.hooks[w.Pid] = k.hook(w.Pid)
	}
	k.asked = time.Time{}
	k.settleEnd = time.Now().Add(keepSettle)
}

// forget drops a window that is gone, and its process's hooks with its last window taken on.
func (k *iconKeeper) forget(hwnd uintptr) {
	delete(k.verdict, hwnd)
	pid, was := k.named[hwnd]
	if !was {
		return
	}
	delete(k.named, hwnd)
	for _, p := range k.named {
		if p == pid {
			return
		}
	}
	if hooks, ok := k.hooks[pid]; ok {
		k.unhook(hooks)
		delete(k.hooks, pid)
	}
}

// sweep looks at every window in hwnds (all top-level windows).
func (k *iconKeeper) sweep(hwnds []uintptr) {
	for _, h := range hwnds {
		k.act(keepCheck, h)
	}
}

// windowAsked notes that a window was asked for (NameWindow).
func (k *iconKeeper) windowAsked(now time.Time) { k.asked = now }

// overdue reports the window asked for that has not appeared.
func (k *iconKeeper) overdue() {
	k.asked = time.Time{}
	k.log("WARN", "The settings window was not found within 30 s; it gets the program's taskbar icon if it appears later.")
}

// warn logs a failure once.
func (k *iconKeeper) warn(kind, msg string) {
	if k.warned == nil {
		k.warned = map[string]bool{}
	}
	if !k.warned[kind] {
		k.warned[kind] = true
		k.log("WARN", msg)
	}
}

// unhookAll ends every process's hooks.
func (k *iconKeeper) unhookAll() {
	for pid, hooks := range k.hooks {
		k.unhook(hooks)
		delete(k.hooks, pid)
	}
}

func (k *iconKeeper) stop() { k.once.Do(k.end) }

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

// StopWindowIcon ends the keeping of the settings window's identity and icon (the program quits).
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

// ensureKeeper returns the keeping for profileDir, started now if none runs; nil when it could not start.
func ensureKeeper(profileDir, exe string, log func(level, msg string)) *iconKeeper {
	ensureMu.Lock()
	defer ensureMu.Unlock()
	if k := currentKeeper(); k != nil && k.profile == profileDir {
		return k
	}
	ready := make(chan *iconKeeper, 1)
	go keepWindowIcons(profileDir, exe, log, iconGen.Load(), ready)
	return <-ready
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

// hookProcess follows the WinEvents of the process pid (out of context: nothing goes into the browser).
func hookProcess(pid uint32) []uintptr {
	var hooks []uintptr
	for _, r := range [][2]uintptr{{eventObjectDestroy, eventObjectShow}, {eventObjectNameChange, eventObjectNameChange}} {
		if hk, _, _ := pSetWinEventHook.Call(r[0], r[1], 0, winEventCallback(), uintptr(pid), 0, winEventOutOfContext|winEventSkipOwn); hk != 0 {
			hooks = append(hooks, hk)
		}
	}
	return hooks
}

func unhookProcess(hooks []uintptr) {
	for _, hk := range hooks {
		pUnhookWinEvent.Call(hk)
	}
}

// keepWindowIcons is the keeping's thread, for the program's life from the first settings window on. It listens to
// the shell hook (a hidden window of its own: every program's taskbar windows as they appear, are redrawn and go) and
// to the WinEvents of the browser processes it took a window of, both through a message loop on this goroutine's own
// thread, and looks at all windows now and then while a window is awaited or was just taken on (keepWait). It sends
// itself to ready once it runs (nil: it does not), and ends with StopWindowIcon.
func keepWindowIcons(profileDir, exe string, log func(level, msg string), gen int64, ready chan<- *iconKeeper) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var m msg
	pPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmNoRemove) // makes the thread's message queue
	tid, _, _ := pGetCurrentThread.Call()
	id, name, command, icon := windowIdentity(exe, os.Getenv("MPRP_HOME"))
	strs := []windowString{{propAppID, id}, {propRelaunchCommand, command}, {propRelaunchDisplayName, name}, {propRelaunchIcon, icon}}
	k := &iconKeeper{profile: profileDir, tid: tid, self: uint32(os.Getpid()), log: log,
		visible: func(h uintptr) bool { v, _, _ := pIsWindowVisible.Call(h); return v != 0 },
		inspect: describeWindow,
		ours:    func(pid uint32) bool { return processUsesProfile(pid, profileDir) },
		name: func(h uintptr) bool {
			loadWindowIcons(exe) // icons that could not be had before are tried again
			return nameWindow(h, strs)
		},
		check:       checkWindowIcons,
		iconsLoaded: haveWindowIcons,
		hook:        hookProcess,
		unhook:      unhookProcess,
		end:         func() { pPostThreadMessage.Call(tid, wmQuit, 0, 0) }}
	if !startKeeper(k, gen) {
		ready <- nil
		return
	}
	defer endKeeper(k)
	defer k.unhookAll()
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
	if !shellHooked {
		k.warn("hook", "The settings window's icon is only set while it opens: the shell hook could not be set up.")
	}
	ready <- k
	for {
		wait, overdue := keepWait(time.Now(), k.asked, k.settleEnd)
		if overdue {
			k.overdue()
			continue
		}
		ms := uintptr(infinite)
		if wait >= 0 {
			ms = uintptr(wait / time.Millisecond)
		}
		r, _, _ := pMsgWaitForObjects.Call(0, 0, 0, ms, qsAllInput)
		switch r {
		case waitFailed:
			log("INFO", "The settings window's icon is no longer looked after: waiting for events failed.")
			return
		case waitTimeout:
			k.sweep(topWindowHandles())
			continue
		}
		for {
			if got, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove); got == 0 {
				break
			}
			switch {
			case m.message == wmQuit: // StopWindowIcon
				log("INFO", "The settings window's icon is no longer looked after: the program quits.")
				return
			case m.hwnd == 0 && m.message == wmWindowAsked:
				k.windowAsked(time.Now())
				k.sweep(topWindowHandles())
			default:
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
	pPrivateExtractIcons      = user32.NewProc("PrivateExtractIconsW")
	pCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")

	// the program's icons for the window, read once (loadWindowIcons): a window only keeps their handles, so they are
	// kept for the program's life
	windowIconMu sync.Mutex
	windowIcons  [2]uintptr
)

const (
	wmSetIcon = 0x0080
	smCxIcon  = 11 // the width of a large icon on this screen; smCxSmIcon (tray_windows.go) is the small one's
)

// icoImage finds in an .ico file the image for an icon of size px: the one of exactly that size, else the smallest
// larger one, else the largest (nil: none, or not an .ico file).
func icoImage(ico []byte, size int) []byte {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[0:]) != 0 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil
	}
	n := int(binary.LittleEndian.Uint16(ico[4:]))
	var best []byte
	bestW := 0
	for i := 0; i < n; i++ {
		e := 6 + 16*i
		if e+16 > len(ico) {
			return nil
		}
		w := int(ico[e])
		if w == 0 {
			w = 256
		}
		length, off := int(binary.LittleEndian.Uint32(ico[e+8:])), int(binary.LittleEndian.Uint32(ico[e+12:]))
		if length <= 0 || off < 0 || off+length > len(ico) {
			continue
		}
		better := best == nil ||
			(w >= size && (bestW < size || w < bestW)) || // a fitting one, smaller than the fitting one before
			(w < size && bestW < size && w > bestW) // none fits yet: the largest
		if better {
			best, bestW = ico[off:off+length], w
		}
	}
	return best
}

// loadWindowIcons makes the program's icon in the two sizes Windows asks a window for, each at exactly that size
// (the taskbar button and Alt+Tab use the large one, the title bar the small one), from the icon built into the
// program (so a program file that cannot be read yet, while a virus scanner looks at it, does not matter), else from
// the program file. A size that could be made is kept; one that could not is tried again next time. An icon handle is
// valid in every process of the session, so the browser's window can use one this program holds.
func loadWindowIcons(exe string) {
	windowIconMu.Lock()
	defer windowIconMu.Unlock()
	for i, metric := range []uintptr{smCxSmIcon, smCxIcon} {
		if windowIcons[i] != 0 {
			continue
		}
		size, _, _ := pGetSystemMetrics.Call(metric)
		var icon uintptr
		if img := icoImage(assets.Icon, int(size)); img != nil {
			b := append([]byte(nil), img...)
			icon, _, _ = pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 1, 0x00030000, size, size, 0)
		}
		if icon == 0 && exe != "" {
			if path, err := syscall.UTF16PtrFromString(exe); err == nil {
				if n, _, _ := pPrivateExtractIcons.Call(uintptr(unsafe.Pointer(path)), 0, size, size, uintptr(unsafe.Pointer(&icon)), 0, 1, 0); int32(n) < 1 {
					icon = 0
				}
			}
		}
		windowIcons[i] = icon
	}
}

// haveWindowIcons says whether both of the program's icons could be made.
func haveWindowIcons() bool {
	windowIconMu.Lock()
	defer windowIconMu.Unlock()
	return windowIcons[0] != 0 && windowIcons[1] != 0
}

// checkWindowIcons gives the window the icons loadWindowIcons made again where it has others, and reports whether it
// had the program's already. (Setting them when it has them already would make the taskbar button redraw, which the
// shell hook reports: it would never end.)
func checkWindowIcons(hwnd uintptr) bool {
	windowIconMu.Lock()
	want := windowIcons
	windowIconMu.Unlock()
	ok := true
	for i, icon := range want {
		var has uintptr
		r, _, _ := pSendMessageTimeout.Call(hwnd, wmGetIcon, uintptr(i), 0, smtoAbortIfHung, 200, uintptr(unsafe.Pointer(&has)))
		if icon == 0 || r == 0 {
			ok = false
			continue
		}
		if has != icon {
			applyWindowIcons(hwnd)
			return false
		}
	}
	return ok
}

// applyWindowIcons gives the window the icons loadWindowIcons made.
func applyWindowIcons(hwnd uintptr) {
	windowIconMu.Lock()
	defer windowIconMu.Unlock()
	for i, icon := range windowIcons { // i is also WM_SETICON's ICON_SMALL (0) and ICON_BIG (1)
		if icon != 0 {
			pPostMessage.Call(hwnd, wmSetIcon, uintptr(i), icon)
		}
	}
}

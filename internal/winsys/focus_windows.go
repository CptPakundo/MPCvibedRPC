//go:build windows

package winsys

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	// user32 and pSetForegroundWindow are declared in tray_windows.go
	pEnumWindows              = user32.NewProc("EnumWindows")
	pGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	pIsWindowVisible          = user32.NewProc("IsWindowVisible")
	pGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	pIsIconic                 = user32.NewProc("IsIconic")
	pShowWindow               = user32.NewProc("ShowWindow")
	pAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")

	// made once: callbacks are never freed
	enumWindowsCallback = syscall.NewCallback(collectWindow)
	// one enumeration at a time: the callback has no argument of its own
	enumMu    sync.Mutex
	enumFound []topWindow
)

const swRestore = 9

// topWindow is what the choice of a window to bring forward, or to give the program's icon (taskbarWindow), needs to
// know about a top-level window.
type topWindow struct {
	Handle    uintptr
	Pid       uint32
	Visible   bool
	Titled    bool
	Minimised bool
	Child     bool // WS_CHILD: not a top-level window after all (a WinEvent can be about one)
	Owned     bool // it has an owner window (a bubble, a dialog)
	Tool      bool // WS_EX_TOOLWINDOW: never on the taskbar
	AppWindow bool // WS_EX_APPWINDOW: on the taskbar even when owned
}

// pickWindow takes the first window (EnumWindows lists them from the top of the stacking order) that belongs to one
// of the pids, is visible and has a title. The browser keeps helper windows (invisible or without a title) around
// that must not be raised.
func pickWindow(wins []topWindow, pids map[uint32]bool) (topWindow, bool) {
	for _, w := range wins {
		if pids[w.Pid] && w.Visible && w.Titled {
			return w, true
		}
	}
	return topWindow{}, false
}

// collectWindow is the EnumWindows callback: it notes each top-level window; returning 1 goes on with the next.
func collectWindow(hwnd uintptr, _ uintptr) uintptr {
	enumFound = append(enumFound, describeWindow(hwnd))
	return 1
}

const (
	gwOwner        = 4
	wsChild        = 0x40000000
	wsExToolWindow = 0x00000080
	wsExAppWindow  = 0x00040000
)

var (
	pGetWindow        = user32.NewProc("GetWindow")
	pGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	gwlStyle          = ^uintptr(15) // GWL_STYLE (-16)
	gwlExStyle        = ^uintptr(19) // GWL_EXSTYLE (-20)
)

// describeWindow reads what topWindow holds about the window hwnd (a window that is gone has pid 0).
func describeWindow(hwnd uintptr) topWindow {
	var pid uint32
	pGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	vis, _, _ := pIsWindowVisible.Call(hwnd)
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	ic, _, _ := pIsIconic.Call(hwnd)
	owner, _, _ := pGetWindow.Call(hwnd, gwOwner)
	style, _, _ := pGetWindowLongPtr.Call(hwnd, gwlStyle)
	ex, _, _ := pGetWindowLongPtr.Call(hwnd, gwlExStyle)
	return topWindow{Handle: hwnd, Pid: pid, Visible: vis != 0, Titled: n > 0, Minimised: ic != 0,
		Child: style&wsChild != 0, Owned: owner != 0, Tool: ex&wsExToolWindow != 0, AppWindow: ex&wsExAppWindow != 0}
}

// topWindows lists the top-level windows, from the top of the stacking order.
func topWindows() []topWindow {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumFound = nil
	pEnumWindows.Call(enumWindowsCallback, 0)
	wins := enumFound
	enumFound = nil
	return wins
}

// FocusWindow brings the settings window that is already open to the front (restoring it when minimised). It looks
// for the top-level windows of the browser processes that use profileDir, so the user's own browser windows are
// never touched. It returns true when such a window was found, even if Windows only flashed its taskbar button
// because this process may not take the foreground (AllowForeground lets it).
func FocusWindow(profileDir string) bool {
	w, ok := findWindow(profileDir)
	if !ok {
		return false
	}
	if w.Minimised {
		pShowWindow.Call(w.Handle, swRestore)
	}
	pSetForegroundWindow.Call(w.Handle)
	return true
}

// findWindow finds the settings window: the top-level window of the browser processes that use profileDir.
func findWindow(profileDir string) (topWindow, bool) {
	return findWindowWith(profileDir, pickWindow)
}

// findWindowWith finds a top-level window of the browser processes that use profileDir, chosen by pick.
func findWindowWith(profileDir string, pick func([]topWindow, map[uint32]bool) (topWindow, bool)) (topWindow, bool) {
	if profileDir == "" {
		return topWindow{}, false
	}
	pids := map[uint32]bool{}
	forProfileProcesses(profileDir, processQueryLimitedInformation, func(pid uint32, _ syscall.Handle) bool {
		pids[pid] = true
		return false
	})
	if len(pids) == 0 {
		return topWindow{}, false
	}
	return pick(topWindows(), pids)
}

// AllowForeground lets the process pid take the foreground. A second start of the program was launched by the user
// and may; the copy already running may not, so the second start hands that right over before it asks for the window.
func AllowForeground(pid int) {
	pAllowSetForegroundWindow.Call(uintptr(pid))
}

var pIsWindow = user32.NewProc("IsWindow")

// SettingsWindow is the handle of the settings window (0: none is open), for WindowState and CloseWindow.
func SettingsWindow(profileDir string) uintptr {
	if w, ok := findWindow(profileDir); ok {
		return w.Handle
	}
	return 0
}

// WindowState reports whether the window h still exists and whether it is minimised.
func WindowState(h uintptr) (exists, minimised bool) {
	if ok, _, _ := pIsWindow.Call(h); ok == 0 {
		return false, false
	}
	ic, _, _ := pIsIconic.Call(h)
	return true, ic != 0
}

// CloseWindow asks the window h to close, as its X button does (the browser then ends by itself). It reports whether
// the request was delivered (the window may still refuse it).
func CloseWindow(h uintptr) bool {
	r, _, _ := pPostMessage.Call(h, wmClose, 0, 0)
	return r != 0
}

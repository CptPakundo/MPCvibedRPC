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

// topWindow is what the choice of a window to bring forward needs to know about a top-level window.
type topWindow struct {
	Handle    uintptr
	Pid       uint32
	Visible   bool
	Titled    bool
	Minimised bool
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
	var pid uint32
	pGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	vis, _, _ := pIsWindowVisible.Call(hwnd)
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	ic, _, _ := pIsIconic.Call(hwnd)
	enumFound = append(enumFound, topWindow{Handle: hwnd, Pid: pid, Visible: vis != 0, Titled: n > 0, Minimised: ic != 0})
	return 1
}

// FocusWindow brings the settings window that is already open to the front (restoring it when minimised). It looks
// for the top-level windows of the browser processes that use profileDir, so the user's own browser windows are
// never touched. It returns true when such a window was found, even if Windows only flashed its taskbar button
// because this process may not take the foreground (AllowForeground lets it).
func FocusWindow(profileDir string) bool {
	if profileDir == "" {
		return false
	}
	pids := map[uint32]bool{}
	forProfileProcesses(profileDir, processQueryLimitedInformation, func(pid uint32, _ syscall.Handle) bool {
		pids[pid] = true
		return false
	})
	if len(pids) == 0 {
		return false
	}
	enumMu.Lock()
	enumFound = nil
	pEnumWindows.Call(enumWindowsCallback, 0)
	wins := enumFound
	enumFound = nil
	enumMu.Unlock()
	w, ok := pickWindow(wins, pids)
	if !ok {
		return false
	}
	if w.Minimised {
		pShowWindow.Call(w.Handle, swRestore)
	}
	pSetForegroundWindow.Call(w.Handle)
	return true
}

// AllowForeground lets the process pid take the foreground. A second start of the program was launched by the user
// and may; the copy already running may not, so the second start hands that right over before it asks for the window.
func AllowForeground(pid int) {
	pAllowSetForegroundWindow.Call(uintptr(pid))
}

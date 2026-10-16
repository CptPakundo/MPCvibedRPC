//go:build windows

package winsys

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// The tray icon is a plain Win32 notification-area icon driven by a hidden window, with no helper process and no
// libraries beyond the system's own DLLs.

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassEx       = user32.NewProc("RegisterClassExW")
	pCreateWindowEx        = user32.NewProc("CreateWindowExW")
	pDefWindowProc         = user32.NewProc("DefWindowProcW")
	pDestroyWindow         = user32.NewProc("DestroyWindow")
	pGetMessage            = user32.NewProc("GetMessageW")
	pTranslateMessage      = user32.NewProc("TranslateMessage")
	pDispatchMessage       = user32.NewProc("DispatchMessageW")
	pPostMessage           = user32.NewProc("PostMessageW")
	pPostQuitMessage       = user32.NewProc("PostQuitMessage")
	pRegisterWindowMessage = user32.NewProc("RegisterWindowMessageW")
	pCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	pAppendMenu            = user32.NewProc("AppendMenuW")
	pDestroyMenu           = user32.NewProc("DestroyMenu")
	pSetMenuDefaultItem    = user32.NewProc("SetMenuDefaultItem")
	pTrackPopupMenu        = user32.NewProc("TrackPopupMenu")
	pGetCursorPos          = user32.NewProc("GetCursorPos")
	pSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	pLoadImage             = user32.NewProc("LoadImageW")
	pDestroyIcon           = user32.NewProc("DestroyIcon")
	pGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	pSetTimer              = user32.NewProc("SetTimer")
	pKillTimer             = user32.NewProc("KillTimer")
	pShellNotifyIcon       = shell32.NewProc("Shell_NotifyIconW")
	pGetModuleHandle       = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy           = 0x0002
	wmClose             = 0x0010
	wmCommand           = 0x0111
	wmTimer             = 0x0113
	wmLButtonDbl        = 0x0203
	wmRButtonUp         = 0x0205
	wmApp               = 0x8000
	wmTrayCB            = wmApp + 1
	nimAdd              = 0
	nimModify           = 1
	nimDelete           = 2
	nifMessage          = 0x1
	nifIcon             = 0x2
	nifTip              = 0x4
	nifInfo             = 0x10
	niifInfo            = 0x1
	ninBalloonUserClick = 0x405
	mfString            = 0x0
	mfSeparator         = 0x800
	mfChecked           = 0x8
	mfGrayed            = 0x1
	mfDisabled          = 0x2
	tpmRightBtn         = 0x2
	imageIcon           = 1
	lrLoadFile          = 0x10
	smCxSmIcon          = 49
	cmdOpen             = 1
	cmdToggle           = 2
	cmdQuit             = 3
	cmdHide             = 4
	timerID             = 1
	ClassName           = "MPCvibedRPC.Tray"
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type point struct{ x, y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	_       uint32
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

// Tray is the notification-area icon.
type Tray struct {
	o       TrayOptions
	hwnd    uintptr
	icon    uintptr
	created uint32 // the "TaskbarCreated" broadcast: Explorer restarted, add the icon again
	tip     string
	added   bool // the icon is on the taskbar (only touched on the tray's own thread)
	ready   chan struct{}
	done    chan struct{}
	once    sync.Once
}

var current *Tray // the window procedure is a plain callback, so it finds its tray here

func utf16Ptr(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

// StartTray creates the icon on its own thread and returns at once; failures are only logged, the program keeps
// working and can still be reached by starting it again.
func StartTray(o TrayOptions) *Tray {
	if o.Log == nil {
		o.Log = func(string, string) {}
	}
	t := &Tray{o: o, ready: make(chan struct{}), done: make(chan struct{})}
	go t.loop()
	<-t.ready
	return t
}

func (t *Tray) loop() {
	runtime.LockOSThread()
	defer close(t.done)
	signalled := false
	signal := func() {
		if !signalled {
			signalled = true
			close(t.ready)
		}
	}
	defer signal()

	current = t
	hinst, _, _ := pGetModuleHandle.Call(0)
	cls := utf16Ptr(ClassName)
	wc := wndClassEx{lpfnWndProc: syscall.NewCallback(wndProc), hInstance: hinst, lpszClassName: cls}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		t.o.Log("WARN", "Tray icon could not start (RegisterClass: "+err.Error()+"). Open the app again to reach the settings.")
		return
	}
	hwnd, _, err := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(utf16Ptr("MPCvibedRPC"))), 0, 0, 0, 0, 0, 0, 0, hinst, 0)
	if hwnd == 0 {
		t.o.Log("WARN", "Tray icon could not start (CreateWindow: "+err.Error()+"). Open the app again to reach the settings.")
		return
	}
	t.hwnd = hwnd
	m, _, _ := pRegisterWindowMessage.Call(uintptr(unsafe.Pointer(utf16Ptr("TaskbarCreated"))))
	t.created = uint32(m)
	t.loadIcon()
	t.tip = t.tooltip()
	if t.added = t.notify(nimAdd); !t.added {
		t.o.Log("WARN", "Tray icon could not be added yet (is the taskbar running?). It will be added when the taskbar appears.")
	} else {
		t.o.Log("INFO", "Tray icon ready.")
	}
	pSetTimer.Call(hwnd, timerID, 3000, 0)
	signal()

	var mm msg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&mm)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&mm)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&mm)))
	}
	if t.icon != 0 {
		pDestroyIcon.Call(t.icon)
	}
}

func (t *Tray) loadIcon() {
	if len(t.o.Icon) == 0 || t.o.Dir == "" {
		return
	}
	path := filepath.Join(t.o.Dir, "tray.ico")
	if err := os.WriteFile(path, t.o.Icon, 0o644); err != nil {
		return
	}
	sz, _, _ := pGetSystemMetrics.Call(smCxSmIcon)
	h, _, _ := pLoadImage.Call(0, uintptr(unsafe.Pointer(utf16Ptr(path))), imageIcon, sz, sz, lrLoadFile)
	t.icon = h
}

func (t *Tray) tooltip() string {
	if t.o.Tooltip != nil {
		return t.o.Tooltip()
	}
	return "MPCvibedRPC"
}

func (t *Tray) notify(action uintptr) bool {
	var nid notifyIconData
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = t.hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = wmTrayCB
	nid.hIcon = t.icon
	tip := syscall.StringToUTF16(t.tip)
	if len(tip) > len(nid.szTip) {
		tip = append(tip[:len(nid.szTip)-1], 0)
	}
	copy(nid.szTip[:], tip)
	r, _, _ := pShellNotifyIcon.Call(action, uintptr(unsafe.Pointer(&nid)))
	return r != 0
}

// Balloon shows a notification next to the tray icon (a toast on current Windows); clicking it opens the window.
// It returns false when Windows did not accept it.
func (t *Tray) Balloon(title, text string) bool {
	if t == nil {
		return false
	}
	var nid notifyIconData
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = t.hwnd
	nid.uID = 1
	nid.uFlags = nifInfo
	fillUTF16(nid.szInfoTitle[:], title)
	fillUTF16(nid.szInfo[:], text)
	nid.dwInfoFlags = niifInfo
	r, _, _ := pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	return r != 0
}

// fillUTF16 copies s into a fixed buffer, cutting it short if needed and always ending with a zero.
func fillUTF16(dst []uint16, s string) {
	u := syscall.StringToUTF16(s)
	if len(u) > len(dst) {
		u = append(u[:len(dst)-1], 0)
	}
	copy(dst, u)
}

func (t *Tray) showMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	toggle := "Show on Discord"
	if t.o.Running != nil && t.o.Running() {
		toggle = "Stop showing on Discord"
	}
	// the first line says what the program is doing (the icon's tooltip, without the program's name)
	status := strings.TrimPrefix(t.tooltip(), "MPCvibedRPC - ")
	pAppendMenu.Call(menu, mfString|mfDisabled|mfGrayed, 0, uintptr(unsafe.Pointer(utf16Ptr(status))))
	pAppendMenu.Call(menu, mfSeparator, 0, 0)
	pAppendMenu.Call(menu, mfString, cmdOpen, uintptr(unsafe.Pointer(utf16Ptr("Open settings"))))
	pAppendMenu.Call(menu, mfString, cmdToggle, uintptr(unsafe.Pointer(utf16Ptr(toggle))))
	if t.o.OnHide != nil {
		flags := uintptr(mfString)
		if t.o.Hidden != nil && t.o.Hidden() {
			flags |= mfChecked
		}
		pAppendMenu.Call(menu, flags, cmdHide, uintptr(unsafe.Pointer(utf16Ptr("Hide what I'm watching"))))
	}
	pAppendMenu.Call(menu, mfSeparator, 0, 0)
	pAppendMenu.Call(menu, mfString, cmdQuit, uintptr(unsafe.Pointer(utf16Ptr("Quit"))))
	pSetMenuDefaultItem.Call(menu, cmdOpen, 0)
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(t.hwnd) // otherwise the menu does not close when the user clicks elsewhere
	pTrackPopupMenu.Call(menu, tpmRightBtn, uintptr(pt.x), uintptr(pt.y), 0, t.hwnd, 0)
	pPostMessage.Call(t.hwnd, 0, 0, 0) // WM_NULL, as the documentation of TrackPopupMenu asks
}

func (t *Tray) command(id uintptr) {
	var f func()
	switch id {
	case cmdOpen:
		f = t.o.OnOpen
	case cmdToggle:
		f = t.o.OnToggle
	case cmdQuit:
		f = t.o.OnQuit
	case cmdHide:
		f = t.o.OnHide
	}
	if f != nil {
		go f() // never block the message loop on the app's work
	}
}

func wndProc(hwnd, m, wParam, lParam uintptr) uintptr {
	t := current
	if t != nil && hwnd == t.hwnd {
		switch uint32(m) {
		case wmTrayCB:
			switch uint32(lParam) & 0xffff {
			case wmLButtonDbl, ninBalloonUserClick:
				t.command(cmdOpen)
			case wmRButtonUp:
				t.showMenu()
			}
			return 0
		case wmCommand:
			t.command(wParam & 0xffff)
			return 0
		case wmTimer:
			if !t.added {
				// the first try failed (at login the taskbar may come up later, without a TaskbarCreated broadcast)
				t.tip = t.tooltip()
				if t.added = t.notify(nimAdd); t.added {
					t.o.Log("INFO", "Tray icon ready.")
				}
			} else if tip := t.tooltip(); tip != t.tip {
				t.tip = tip
				t.notify(nimModify)
			}
			return 0
		case wmClose:
			pDestroyWindow.Call(hwnd)
			return 0
		case wmDestroy:
			pKillTimer.Call(hwnd, timerID)
			t.notify(nimDelete)
			pPostQuitMessage.Call(0)
			return 0
		}
		if t.created != 0 && uint32(m) == t.created {
			t.added = t.notify(nimAdd)
			return 0
		}
	}
	r, _, _ := pDefWindowProc.Call(hwnd, m, wParam, lParam)
	return r
}

// Close removes the icon and ends the tray thread.
func (t *Tray) Close() {
	if t == nil || t.hwnd == 0 {
		return
	}
	t.once.Do(func() {
		pPostMessage.Call(t.hwnd, wmClose, 0, 0)
		select {
		case <-t.done:
		case <-time.After(1500 * time.Millisecond): // the process is about to exit anyway
		}
	})
}

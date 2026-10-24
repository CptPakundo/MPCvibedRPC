//go:build windows

package potplayer

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// The messages, sent as WM_USER with one of these in wParam. The answers come back as the result, except the file
// path, which PotPlayer sends to the window named in lParam as WM_COPYDATA (UTF-8, dwData = getFileName).
const (
	getTotalTime   = 0x5002 // ms
	getCurrentTime = 0x5004 // ms
	getPlayStatus  = 0x5006 // 0 stopped, 1 paused, 2 playing
	getSpeed       = 0x5015 // 200..12000 (0.2x..12x); 0 from versions without it
	getFileName    = 0x6020
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	pEnumWindows        = user32.NewProc("EnumWindows")
	pGetClassName       = user32.NewProc("GetClassNameW")
	pGetWindowText      = user32.NewProc("GetWindowTextW")
	pSendMessageTimeout = user32.NewProc("SendMessageTimeoutW")
	pRegisterClassEx    = user32.NewProc("RegisterClassExW")
	pCreateWindowEx     = user32.NewProc("CreateWindowExW")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pDefWindowProc      = user32.NewProc("DefWindowProcW")
	pPeekMessage        = user32.NewProc("PeekMessageW")
	pGetModuleHandle    = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmUser          = 0x0400
	wmCopyData      = 0x004A
	smtoAbortIfHung = 0x0002
	pmRemove        = 0x0001
	hwndMessage     = ^uintptr(2) // HWND_MESSAGE, (HWND)-3: a window that only receives messages
	answerWait      = 500         // ms PotPlayer may take for one answer; a hung player is skipped
)

// classes are the window classes of PotPlayer's main window (64-bit and 32-bit builds).
var classes = map[string]bool{"PotPlayer64": true, "PotPlayer": true}

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

// copyData is COPYDATASTRUCT.
type copyData struct {
	dwData uintptr
	cbData uint32
	lpData *byte
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

var (
	mu        sync.Mutex // one query at a time: the windows found and the path received are shared
	setupOnce sync.Once
	replyCls  *uint16
	hinst     uintptr
	enumCB    uintptr
	found     []uintptr
	path      string // the path PotPlayer sent back (WM_COPYDATA)
	gotPath   bool
	noPath    = map[uintptr]bool{} // windows that did not send the path when asked (an older PotPlayer): not asked again
)

func setup() {
	hinst, _, _ = pGetModuleHandle.Call(0)
	replyCls, _ = syscall.UTF16PtrFromString("MPCvibedRPC.PotPlayerReply")
	wc := wndClassEx{lpfnWndProc: syscall.NewCallback(replyProc), hInstance: hinst, lpszClassName: replyCls}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	enumCB = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var buf [64]uint16
		n, _, _ := pGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if classes[syscall.UTF16ToString(buf[:n])] {
			found = append(found, hwnd)
		}
		return 1 // go on
	})
}

// replyProc receives the file path PotPlayer sends back.
func replyProc(hwnd, m, wParam, lParam uintptr) uintptr {
	if m == wmCopyData && lParam != 0 {
		cds := *(**copyData)(unsafe.Pointer(&lParam))
		if cds.dwData == getFileName {
			path, gotPath = "", true
			if cds.lpData != nil && cds.cbData > 0 && cds.cbData < 1<<16 {
				path = string(unsafe.Slice(cds.lpData, cds.cbData))
			}
		}
		return 1
	}
	r, _, _ := pDefWindowProc.Call(hwnd, m, wParam, lParam)
	return r
}

// ask sends one question to a PotPlayer window; ok is false when it did not answer in time.
func ask(hwnd, what, lParam uintptr) (int, bool) {
	var res uintptr
	r, _, _ := pSendMessageTimeout.Call(hwnd, wmUser, what, lParam, smtoAbortIfHung, answerWait, uintptr(unsafe.Pointer(&res)))
	return int(int32(res)), r != 0
}

func windowText(hwnd uintptr) string {
	var buf [512]uint16
	n, _, _ := pGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

// Query asks every open PotPlayer window what it plays (nil when none is open).
func Query() []*core.Info {
	mu.Lock()
	defer mu.Unlock()
	setupOnce.Do(setup)
	found = nil
	pEnumWindows.Call(enumCB, 0)
	if len(found) == 0 {
		return nil
	}
	// The path comes back as a message to a window of ours, handled on the thread that created it while that thread
	// waits for PotPlayer's answer.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	reply, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(replyCls)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, hinst, 0)
	if reply != 0 {
		defer pDestroyWindow.Call(reply)
	}
	var out []*core.Info
	for _, hwnd := range append([]uintptr(nil), found...) {
		status, ok := ask(hwnd, getPlayStatus, 0)
		if !ok {
			continue // hung, or gone
		}
		dur, _ := ask(hwnd, getTotalTime, 0)
		pos, _ := ask(hwnd, getCurrentTime, 0)
		speed, _ := ask(hwnd, getSpeed, 0)
		path, gotPath = "", false
		if reply != 0 && status != 0 && !noPath[hwnd] {
			ask(hwnd, getFileName, reply)
			for end := time.Now().Add(200 * time.Millisecond); !gotPath && time.Now().Before(end); {
				var m msg // a copy sent after the answer is handled here
				if r, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&m)), reply, 0, 0, pmRemove); r == 0 {
					time.Sleep(10 * time.Millisecond)
				}
			}
			noPath[hwnd] = !gotPath
		}
		out = append(out, info(status, pos, dur, speed, path, windowText(hwnd)))
	}
	return out
}

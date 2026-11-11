package potplayer

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"unsafe"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

var (
	pGetMessage     = user32.NewProc("GetMessageW")
	pDispatchMsg    = user32.NewProc("DispatchMessageW")
	pPostThreadMsg  = user32.NewProc("PostThreadMessageW")
	pSendMessage    = user32.NewProc("SendMessageW")
	pGetThreadID    = kernel32.NewProc("GetCurrentThreadId")
	fakeProcPointer = syscall.NewCallback(fakeProc)
	pSetWindowText  = user32.NewProc("SetWindowTextW")
	fakePath        = []byte(`D:\Videos\Sample Show\Sample Show - S01E02.mkv`)
	fakeMute        atomic.Bool // the fake does not send the path while set
	fakeHwnd        uintptr     // the window startFake opened; set before startFake returns
)

// fakeProc answers like PotPlayer's main window: the numbers as results, the path as WM_COPYDATA to the window in
// lParam.
func fakeProc(hwnd, m, wParam, lParam uintptr) uintptr {
	if m != wmUser {
		r, _, _ := pDefWindowProc.Call(hwnd, m, wParam, lParam)
		return r
	}
	switch wParam {
	case getPlayStatus:
		return 2
	case getTotalTime:
		return 1440000
	case getCurrentTime:
		return 61000
	case getSpeed:
		return 1000
	case getFileName:
		if fakeMute.Load() {
			return 0 // a PotPlayer that is still loading, or too old: no path comes back
		}
		cds := copyData{dwData: getFileName, cbData: uint32(len(fakePath)), lpData: &fakePath[0]}
		pSendMessage.Call(lParam, wmCopyData, hwnd, uintptr(unsafe.Pointer(&cds)))
	}
	return 0
}

// startFake opens a window of PotPlayer's class on a thread of its own, with a message loop, until the test ends.
func startFake(t *testing.T) {
	ready := make(chan uintptr)
	go func() {
		runtime.LockOSThread()
		cls, _ := syscall.UTF16PtrFromString("PotPlayer64")
		h, _, _ := pGetModuleHandle.Call(0)
		wc := wndClassEx{lpfnWndProc: fakeProcPointer, hInstance: h, lpszClassName: cls}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		title, _ := syscall.UTF16PtrFromString("Sample Show - S01E02.mkv - PotPlayer")
		hwnd, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, h, 0)
		tid, _, _ := pGetThreadID.Call()
		fakeHwnd = hwnd
		ready <- tid
		if hwnd == 0 {
			return
		}
		var m msg
		for {
			if r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); r == 0 || int32(r) == -1 {
				break
			}
			pDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
		}
		pDestroyWindow.Call(hwnd)
	}()
	tid := <-ready
	t.Cleanup(func() { pPostThreadMsg.Call(tid, 0x0012 /* WM_QUIT */, 0, 0) })
}

// Query finds a PotPlayer window, asks it the documented questions, and receives the file's path.
func TestQueryFakePotPlayer(t *testing.T) {
	startFake(t)
	var got *struct{ file, dir string }
	for _, in := range Query() {
		if in.File == "Sample Show - S01E02.mkv" {
			got = &struct{ file, dir string }{in.File, in.FullDir}
			if in.State != 2 || in.Position != 61000 || in.Duration != 1440000 || in.Rate != 1 {
				t.Fatalf("answers: %+v", in)
			}
		}
	}
	if got == nil {
		t.Fatalf("the fake PotPlayer was not found: %+v", Query())
	}
	if got.dir != `D:\Videos\Sample Show` {
		t.Fatalf("the path did not come back (folder %q)", got.dir)
	}
}

// A window that did not send the path once (PotPlayer still loading) is asked again when it shows something else.
func TestPathAskedAgainWhenTheTitleChanges(t *testing.T) {
	fakeMute.Store(true)
	t.Cleanup(func() { fakeMute.Store(false) })
	startFake(t)
	find := func(title string) *core.Info {
		for _, in := range Query() {
			if in.State == 2 && in.Duration == 1440000 && windowText(fakeHwnd) == title {
				return in
			}
		}
		return nil
	}
	if in := find("Sample Show - S01E02.mkv - PotPlayer"); in == nil || in.FullDir != "" {
		t.Fatalf("no answer should give no folder: %+v", in)
	}
	// still the same text: not asked again, even though the player could answer now
	fakeMute.Store(false)
	if in := find("Sample Show - S01E02.mkv - PotPlayer"); in == nil || in.FullDir != "" {
		t.Fatalf("asked again without a change: %+v", in)
	}
	title, _ := syscall.UTF16PtrFromString("Sample Show - S01E03.mkv - PotPlayer")
	pSetWindowText.Call(fakeHwnd, uintptr(unsafe.Pointer(title)))
	if in := find("Sample Show - S01E03.mkv - PotPlayer"); in == nil || in.FullDir != `D:\Videos\Sample Show` {
		t.Fatalf("a new title should ask again: %+v", in)
	}
}

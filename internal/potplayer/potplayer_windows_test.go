package potplayer

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

var (
	pGetMessage     = user32.NewProc("GetMessageW")
	pDispatchMsg    = user32.NewProc("DispatchMessageW")
	pPostThreadMsg  = user32.NewProc("PostThreadMessageW")
	pSendMessage    = user32.NewProc("SendMessageW")
	pGetThreadID    = kernel32.NewProc("GetCurrentThreadId")
	fakeProcPointer = syscall.NewCallback(fakeProc)
	fakePath        = []byte(`D:\Videos\Sample Show\Sample Show - S01E02.mkv`)
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

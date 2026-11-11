package winsys

import (
	"os"
	"runtime"
	"strings"
	"sync"
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

	nameWindowFor   = 15 * time.Second       // how long a window that was just opened is looked for
	nameWindowEvery = 250 * time.Millisecond // and how often
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

// NameWindow gives the settings window that was just opened with profileDir the program's own taskbar identity. The
// window takes a moment to appear, so it is looked for until it is there (at most nameWindowFor); the caller runs
// this on a goroutine of its own. It reports whether a window was named.
func NameWindow(profileDir string) bool {
	exe, err := os.Executable()
	if err != nil || profileDir == "" {
		return false
	}
	id, name, command, icon := windowIdentity(exe, os.Getenv("MPCRPC_HOME"))
	for deadline := time.Now().Add(nameWindowFor); ; time.Sleep(nameWindowEvery) {
		if w, ok := findWindow(profileDir); ok {
			// the ID first: the other three belong to it
			named := setWindowStrings(w.Handle, []windowString{
				{propAppID, id}, {propRelaunchCommand, command}, {propRelaunchDisplayName, name}, {propRelaunchIcon, icon},
			})
			// The browser gives its window the page's icon in a small size, which the taskbar enlarges. It does so
			// when the page has loaded, which may be after the window was found: the program's icon is set now and
			// twice more.
			for _, wait := range []time.Duration{0, time.Second, 3 * time.Second} {
				time.Sleep(wait)
				setWindowIcons(w.Handle, exe)
			}
			return named
		}
		if time.Now().After(deadline) {
			return false
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

	// the icons the window was last given: they must outlive the call (the window only keeps their handles), and the
	// ones before them are freed when new ones are set
	windowIconMu sync.Mutex
	windowIcons  [2]uintptr
)

const (
	wmSetIcon = 0x0080
	smCxIcon  = 11 // the width of a large icon on this screen; smCxSmIcon (tray_windows.go) is the small one's
)

// setWindowIcons gives the window the program file's own icon in the two sizes Windows asks a window for, each read
// at exactly that size (the taskbar button and Alt+Tab use the large one, the title bar the small one). An icon
// handle is valid in every process of the session, so the browser's window can use one this program holds.
func setWindowIcons(hwnd uintptr, exe string) {
	path, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return
	}
	windowIconMu.Lock()
	defer windowIconMu.Unlock()
	for i, metric := range []uintptr{smCxSmIcon, smCxIcon} { // i is also WM_SETICON's ICON_SMALL (0) and ICON_BIG (1)
		size, _, _ := pGetSystemMetrics.Call(metric)
		var icon uintptr
		if n, _, _ := pPrivateExtractIcons.Call(uintptr(unsafe.Pointer(path)), 0, size, size, uintptr(unsafe.Pointer(&icon)), 0, 1, 0); int32(n) < 1 || icon == 0 {
			continue // a program file without an icon (a plain development build): the browser's stays
		}
		pPostMessage.Call(hwnd, wmSetIcon, uintptr(i), icon)
		if old := windowIcons[i]; old != 0 {
			// freed a moment later: the window may still be drawing with it
			time.AfterFunc(5*time.Second, func() { pDestroyIcon.Call(old) })
		}
		windowIcons[i] = icon
	}
}

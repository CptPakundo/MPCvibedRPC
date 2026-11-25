// Package winsys is everything that touches the operating system: run-at-login, the players' web-interface switch
// and connections, the settings window and the tray icon. Windows is the main system; macOS and Linux have their own
// versions of each part where there is one (a LaunchAgent or an autostart entry, mpv.conf, IINA's settings, the
// default browser) and do without the rest (no registry; no tray icon on Linux).
package winsys

import (
	"runtime"
	"sync/atomic"
)

// IsWindows is true on Windows.
const IsWindows = runtime.GOOS == "windows"

// programFile is the program file's path once the program has renamed it (SetProgramFile; nil: os.Executable).
var programFile atomic.Pointer[string]

// SetProgramFile tells this package the program file's new path after the program renamed its own file: on Windows
// os.Executable goes on naming the file the program was started from, and a pinned settings window must start the
// file that is there.
func SetProgramFile(path string) { programFile.Store(&path) }

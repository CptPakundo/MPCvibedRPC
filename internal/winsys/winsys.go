// Package winsys is everything that touches the operating system: run-at-login, the players' web-interface switch
// and connections, the settings window and the tray icon. Windows is the main system; macOS and Linux have their own
// versions of each part where there is one (a LaunchAgent or an autostart entry, mpv.conf, IINA's settings, the
// default browser) and do without the rest (no registry; no tray icon on Linux).
package winsys

import "runtime"

// IsWindows is true on Windows.
const IsWindows = runtime.GOOS == "windows"

//go:build !windows

package winsys

// NameWindow does nothing here: only Windows takes the settings window's taskbar button for the browser's. (macOS
// shows the settings in a window of the program's own.)
func NameWindow(profileDir string, log func(level, msg string)) bool { return false }

// StopWindowIcon does nothing here (see NameWindow).
func StopWindowIcon() {}

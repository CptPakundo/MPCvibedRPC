//go:build !windows

package winsys

// NameWindow does nothing here: only Windows takes the settings window's taskbar button for the browser's. (macOS
// shows the settings in a window of the program's own.)
func NameWindow(profileDir string) bool { return false }

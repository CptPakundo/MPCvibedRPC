//go:build !windows

package winsys

// FocusWindow reports false: only Windows can find the browser window of the settings by its profile. The caller
// then opens a window as usual (on macOS the native window is simply shown again).
func FocusWindow(profileDir string) bool { return false }

// AllowForeground does nothing here: only Windows restricts which process may take the foreground.
func AllowForeground(pid int) {}

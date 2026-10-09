//go:build !windows

package winsys

// CloseWindowBrowser does nothing outside Windows (the settings window opens in the default browser there).
func CloseWindowBrowser(profileDir string) int { return 0 }

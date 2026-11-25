package main

import "testing"

// The settings window's going quits the program only when the user closed the program's own window.
func TestCloseQuits(t *testing.T) {
	for _, c := range []struct {
		name                        string
		windows                     int32
		appWindow, inTray, quitting bool
		goos                        string
		want                        bool
	}{
		{"closed with its X", 0, true, false, false, "windows", true},
		{"Linux app window", 0, true, false, false, "linux", true},
		{"a window is open again (a reload)", 1, true, false, false, "windows", false},
		{"a tab of the user's browser", 0, false, false, false, "windows", false},
		{"minimised to the tray", 0, true, true, false, "windows", false},
		{"quitting already", 0, true, false, true, "windows", false},
		{"macOS closes to the menu bar", 0, true, false, false, "darwin", false},
	} {
		if got := closeQuits(c.windows, c.appWindow, c.inTray, c.quitting, c.goos); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

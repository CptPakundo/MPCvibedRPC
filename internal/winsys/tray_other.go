//go:build !windows

package winsys

// TrayOptions describes the tray icon.
type TrayOptions struct {
	Icon     []byte
	Dir      string
	Tooltip  func() string
	Running  func() bool
	OnOpen   func()
	OnToggle func()
	OnQuit   func()
	Log      func(level, msg string)
}

// Tray is the notification-area icon (Windows only).
type Tray struct{}

// StartTray does nothing outside Windows.
func StartTray(TrayOptions) *Tray { return nil }

// Close removes the icon.
func (t *Tray) Close() {}

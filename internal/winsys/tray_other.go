//go:build !windows && !(darwin && cgo)

package winsys

// TrayOptions describes the tray icon.
type TrayOptions struct {
	Icon     []byte
	Dir      string
	Tooltip  func() string
	Running  func() bool
	OnOpen   func()
	OnToggle func()
	Hidden   func() bool // whether "Hide what I'm watching" is on (its menu item is ticked)
	OnHide   func()      // switches "Hide what I'm watching" over
	OnQuit   func()
	Log      func(level, msg string)
}

// Tray is the tray icon (Windows) or menu bar icon (macOS); on the other systems there is none.
type Tray struct{}

// StartTray does nothing here (no tray icon on this system, or macOS built without Cocoa).
func StartTray(TrayOptions) *Tray { return nil }

// Close removes the icon.
func (t *Tray) Close() {}

// Balloon shows a notification (Windows only; here it does nothing).
func (t *Tray) Balloon(title, text string) bool { return false }

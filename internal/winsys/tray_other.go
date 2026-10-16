//go:build !windows && !(darwin && cgo)

package winsys

// Tray is the tray icon (Windows) or menu bar icon (macOS); on the other systems there is none.
type Tray struct{}

// StartTray does nothing here (no tray icon on this system, or macOS built without Cocoa).
func StartTray(TrayOptions) *Tray { return nil }

// Close removes the icon.
func (t *Tray) Close() {}

// Balloon shows a notification (Windows only; here it does nothing).
func (t *Tray) Balloon(title, text string) bool { return false }

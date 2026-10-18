package winsys

// TrayOptions describes the tray icon (Windows) or menu bar icon (macOS).
type TrayOptions struct {
	Icon     []byte
	Badged   []byte      // the icon while Badge says so (Windows; macOS draws its own dot)
	Badge    func() bool // an update is waiting: the icon gets its dot
	Dir      string
	Tooltip  func() string // text shown on hover
	Running  func() bool   // decides "Stop showing on Discord" / "Show on Discord"
	OnOpen   func()
	OnToggle func()
	Hidden   func() bool // whether "Hide what I'm watching" is on (its menu item is ticked)
	OnHide   func()      // switches "Hide what I'm watching" over
	OnQuit   func()
	Log      func(level, msg string)
}

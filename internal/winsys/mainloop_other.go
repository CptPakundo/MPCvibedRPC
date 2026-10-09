//go:build !(darwin && cgo)

package winsys

// macApp is false here: only the macOS build (macapp_darwin.go) has a window and menu bar icon of its own.
const macApp = false

// RunMain runs the program (f). The macOS build runs it beside its Cocoa event loop instead.
func RunMain(f func()) { f() }

func openNativeWindow(string) bool { return false }

func closeNativeWindow() {}

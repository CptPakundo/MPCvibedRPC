//go:build !windows && !(darwin && cgo)

package winsys

// closeNativeWindow closes the macOS app's own window; there is none here. (Windows needs no stand-in: only the code
// for the other systems calls it.)
func closeNativeWindow() {}

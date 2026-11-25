//go:build darwin && cgo

package winsys

import "testing"

// A program that is not an app bundle (this test binary, a bare build) has no bundle identifier: it cannot notify, and
// asking must say so rather than stop it with the notification centre's exception.
func TestBalloonWithoutBundle(t *testing.T) {
	if (&Tray{}).Balloon("MPRP 9.9.9 is available", "text") {
		t.Error("a bare program reported a notification as shown")
	}
	var none *Tray
	if none.Balloon("title", "text") {
		t.Error("no menu bar icon, yet a notification was shown")
	}
}

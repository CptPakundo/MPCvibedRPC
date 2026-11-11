package winsys

import "testing"

func TestWindowIdentity(t *testing.T) {
	const exe = `C:\Programs\Sample\MPCvibedRPC.exe`
	id, name, command, icon := windowIdentity(exe, "")
	if id != "MPCvibedRPC" || name != "MPCvibedRPC" || command != `"`+exe+`"` || icon != exe+",0" {
		t.Errorf("the installed copy: id %q, name %q, command %q, icon %q", id, name, command, icon)
	}
	// a copy with a data folder of its own: another button group, and a pinned button starts it with that folder
	id2, name2, command2, _ := windowIdentity(exe, `D:\Portable\data\`)
	if id2 == id || id2 != runNameFor(`D:\Portable\data`) {
		t.Errorf("a copy with its own folder has the ID %q (the installed one: %q)", id2, id)
	}
	if name2 != "MPCvibedRPC" || command2 != `"`+exe+`" --home "D:\Portable\data"` {
		t.Errorf("a copy with its own folder: name %q, command %q", name2, command2)
	}
}

// A window that is not there is not named, and nothing is waited for without a profile.
func TestNameWindowWithoutAWindow(t *testing.T) {
	if NameWindow("") {
		t.Error("a window was named without a profile")
	}
	if setWindowStrings(0, []windowString{{propAppID, "Sample"}}) {
		t.Error("properties were set on no window")
	}
}

package winsys

import "testing"

// The single-instance mutexes follow the former variable too: the updated copy waits for the old copy's own mutex,
// never for the installed program's.
func TestInstanceNamesFollowTheFormerHomeVariable(t *testing.T) {
	onlyFormerHome(t, `C:\Temp\Home`)
	got := instanceNames()
	if len(got) != 2 || got[0] != `Local\MPRP-3e950f5e-single-instance` || got[1] != `Local\`+formerRunName()+`-single-instance` || formerRunName() == "MPCvibedRPC" {
		t.Errorf("instanceNames() = %q", got)
	}
}

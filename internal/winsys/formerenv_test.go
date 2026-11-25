package winsys

import (
	"os"
	"testing"

	"github.com/CptPakundo/MPRP/internal/core"
)

// onlyFormerHome is the environment a copy of the former version gives the copy it updated itself to, when it has a
// data folder of its own: MPCRPC_HOME and no MPRP_HOME. main adopts it first (core.AdoptFormerEnv).
func onlyFormerHome(t *testing.T, home string) {
	t.Setenv("MPRP_HOME", "")
	os.Unsetenv("MPRP_HOME")
	t.Setenv("MPCRPC_HOME", home)
	core.AdoptFormerEnv()
}

// The run-at-login value follows the former variable, so the updated copy never touches the installed program's.
func TestRunNameFollowsTheFormerHomeVariable(t *testing.T) {
	onlyFormerHome(t, `C:\Temp\Home`)
	if got := runName(); got != "MPRP-3e950f5e" {
		t.Errorf("runName() = %q, want the custom folder's name", got)
	}
	if got := formerRunName(); got != nameFor(core.FormerName, `C:\Temp\Home`) {
		t.Errorf("formerRunName() = %q", got)
	}
}

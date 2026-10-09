package winsys

import "testing"

func TestRunNameFor(t *testing.T) {
	if got := runNameFor(""); got != "MPCvibedRPC" {
		t.Errorf("the installed program keeps the plain name, got %q", got)
	}
	if got := runNameFor("   "); got != "MPCvibedRPC" {
		t.Errorf("blank counts as unset, got %q", got)
	}
	// values computed independently in PowerShell: tools/ci/smoke.ps1 derives the same name for its own data folder
	for home, want := range map[string]string{
		`C:\Temp\Home`:                 "MPCvibedRPC-3e950f5e",
		`C:\Temp\HOME\`:                "MPCvibedRPC-3e950f5e", // case and a trailing separator do not matter
		`c:\temp\home/`:                "MPCvibedRPC-3e950f5e",
		`D:\Portable\MPCvibedRPC\data`: "MPCvibedRPC-649c6423",
	} {
		if got := runNameFor(home); got != want {
			t.Errorf("runNameFor(%q) = %q, want %q", home, got, want)
		}
	}
	if runNameFor(`C:\a`) == runNameFor(`C:\b`) {
		t.Error("different data folders must get different names")
	}
}

func TestRunNameFollowsTheEnvironment(t *testing.T) {
	t.Setenv("MPCRPC_HOME", "")
	if runName() != "MPCvibedRPC" {
		t.Errorf("no custom folder: %q", runName())
	}
	t.Setenv("MPCRPC_HOME", `C:\Temp\Home`)
	if runName() != "MPCvibedRPC-3e950f5e" {
		t.Errorf("custom folder: %q", runName())
	}
}
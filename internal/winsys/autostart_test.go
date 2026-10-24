package winsys

import "testing"

func TestRunName(t *testing.T) {
	// values computed independently in PowerShell: tools/ci/smoke.ps1 derives the same name for its own data folder
	for _, c := range []struct{ home, want, why string }{
		{"", "MPCvibedRPC", "the installed program keeps the plain name"},
		{"   ", "MPCvibedRPC", "blank counts as unset"},
		{`C:\Temp\Home`, "MPCvibedRPC-3e950f5e", "a custom folder"},
		{`C:\Temp\HOME\`, "MPCvibedRPC-3e950f5e", "case and a trailing separator do not matter"},
		{`c:\temp\home/`, "MPCvibedRPC-3e950f5e", "case and a trailing separator do not matter"},
		{`D:\Portable\MPCvibedRPC\data`, "MPCvibedRPC-649c6423", "another custom folder"},
	} {
		if got := runNameFor(c.home); got != c.want {
			t.Errorf("runNameFor(%q) = %q, want %q (%s)", c.home, got, c.want, c.why)
		}
		// the name follows the environment
		t.Setenv("MPCRPC_HOME", c.home)
		if got := runName(); got != c.want {
			t.Errorf("runName() with MPCRPC_HOME=%q = %q, want %q (%s)", c.home, got, c.want, c.why)
		}
	}
	if runNameFor(`C:\a`) == runNameFor(`C:\b`) {
		t.Error("different data folders must get different names")
	}
}

func TestRunEntryNames(t *testing.T) {
	for _, c := range []struct {
		command, exe string
		want         bool
	}{
		{`"C:\Apps\MPCvibedRPC.exe" --background`, `C:\Other\MPCvibedRPC.exe`, true}, // the folder is not compared
		{`"c:\apps\mpcvibedrpc.EXE" --background`, `MPCvibedRPC.exe`, true},          // case does not matter
		{`"D:\Portable\MPCvibedRPC.exe" --background --home "D:\Portable\data"`, `MPCvibedRPC.exe`, true},
		{`"C:\Apps\OtherProgram.exe" --background`, `MPCvibedRPC.exe`, false},
		{``, `MPCvibedRPC.exe`, false},
	} {
		if got := runEntryNames(c.command, c.exe); got != c.want {
			t.Errorf("runEntryNames(%q, %q) = %v, want %v", c.command, c.exe, got, c.want)
		}
	}
}

func TestRunCommandCarriesTheDataFolder(t *testing.T) {
	for _, c := range []struct{ name, exe, home, want string }{
		{"default copy", `C:\Apps\MPCvibedRPC.exe`, "", `"C:\Apps\MPCvibedRPC.exe" --background`},
		{"own data folder", `D:\Portable\MPCvibedRPC.exe`, `D:\Portable\data\`, `"D:\Portable\MPCvibedRPC.exe" --background --home "D:\Portable\data"`},
	} {
		if got := runCommand(c.exe, c.home); got != c.want {
			t.Errorf("%s: %s", c.name, got)
		}
	}
}

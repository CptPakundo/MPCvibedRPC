package winsys

import "testing"

func TestRunName(t *testing.T) {
	// values computed independently (PowerShell, sha256sum): tools/ci/smoke.ps1 derives the same name for its own data folder
	for _, c := range []struct{ home, want, why string }{
		{"", "MPRP", "the installed program keeps the plain name"},
		{"   ", "MPRP", "blank counts as unset"},
		{`C:\Temp\Home`, "MPRP-3e950f5e", "a custom folder"},
		{`C:\Temp\HOME\`, "MPRP-3e950f5e", "case and a trailing separator do not matter"},
		{`c:\temp\home/`, "MPRP-3e950f5e", "case and a trailing separator do not matter"},
		{`D:\Portable\MPRP\data`, "MPRP-dd9ad258", "another custom folder"},
	} {
		if got := runNameFor(c.home); got != c.want {
			t.Errorf("runNameFor(%q) = %q, want %q (%s)", c.home, got, c.want, c.why)
		}
		// the name follows the environment
		t.Setenv("MPRP_HOME", c.home)
		if got := runName(); got != c.want {
			t.Errorf("runName() with MPRP_HOME=%q = %q, want %q (%s)", c.home, got, c.want, c.why)
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
		{`"C:\Apps\MPRP.exe" --background`, `C:\Other\MPRP.exe`, true}, // the folder is not compared
		{`"c:\apps\mprp.EXE" --background`, `MPRP.exe`, true},          // case does not matter
		{`"D:\Portable\MPRP.exe" --background --home "D:\Portable\data"`, `MPRP.exe`, true},
		{`"C:\Apps\OtherProgram.exe" --background`, `MPRP.exe`, false},
		{``, `MPRP.exe`, false},
	} {
		if got := runEntryNames(c.command, c.exe); got != c.want {
			t.Errorf("runEntryNames(%q, %q) = %v, want %v", c.command, c.exe, got, c.want)
		}
	}
}

func TestRunCommandCarriesTheDataFolder(t *testing.T) {
	for _, c := range []struct{ name, exe, home, want string }{
		{"default copy", `C:\Apps\MPRP.exe`, "", `"C:\Apps\MPRP.exe" --background`},
		{"own data folder", `D:\Portable\MPRP.exe`, `D:\Portable\data\`, `"D:\Portable\MPRP.exe" --background --home "D:\Portable\data"`},
	} {
		if got := runCommand(c.exe, c.home); got != c.want {
			t.Errorf("%s: %s", c.name, got)
		}
	}
}

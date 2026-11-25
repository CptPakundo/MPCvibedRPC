package winsys

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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

// fakeRegistry is a runRegistry over two maps, as the migration sees the Run key and StartupApproved\Run.
type fakeRegistry struct {
	runs      map[string]string
	approvals map[string][]byte
	refuse    bool // every write fails
}

func (f *fakeRegistry) registry() runRegistry {
	return runRegistry{
		run:      func(n string) (string, bool) { v, ok := f.runs[n]; return v, ok },
		approved: func(n string) ([]byte, bool) { v, ok := f.approvals[n]; return v, ok },
		setRun: func(n, c string) bool {
			if f.refuse {
				return false
			}
			f.runs[n] = c
			return true
		},
		setApproved: func(n string, d []byte) bool {
			if f.refuse {
				return false
			}
			f.approvals[n] = d
			return true
		},
		deleteRun:      func(n string) bool { delete(f.runs, n); return true },
		deleteApproved: func(n string) bool { delete(f.approvals, n); return true },
	}
}

func TestFormerRunName(t *testing.T) {
	t.Setenv("MPRP_HOME", "")
	if n := formerRunName(); n != "MPCvibedRPC" {
		t.Errorf("formerRunName() = %q", n)
	}
	t.Setenv("MPRP_HOME", `C:\Temp\Home`)
	if n := formerRunName(); n != "MPCvibedRPC-3e950f5e" { // the same folder gives the same suffix as runNameFor
		t.Errorf("formerRunName() with a data folder = %q", n)
	}
}

func TestMigrateRunValue(t *testing.T) {
	const cmd = `"C:\Apps\MPRP.exe" --background`
	disabled, enabled := []byte{3, 0, 0, 0}, []byte{2, 0, 0, 0}
	for _, c := range []struct {
		name          string
		runs          map[string]string
		approvals     map[string][]byte
		refuse        bool
		wantRuns      map[string]string
		wantApprovals map[string][]byte
		wantNote      string
	}{
		{name: "nothing of the former name",
			runs: map[string]string{"Other": "x"}, wantRuns: map[string]string{"Other": "x"}},
		{name: "on: moved, pointing at this program file",
			runs:     map[string]string{"MPCvibedRPC": `"C:\Old\MPCvibedRPC.exe" --background`},
			wantRuns: map[string]string{"MPRP": cmd}, wantNote: "Moved the run-at-login entry MPCvibedRPC to MPRP"},
		{name: "switched off in Task Manager: stays off",
			runs: map[string]string{"MPCvibedRPC": "x"}, approvals: map[string][]byte{"MPCvibedRPC": disabled},
			wantRuns: map[string]string{"MPRP": cmd}, wantApprovals: map[string][]byte{"MPRP": disabled},
			wantNote: "stays switched off"},
		{name: "switched on in Task Manager: the switch comes along",
			runs: map[string]string{"MPCvibedRPC": "x"}, approvals: map[string][]byte{"MPCvibedRPC": enabled},
			wantRuns: map[string]string{"MPRP": cmd}, wantApprovals: map[string][]byte{"MPRP": enabled},
			wantNote: "Moved"},
		{name: "a value of the new name is kept",
			runs:     map[string]string{"MPCvibedRPC": "old", "MPRP": "mine"},
			wantRuns: map[string]string{"MPRP": "mine"}, wantNote: "Moved"},
		{name: "a leftover switch without its value goes",
			approvals: map[string][]byte{"MPCvibedRPC": disabled}, wantNote: "leftover"},
		{name: "writing fails: the former entry stays as it was",
			runs: map[string]string{"MPCvibedRPC": "x"}, approvals: map[string][]byte{"MPCvibedRPC": disabled}, refuse: true,
			wantRuns: map[string]string{"MPCvibedRPC": "x"}, wantApprovals: map[string][]byte{"MPCvibedRPC": disabled},
			wantNote: "left as it was"},
	} {
		f := &fakeRegistry{runs: map[string]string{}, approvals: map[string][]byte{}, refuse: c.refuse}
		for k, v := range c.runs {
			f.runs[k] = v
		}
		for k, v := range c.approvals {
			f.approvals[k] = v
		}
		note := migrateRunValue(f.registry(), "MPCvibedRPC", "MPRP", cmd)
		if (c.wantNote == "") != (note == "") || !strings.Contains(note, c.wantNote) {
			t.Errorf("%s: note %q, want one with %q", c.name, note, c.wantNote)
		}
		if c.wantRuns == nil {
			c.wantRuns = map[string]string{}
		}
		if c.wantApprovals == nil {
			c.wantApprovals = map[string][]byte{}
		}
		if !reflect.DeepEqual(f.runs, c.wantRuns) || !reflect.DeepEqual(f.approvals, c.wantApprovals) {
			t.Errorf("%s: runs %v approvals %v, want %v %v", c.name, f.runs, f.approvals, c.wantRuns, c.wantApprovals)
		}
	}
}

func TestMigrateLoginItem(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		t.Setenv("MPRP_HOME", "")
		home := t.TempDir()
		exe := "/opt/MPRP/MPRP"
		from := loginItemPathNamed("MPCvibedRPC", goos, home, "")
		to := loginItemPath(goos, home, "")
		if note := migrateLoginItem(goos, home, "", exe, ""); note != "" {
			t.Errorf("%s: nothing to move, but %q", goos, note)
		}
		os.MkdirAll(filepath.Dir(from), 0o755)
		os.WriteFile(from, []byte("the former login item"), 0o644)
		note := migrateLoginItem(goos, home, "", exe, "")
		if !strings.HasPrefix(note, "Moved the login item") {
			t.Fatalf("%s: note %q", goos, note)
		}
		if _, err := os.Stat(from); err == nil {
			t.Errorf("%s: the former login item is still there", goos)
		}
		b, err := os.ReadFile(to)
		if err != nil || string(b) != loginItem(goos, exe, "") {
			t.Errorf("%s: new login item %q (%v)", goos, b, err)
		}
		// a translocated macOS app cannot be started at login: the former item stays
		if goos == "darwin" {
			os.Remove(to)
			os.WriteFile(from, []byte("the former login item"), 0o644)
			if note := migrateLoginItem(goos, home, "", "/private/var/folders/x/AppTranslocation/y/MPRP.app/Contents/MacOS/MPRP", ""); !strings.Contains(note, "left as it was") {
				t.Errorf("translocated: note %q", note)
			}
			if _, err := os.Stat(from); err != nil {
				t.Error("translocated: the former login item was removed")
			}
		}
	}
}

// After the program renamed its file, the Run value that named the old path starts the new one; any other value, and
// the Task Manager switch, stay as they are.
func TestFollowRunValue(t *testing.T) {
	oldExe, newExe := `C:\Apps\MPCvibedRPC.exe`, `C:\Apps\MPRP.exe`
	f := &fakeRegistry{runs: map[string]string{"MPRP": `"C:\Apps\mpcvibedrpc.exe" --background`, "Other": `"C:\Other\MPCvibedRPC.exe"`},
		approvals: map[string][]byte{"MPRP": {3, 0}}}
	note := followRunValue(f.registry(), "MPRP", oldExe, newExe, "")
	if f.runs["MPRP"] != `"C:\Apps\MPRP.exe" --background` || !strings.Contains(note, "now starts") {
		t.Errorf("run %q, note %q", f.runs["MPRP"], note)
	}
	if f.runs["Other"] != `"C:\Other\MPCvibedRPC.exe"` || f.approvals["MPRP"][0] != 3 {
		t.Error("another value or the Task Manager switch was changed")
	}
	// a copy with a data folder of its own keeps --home
	f.runs["MPRP-1"] = `"C:\Apps\MPCvibedRPC.exe" --background --home "D:\Data"`
	followRunValue(f.registry(), "MPRP-1", oldExe, newExe, `D:\Data`)
	if f.runs["MPRP-1"] != `"C:\Apps\MPRP.exe" --background --home "D:\Data"` {
		t.Errorf("with a data folder: %q", f.runs["MPRP-1"])
	}
	// a value naming another file, or none at all: nothing to do
	if note := followRunValue(f.registry(), "Other", `C:\Apps\MPCvibedRPC.exe`, newExe, ""); note != "" || f.runs["Other"] != `"C:\Other\MPCvibedRPC.exe"` {
		t.Errorf("another file's value: %q", note)
	}
	if note := followRunValue(f.registry(), "None", oldExe, newExe, ""); note != "" {
		t.Errorf("no value: %q", note)
	}
	f.runs["MPRP"] = `"C:\Apps\MPCvibedRPC.exe" --background`
	f.refuse = true
	if note := followRunValue(f.registry(), "MPRP", oldExe, newExe, ""); !strings.Contains(note, "Could not") {
		t.Errorf("a write that fails: %q", note)
	}
}

func TestFollowLoginItem(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		dir := t.TempDir()
		p := filepath.Join(dir, "item")
		oldExe, newExe := "/opt/x/MPCvibedRPC-linux-amd64", "/opt/x/MPRP-linux-amd64"
		if note := followLoginItem(goos, p, oldExe, newExe, ""); note != "" {
			t.Errorf("%s: no login item, but %q", goos, note)
		}
		os.WriteFile(p, []byte(loginItem(goos, "/opt/y/other", "")), 0o644)
		if note := followLoginItem(goos, p, oldExe, newExe, ""); note != "" {
			t.Errorf("%s: another program's login item, but %q", goos, note)
		}
		os.WriteFile(p, []byte(loginItem(goos, oldExe, "/data")), 0o644)
		note := followLoginItem(goos, p, oldExe, newExe, "/data")
		if b, _ := os.ReadFile(p); string(b) != loginItem(goos, newExe, "/data") || !strings.Contains(note, "now starts") {
			t.Errorf("%s: login item %q, note %q", goos, b, note)
		}
	}
}

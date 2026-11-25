package winsys

import (
	"os"
	"strings"
	"testing"
)

// The registry read gives what `reg query` gave. Read-only: it only looks at values, and never writes.
func TestRegistryRunValueMatchesRegExe(t *testing.T) {
	if _, ok := registryRunValue("MPRP-test-no-such-value"); ok {
		t.Error("a value that does not exist was found")
	}
	for _, name := range []string{"MPRP", "MPRP-test-no-such-value"} {
		r := run("reg", "query", runKey, "/v", name)
		data, ok := registryRunValue(name)
		if ok != r.ok {
			t.Errorf("%s: registry read found=%v, reg query found=%v", name, ok, r.ok)
			continue
		}
		// the same data appears in reg's output (after the type), whatever the program is
		if ok && !strings.Contains(r.stdout, data) {
			t.Errorf("%s: reg query output does not contain the value read from the registry", name)
		}
	}
}

// End to end against the real registry: opt-in, because it writes the Run value that belongs to a throwaway data
// folder (MPRP_HOME) and removes it again. It never touches the plain "MPRP" value.
func TestAutostartRegistryRoundTrip(t *testing.T) {
	if os.Getenv("MPRP_TEST_REGISTRY") != "1" {
		t.Skip("set MPRP_TEST_REGISTRY=1 to write and remove a Run value for a throwaway data folder")
	}
	t.Setenv("MPRP_HOME", t.TempDir())
	if runName() == runNameBase {
		t.Fatal("the test must use a value name of its own")
	}
	exe := `C:\Test\MPRP.exe`
	t.Cleanup(func() { SetAutoStart(false, exe) })
	if GetAutoStart(exe) {
		t.Fatal("on before it was set")
	}
	if !SetAutoStart(true, exe) {
		t.Fatal("could not set the Run value")
	}
	if !GetAutoStart(exe) {
		t.Error("the Run value that was just set is not read back")
	}
	if GetAutoStart(`C:\Test\SomethingElse.exe`) {
		t.Error("a different program name must not match")
	}
	data, ok := registryRunValue(runName())
	if !ok || data != runCommand(exe, os.Getenv("MPRP_HOME")) {
		t.Errorf("stored command %q, want %q", data, runCommand(exe, os.Getenv("MPRP_HOME")))
	}
	SetAutoStart(false, exe)
	if GetAutoStart(exe) {
		t.Error("still on after removing it")
	}
}

// The migration from the former name against the real registry: opt-in like the test above, and only with the values
// of a throwaway data folder (never the plain "MPRP" or former-name values).
func TestAutostartMigrationRegistry(t *testing.T) {
	if os.Getenv("MPRP_TEST_REGISTRY") != "1" {
		t.Skip("set MPRP_TEST_REGISTRY=1 to write and remove Run values for a throwaway data folder")
	}
	t.Setenv("MPRP_HOME", t.TempDir())
	from, to := formerRunName(), runName()
	if to == runNameBase || !strings.Contains(from, "-") {
		t.Fatal("the test must use value names of its own")
	}
	exe := `C:\Test\MPRP.exe`
	r := systemRegistry
	t.Cleanup(func() {
		for _, n := range []string{from, to} {
			r.deleteRun(n)
			r.deleteApproved(n)
		}
	})
	if !r.setRun(from, `"C:\Test\Old.exe" --background`) || !r.setApproved(from, []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("could not write the former values")
	}
	if note := MigrateAutoStart(exe); !strings.Contains(note, "stays switched off") {
		t.Errorf("note %q", note)
	}
	if _, ok := registryRunValue(from); ok {
		t.Error("the former Run value is still there")
	}
	if data, ok := registryRunValue(to); !ok || data != runCommand(exe, os.Getenv("MPRP_HOME")) {
		t.Errorf("new Run value %q (%v)", data, ok)
	}
	if !startupDisabled(approvedData(to)) || GetAutoStart(exe) {
		t.Error("the start switched off in Task Manager came back on")
	}
}

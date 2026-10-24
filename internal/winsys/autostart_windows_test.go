package winsys

import (
	"os"
	"strings"
	"testing"
)

// The registry read gives what `reg query` gave. Read-only: it only looks at values, and never writes.
func TestRegistryRunValueMatchesRegExe(t *testing.T) {
	if _, ok := registryRunValue("MPCvibedRPC-test-no-such-value"); ok {
		t.Error("a value that does not exist was found")
	}
	for _, name := range []string{"MPCvibedRPC", "MPCvibedRPC-test-no-such-value"} {
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
// folder (MPCRPC_HOME) and removes it again. It never touches the plain "MPCvibedRPC" value.
func TestAutostartRegistryRoundTrip(t *testing.T) {
	if os.Getenv("MPCRPC_TEST_REGISTRY") != "1" {
		t.Skip("set MPCRPC_TEST_REGISTRY=1 to write and remove a Run value for a throwaway data folder")
	}
	t.Setenv("MPCRPC_HOME", t.TempDir())
	if runName() == runNameBase {
		t.Fatal("the test must use a value name of its own")
	}
	exe := `C:\Test\MPCvibedRPC.exe`
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
	if !ok || data != runCommand(exe, os.Getenv("MPCRPC_HOME")) {
		t.Errorf("stored command %q, want %q", data, runCommand(exe, os.Getenv("MPCRPC_HOME")))
	}
	SetAutoStart(false, exe)
	if GetAutoStart(exe) {
		t.Error("still on after removing it")
	}
}

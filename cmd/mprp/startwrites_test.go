package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CptPakundo/MPRP/internal/engine"
	"github.com/CptPakundo/MPRP/internal/store"
)

// When every old Fix a match line is left out after an update, the engine that runs already drops them too: the
// migration wrote an empty list, and the settings are applied although no fix was written.
func TestStartWritesAppliesWhenNoOldLineIsKept(t *testing.T) {
	dir := t.TempDir()
	old := `{"artworkOverrides": {"sample show": "C:/covers/show.jpg"}, "app": {"welcomeSeen": true, "checkUpdates": false}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.New(st.Config(), engine.Options{Log: func(string, string) {}})
	if c0 := eng.Config(); len(c0.Fixes()) != 1 {
		t.Fatalf("before: the engine has the fixes %+v, want the old line", c0.Fixes())
	}
	a := &app{st: st, eng: eng, log: func(string, string) {}}
	a.startWrites(false)
	if c1 := eng.Config(); len(c1.Fixes()) != 0 {
		t.Errorf("after: the engine still has the fixes %+v", c1.Fixes())
	}
}

// After an update nothing is written to config.json before ipc.json shows that this copy runs: every write encrypts
// the sign-ins, and the version before, put back if this copy does not come up, may not read them. A normal start
// makes the same writes before.
func TestStartWritesWaitForIPCAfterAnUpdate(t *testing.T) {
	const old = `{"artworkAliases": {"sample show": "Sample Series"}, "app": {"checkUpdates": false}}`
	for _, updating := range []bool{true, false} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(old), 0o600); err != nil {
			t.Fatal(err)
		}
		st, err := store.New(dir)
		if err != nil {
			t.Fatal(err)
		}
		saved := afterUpdate
		afterUpdate = updating
		t.Cleanup(func() { afterUpdate = saved })
		a := &app{st: st, log: func(string, string) {}, ipcFile: st.IPCPath(), token: "t"}
		written := func() bool {
			b, _ := os.ReadFile(st.File)
			return string(b) != old
		}

		a.writeEarly(false)
		if written() == updating {
			t.Errorf("updating %v: config.json written before ipc.json: %v", updating, written())
		}
		a.announce(4321, false)
		if _, err := os.Stat(a.ipcFile); err != nil {
			t.Errorf("updating %v: no ipc.json", updating)
		}
		b, _ := os.ReadFile(st.File)
		if !strings.Contains(string(b), `"matchFixes"`) || !st.App().WelcomeSeen {
			t.Errorf("updating %v: the start's writes were not made:\n%s", updating, b)
		}
	}
}

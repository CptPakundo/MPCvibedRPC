package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The carrying over of an install made under the program's former name, at the start.
func TestChooseDataDir(t *testing.T) {
	noCopy := func(string) *ipcInfo { return nil }
	setup := func(t *testing.T, withFormer, withNew bool) (dir, former string) {
		base := t.TempDir()
		dir, former = filepath.Join(base, "MPRP"), filepath.Join(base, "MPCvibedRPC")
		if withFormer {
			os.MkdirAll(former, 0o700)
			os.WriteFile(filepath.Join(former, "config.json"), []byte(`{"port":13580}`), 0o600)
		}
		if withNew {
			os.MkdirAll(dir, 0o700)
		}
		return dir, former
	}

	t.Run("a new install", func(t *testing.T) {
		dir, former := setup(t, false, false)
		if use, other, note := chooseDataDir(dir, former, false, noCopy); use != dir || other != nil || note != "" {
			t.Errorf("use %q other %v note %q", use, other, note)
		}
	})
	t.Run("a copy with a data folder of its own never moves anything", func(t *testing.T) {
		dir, former := setup(t, true, false)
		if use, _, note := chooseDataDir(dir, "", false, noCopy); use != dir || note != "" {
			t.Errorf("use %q note %q", use, note)
		}
		if _, err := os.Stat(former); err != nil {
			t.Error("the former folder was touched")
		}
	})
	t.Run("already moved", func(t *testing.T) {
		dir, former := setup(t, true, true)
		if use, _, note := chooseDataDir(dir, former, false, noCopy); use != dir || note != "" {
			t.Errorf("use %q note %q", use, note)
		}
	})
	t.Run("the first start under the new name moves the folder", func(t *testing.T) {
		dir, former := setup(t, true, false)
		use, other, note := chooseDataDir(dir, former, false, noCopy)
		if use != dir || other != nil || !strings.HasPrefix(note, "Moved the data folder") {
			t.Fatalf("use %q other %v note %q", use, other, note)
		}
		if b, err := os.ReadFile(filepath.Join(dir, "config.json")); err != nil || string(b) != `{"port":13580}` {
			t.Error("the settings did not come along")
		}
	})
	t.Run("during an update from the former version the folder stays where the old copy looks", func(t *testing.T) {
		dir, former := setup(t, true, false)
		asked := false
		use, other, note := chooseDataDir(dir, former, true, func(string) *ipcInfo { asked = true; return nil })
		if use != former || other != nil || !strings.Contains(note, "during the update") || asked {
			t.Errorf("use %q other %v note %q asked %v", use, other, note, asked)
		}
		if _, err := os.Stat(dir); err == nil {
			t.Error("the new folder was made during the update")
		}
	})
	t.Run("updating while both folders exist: the new folder, with ipc.json in both", func(t *testing.T) {
		// a copy of the former version that runs again after a downgrade made its folder anew, and updates itself
		dir, former := setup(t, true, true)
		use, other, note := chooseDataDir(dir, former, true, noCopy)
		if use != dir || other != nil || note != "" {
			t.Fatalf("use %q other %v note %q", use, other, note)
		}
		mirror := formerIPC(use, former, true)
		if mirror != filepath.Join(former, "ipc.json") {
			t.Fatalf("formerIPC = %q", mirror)
		}
		a := &app{ipcFile: filepath.Join(use, "ipc.json"), ipcFormer: mirror, token: "t"}
		a.writeIPC(4321)
		// the old copy, whichever folder it watches, sees this copy come up
		for _, f := range []string{a.ipcFile, mirror} {
			if !copyCameUp(f, os.Getpid(), make(chan struct{}), time.Second) {
				t.Errorf("%s does not show this copy", f)
			}
		}
		a.removeIPCFiles()
		for _, f := range []string{a.ipcFile, mirror} {
			if _, err := os.Stat(f); err == nil {
				t.Errorf("%s is left behind", f)
			}
		}
		// no second place outside an update, or when the former folder is the one in use, or is not there
		if formerIPC(use, former, false) != "" || formerIPC(former, former, true) != "" || formerIPC(use, "", true) != "" ||
			formerIPC(use, filepath.Join(t.TempDir(), "none"), true) != "" {
			t.Error("a second ipc.json outside an update while both folders are there")
		}
	})
	t.Run("a copy of the former version still runs: it is shown and nothing moves", func(t *testing.T) {
		dir, former := setup(t, true, false)
		running := &ipcInfo{Port: 1234, Token: "t", Pid: 42}
		var askedFor string
		use, other, note := chooseDataDir(dir, former, false, func(f string) *ipcInfo { askedFor = f; return running })
		if other != running || note != "" || askedFor != filepath.Join(former, "ipc.json") {
			t.Errorf("use %q other %v note %q asked for %q", use, other, note, askedFor)
		}
		if _, err := os.Stat(dir); err == nil {
			t.Error("the folder was moved from under a running copy")
		}
	})
}

// A running copy answers /api/ping with its name: the current one and the former one both count as this program.
func TestPingExistingKnowsTheFormerName(t *testing.T) {
	for _, c := range []struct {
		app  string
		want bool
	}{{"MPRP", true}, {"MPCvibedRPC", true}, {"SomethingElse", false}, {"", false}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"app": c.app, "pid": 7})
		}))
		u, _ := url.Parse(srv.URL)
		port, _ := strconv.Atoi(u.Port())
		f := filepath.Join(t.TempDir(), "ipc.json")
		b, _ := json.Marshal(ipcInfo{Port: port, Token: "t", Pid: 7})
		os.WriteFile(f, b, 0o600)
		if got := pingExisting(f) != nil; got != c.want {
			t.Errorf("app %q: found %v, want %v", c.app, got, c.want)
		}
		srv.Close()
	}
}

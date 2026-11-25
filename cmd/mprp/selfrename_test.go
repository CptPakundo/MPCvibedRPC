package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The program file is renamed only at a normal start, or after an update once its hand-over is over; never in the
// old version that came back after an update that did not start.
func TestWhenToRename(t *testing.T) {
	for _, c := range []struct {
		after, failed bool
		want          renameMoment
	}{
		{false, false, renameAtStart},
		{true, false, renameAfterHandOver},
		{false, true, renameNever},
		{true, true, renameNever},
	} {
		if got := whenToRename(c.after, c.failed); got != c.want {
			t.Errorf("after update %v, update failed %v: %v, want %v", c.after, c.failed, got, c.want)
		}
	}
}

// Only the exact former release names are renamed (any case on Windows), each to its current twin; any other name,
// and the macOS app, never.
func TestRenameTarget(t *testing.T) {
	dir := filepath.Join("some", "folder")
	for _, c := range []struct{ goos, goarch, name, want string }{
		{"windows", "amd64", "MPCvibedRPC.exe", "MPRP.exe"},
		{"windows", "arm64", "mpcvibedrpc.EXE", "MPRP.exe"},
		{"windows", "amd64", "MPRP.exe", ""},
		{"windows", "amd64", "MPCvibedRPC (1).exe", ""},
		{"windows", "amd64", "MyPresence.exe", ""},
		{"windows", "amd64", "MPCvibedRPC-linux-amd64", ""},
		{"linux", "amd64", "MPCvibedRPC-linux-amd64", "MPRP-linux-amd64"},
		{"linux", "arm64", "MPCvibedRPC-linux-arm64", "MPRP-linux-arm64"},
		{"linux", "amd64", "mpcvibedrpc-linux-amd64", ""}, // case counts on Linux
		{"linux", "amd64", "MPCvibedRPC-linux-arm64", ""}, // another system's file
		{"linux", "amd64", "mprp", ""},
		{"darwin", "arm64", "MPCvibedRPC", ""},
		{"darwin", "arm64", "MPRP", ""},
	} {
		want := ""
		if c.want != "" {
			want = filepath.Join(dir, c.want)
		}
		if got := renameTarget(c.goos, c.goarch, filepath.Join(dir, c.name)); got != want {
			t.Errorf("%s/%s %s: %q, want %q", c.goos, c.goarch, c.name, got, want)
		}
	}
}

// Each pair is a file line of tools/ci/former-names.txt, the list of former-name files every release publishes.
func TestFormerFileNamesAreInTheList(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "tools", "ci", "former-names.txt"))
	if err != nil {
		t.Fatal(err)
	}
	list := strings.ReplaceAll(string(b), "\r", "")
	for _, s := range [][2]string{{"windows", "amd64"}, {"linux", "amd64"}, {"linux", "arm64"}} {
		former, current := formerFileName(s[0], s[1])
		if !strings.Contains(list, "\nfile "+current+" "+former+"\n") {
			t.Errorf("former-names.txt has no line \"file %s %s\"", current, former)
		}
	}
}

// thisSystem skips where the program does not rename itself (macOS), and gives this system's former and current
// file names.
func thisSystem(t *testing.T) (former, current string) {
	t.Helper()
	former, current = formerFileName(runtime.GOOS, runtime.GOARCH)
	if former == "" {
		t.Skip("the program file is not renamed on " + runtime.GOOS)
	}
	return former, current
}

func TestRenameProgramFile(t *testing.T) {
	former, current := thisSystem(t)
	setup := func(t *testing.T, files map[string]string) (dir string) {
		dir = t.TempDir()
		for n, c := range files {
			os.WriteFile(filepath.Join(dir, n), []byte(c), 0o755)
		}
		return dir
	}
	exists := func(p string) bool { _, err := os.Lstat(p); return err == nil }

	t.Run("renamed", func(t *testing.T) {
		dir := setup(t, map[string]string{former: "program"})
		now, level, msg := renameProgramFile(runtime.GOOS, runtime.GOARCH, filepath.Join(dir, former))
		if now != filepath.Join(dir, current) || level != "INFO" || !strings.Contains(msg, "renamed to "+current) {
			t.Fatalf("now %q, %s %q", now, level, msg)
		}
		if fileText(now) != "program" || exists(filepath.Join(dir, former)) {
			t.Error("the file was not renamed")
		}
	})
	t.Run("any other name is left alone", func(t *testing.T) {
		dir := setup(t, map[string]string{"dev-build": "program"})
		exe := filepath.Join(dir, "dev-build")
		if now, _, msg := renameProgramFile(runtime.GOOS, runtime.GOARCH, exe); now != exe || msg != "" || !exists(exe) {
			t.Errorf("now %q, msg %q", now, msg)
		}
	})
	t.Run("a second copy of the same file is moved aside", func(t *testing.T) {
		dir := setup(t, map[string]string{former: "program", current: "program"})
		now, level, msg := renameProgramFile(runtime.GOOS, runtime.GOARCH, filepath.Join(dir, former))
		if now != filepath.Join(dir, current) || level != "INFO" || !strings.Contains(msg, "second copy") {
			t.Fatalf("now %q, %s %q", now, level, msg)
		}
		if exists(filepath.Join(dir, former)) || fileText(now) != "program" {
			t.Error("the second copy is still there, or the other file changed")
		}
		// the file moved aside (still there where the system keeps a running program's file) goes at the next start
		removeFormerLeftovers(runtime.GOOS, runtime.GOARCH, now)
		if exists(filepath.Join(dir, former+".old")) {
			t.Error("the file moved aside is still there")
		}
	})
	t.Run("a different file of the new name: both stay", func(t *testing.T) {
		dir := setup(t, map[string]string{former: "program", current: "another program"})
		exe := filepath.Join(dir, former)
		now, level, msg := renameProgramFile(runtime.GOOS, runtime.GOARCH, exe)
		if now != exe || level != "INFO" || !strings.Contains(msg, "keeps its former name") {
			t.Fatalf("now %q, %s %q", now, level, msg)
		}
		if fileText(exe) != "program" || fileText(filepath.Join(dir, current)) != "another program" {
			t.Error("a file was changed")
		}
	})
	t.Run("a folder that cannot be written to", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("a read-only folder still allows renames on Windows")
		}
		dir := setup(t, map[string]string{former: "program"})
		os.Chmod(dir, 0o555)
		defer os.Chmod(dir, 0o755)
		exe := filepath.Join(dir, former)
		if f, err := os.Create(filepath.Join(dir, "probe")); err == nil { // running as root
			f.Close()
			t.Skip("the folder is writable all the same")
		}
		now, level, msg := renameProgramFile(runtime.GOOS, runtime.GOARCH, exe)
		if now != exe || level != "WARN" || !strings.Contains(msg, "keeps working") || !exists(exe) {
			t.Errorf("now %q, %s %q", now, level, msg)
		}
	})
}

// Leftovers of the former name go once the program runs under its current name, and only when no file of the former
// name is there (its own update may need them).
func TestRemoveFormerLeftovers(t *testing.T) {
	former, current := thisSystem(t)
	dir := t.TempDir()
	for _, n := range []string{former + ".old", former + ".new", former} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o755)
	}
	removeFormerLeftovers(runtime.GOOS, runtime.GOARCH, filepath.Join(dir, current))
	if _, err := os.Stat(filepath.Join(dir, former+".old")); err != nil {
		t.Error("removed with a file of the former name there")
	}
	os.Remove(filepath.Join(dir, former))
	removeFormerLeftovers(runtime.GOOS, runtime.GOARCH, filepath.Join(dir, former)) // not under the current name yet
	if _, err := os.Stat(filepath.Join(dir, former+".old")); err != nil {
		t.Error("removed while the program still has the former name")
	}
	removeFormerLeftovers(runtime.GOOS, runtime.GOARCH, filepath.Join(dir, current))
	for _, n := range []string{former + ".old", former + ".new"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			t.Errorf("%s is still there", n)
		}
	}
}

// renameApp is an app whose program file is a stand-in of the former name in a temporary folder; the run-at-login
// entry and the window code are fakes.
func renameApp(t *testing.T) (a *app, logs func() string, entry *string) {
	t.Helper()
	former, _ := thisSystem(t)
	a, logs = autoApp(t)
	a.exe = filepath.Join(t.TempDir(), former)
	os.WriteFile(a.exe, []byte("program"), 0o755)
	cmd := `"` + a.exe + `" --background` // the fake Run value
	entry = &cmd
	f, s := followRunEntry, setProgramFile
	t.Cleanup(func() { followRunEntry, setProgramFile = f, s })
	followRunEntry = func(old, now string) string {
		if !strings.Contains(*entry, old) {
			return ""
		}
		*entry = `"` + now + `" --background`
		return "The run-at-login entry now starts " + now + "."
	}
	setProgramFile = func(string) {}
	return a, logs, entry
}

// After a rename everything follows the new path: the updates, the run-at-login entry; the log says so once.
func TestFollowName(t *testing.T) {
	_, current := thisSystem(t)
	a, logs, entry := renameApp(t)
	old := a.exe
	a.followName()
	want := filepath.Join(filepath.Dir(old), current)
	if a.exePath() != want || fileText(want) != "program" {
		t.Fatalf("exe %q\n%s", a.exePath(), logs())
	}
	if *entry != `"`+want+`" --background` {
		t.Errorf("run entry %q", *entry)
	}
	if n := strings.Count(logs(), "renamed to "+current); n != 1 {
		t.Errorf("%d log lines of the rename\n%s", n, logs())
	}
	if a.upd.installing.Load() {
		t.Error("an update stays blocked")
	}
	a.followName() // the next time there is nothing to do
	if strings.Count(logs(), "renamed") != 1 {
		t.Errorf("renamed twice\n%s", logs())
	}
}

// An update being installed holds the rename off: the update's own files are next to the program file.
func TestFollowNameWaitsForAnInstall(t *testing.T) {
	a, _, _ := renameApp(t)
	old := a.exe
	a.upd.installing.Store(true)
	a.followName()
	if a.exePath() != old || fileText(old) != "program" {
		t.Error("renamed during an install")
	}
}

// After an update the rename waits for the end of the hand-over (afterHandOver): until then the copy that started
// this one may still put its own program file back under the former name.
func TestAfterHandOverRenames(t *testing.T) {
	_, current := thisSystem(t)
	a, logs, _ := renameApp(t)
	old := a.exe
	os.WriteFile(old+".old", []byte("the version before"), 0o755)
	os.WriteFile(filepath.Join(a.st.Dir, failedMarker), []byte("9.9.9"), 0o644)
	a.afterHandOver()
	if a.exePath() != filepath.Join(filepath.Dir(old), current) {
		t.Fatalf("exe %q\n%s", a.exePath(), logs())
	}
	for _, f := range []string{old + ".old", filepath.Join(a.st.Dir, failedMarker)} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s is still there", f)
		}
	}
}

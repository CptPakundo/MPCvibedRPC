package store

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// formerFolder makes a data folder as the former version left it: settings, a log and a browser profile folder.
func formerFolder(t *testing.T, dir string) {
	t.Helper()
	for name, text := range map[string]string{
		"config.json": `{"port":13580}`,
		formerLog:     "a log line\n",
		filepath.Join("window", "Default", "Preferences"): "{}",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func readText(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return string(b)
}

func TestMoveFormerDataRenames(t *testing.T) {
	base := t.TempDir()
	from, to := filepath.Join(base, "Old"), filepath.Join(base, "New")
	formerFolder(t, from)
	dir, note := MoveFormerData(from, to)
	if dir != to || !strings.HasPrefix(note, "Moved the data folder") {
		t.Fatalf("dir %q, note %q", dir, note)
	}
	if exists(from) {
		t.Error("the former folder is still there after a move")
	}
	if readText(t, filepath.Join(to, "config.json")) != `{"port":13580}` || readText(t, filepath.Join(to, "window", "Default", "Preferences")) != "{}" {
		t.Error("the settings or the window's profile did not come along")
	}
	if readText(t, filepath.Join(to, logName)) != "a log line\n" || exists(filepath.Join(to, formerLog)) {
		t.Error("the log was not given its new name")
	}
	// once: a second start finds the new folder and leaves everything alone
	if dir, note := MoveFormerData(from, to); dir != to || note != "" {
		t.Errorf("second start: dir %q, note %q", dir, note)
	}
}

func TestMoveFormerDataNothingToDo(t *testing.T) {
	base := t.TempDir()
	from, to := filepath.Join(base, "Old"), filepath.Join(base, "New")
	// no former folder
	if dir, note := MoveFormerData(from, to); dir != to || note != "" || exists(to) {
		t.Errorf("no former folder: dir %q, note %q", dir, note)
	}
	// MPRP_HOME: FormerDataDir gives ""
	if dir, note := MoveFormerData("", to); dir != to || note != "" {
		t.Errorf("no former folder name: dir %q, note %q", dir, note)
	}
	// both there: the new one is used and the former one left alone
	formerFolder(t, from)
	os.MkdirAll(to, 0o700)
	os.WriteFile(filepath.Join(to, "config.json"), []byte(`{"port":1}`), 0o600)
	if dir, note := MoveFormerData(from, to); dir != to || note != "" {
		t.Errorf("both there: dir %q, note %q", dir, note)
	}
	if readText(t, filepath.Join(to, "config.json")) != `{"port":1}` || !exists(filepath.Join(from, "config.json")) {
		t.Error("a folder was changed although the new one existed")
	}
	// a file of the former name is not a data folder
	f := filepath.Join(base, "File")
	os.WriteFile(f, []byte("x"), 0o600)
	if dir, note := MoveFormerData(f, filepath.Join(base, "New2")); note != "" || dir != filepath.Join(base, "New2") {
		t.Errorf("a file: dir %q, note %q", dir, note)
	}
}

func TestFormerDataDirNeverWithOwnHome(t *testing.T) {
	t.Setenv("MPRP_HOME", t.TempDir())
	if d := FormerDataDir(); d != "" {
		t.Errorf("FormerDataDir with MPRP_HOME = %q, want none", d)
	}
}

func TestDataDirNames(t *testing.T) {
	t.Setenv("MPRP_HOME", "")
	t.Setenv("LOCALAPPDATA", filepath.Join("C:", "Local"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(string(filepath.Separator), "xdg"))
	now, former := DataDir(), FormerDataDir()
	if filepath.Base(now) != "MPRP" || filepath.Base(former) != "MPCvibedRPC" || filepath.Dir(now) != filepath.Dir(former) {
		t.Errorf("data folder %q, former %q: want the two names side by side", now, former)
	}
}

// The copy that is made when the folder cannot be renamed: everything comes along, the former folder stays.
func TestCopyDir(t *testing.T) {
	base := t.TempDir()
	from, to := filepath.Join(base, "Old"), filepath.Join(base, "New.moving")
	formerFolder(t, from)
	if err := copyDir(from, to); err != nil {
		t.Fatal(err)
	}
	if readText(t, filepath.Join(to, "config.json")) != `{"port":13580}` || readText(t, filepath.Join(to, "window", "Default", "Preferences")) != "{}" {
		t.Error("the copy is incomplete")
	}
	if !exists(filepath.Join(from, "config.json")) {
		t.Error("the copy changed the former folder")
	}
	// a file that is there already is never overwritten
	if err := copyDir(from, to); err == nil {
		t.Error("copying over an existing copy should fail")
	}
}

// A former folder that cannot be renamed (here: the new name's parent is a file, so neither rename nor copy can
// work) is used where it is, untouched.
func TestMoveFormerDataFallsBack(t *testing.T) {
	base := t.TempDir()
	from := filepath.Join(base, "Old")
	formerFolder(t, from)
	blocker := filepath.Join(base, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o600)
	dir, note := MoveFormerData(from, filepath.Join(blocker, "New"))
	if dir != from || !strings.Contains(note, "used where it is") {
		t.Fatalf("dir %q, note %q", dir, note)
	}
	if readText(t, filepath.Join(from, "config.json")) != `{"port":13580}` {
		t.Error("the former folder was changed")
	}
}

func TestFormerUpdateSourceReadsAsTheCurrentOne(t *testing.T) {
	s := openStore(t, t.TempDir())
	os.WriteFile(s.File, []byte(`{"app":{"updateRepo":"CptPakundo/MPCvibedRPC"}}`), 0o600)
	if r := s.App().UpdateRepo; r != DefaultUpdateRepo {
		t.Errorf("former default reads as %q, want %q", r, DefaultUpdateRepo)
	}
	os.WriteFile(s.File, []byte(`{"app":{"updateRepo":"someone/fork"}}`), 0o600)
	if r := s.App().UpdateRepo; r != "someone/fork" {
		t.Errorf("a fork reads as %q", r)
	}
}

// On Windows a folder with a file held open cannot be renamed (the case of a copy of the former version that still
// runs, or a scanner): the folder is copied instead and the former one left in place.
func TestMoveFormerDataCopiesWhenAFileIsHeld(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows refuses to rename a folder with an open file")
	}
	base := t.TempDir()
	from, to := filepath.Join(base, "Old"), filepath.Join(base, "New")
	formerFolder(t, from)
	held, err := os.Open(filepath.Join(from, formerLog))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	dir, note := MoveFormerData(from, to)
	if dir != to || !strings.HasPrefix(note, "Copied the data folder") {
		t.Fatalf("dir %q, note %q", dir, note)
	}
	if readText(t, filepath.Join(to, "config.json")) != `{"port":13580}` || readText(t, filepath.Join(to, logName)) != "a log line\n" {
		t.Error("the copy is incomplete")
	}
	if !exists(filepath.Join(from, "config.json")) || exists(to+".moving") {
		t.Error("the former folder should stay and the temporary copy should be gone")
	}
}

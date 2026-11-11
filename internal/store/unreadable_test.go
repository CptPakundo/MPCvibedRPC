package store

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
)

func patchOf(t *testing.T, js string) *jsonx.Obj {
	t.Helper()
	v, err := jsonx.Parse(js)
	if err != nil {
		t.Fatal(err)
	}
	return v.(*jsonx.Obj)
}

// A config.json that cannot be read for a moment (a scanner holds it) is not an empty one: nothing is written over
// it, and readers keep the last good values.
func TestUnreadableConfigIsNeverOverwritten(t *testing.T) {
	s := openStore(t, t.TempDir())
	s.retryWait = 0
	if err := s.Update(patchOf(t, `{"settings":{"hideTitle":true,"vlcPassword":"pw"},"app":{"checkUpdates":false}}`)); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.File)
	if err != nil {
		t.Fatal(err)
	}
	tries := 0
	s.readFile = func(string) ([]byte, error) { tries++; return nil, errors.New("sharing violation") }

	if err := s.Update(patchOf(t, `{"settings":{"hideTitle":false}}`)); err == nil {
		t.Fatal("Update on an unreadable file must fail")
	}
	if tries != 5 {
		t.Errorf("tried %d times, want 5", tries)
	}
	if err := s.SetHidden([]string{"plexToken"}, map[string]any{"plexToken": "x"}); err == nil {
		t.Fatal("SetHidden on an unreadable file must fail")
	}
	if err := s.Reset(); err == nil {
		t.Fatal("Reset on an unreadable file must fail")
	}
	if cfg := s.Config(); !cfg.HideTitle || cfg.VlcPassword != "pw" {
		t.Errorf("Config() lost the last good values: %+v", cfg)
	}
	if s.App().CheckUpdates {
		t.Error("App() lost the last good values")
	}

	s.readFile = nil
	after, _ := os.ReadFile(s.File)
	if string(after) != string(before) {
		t.Errorf("config.json changed:\n%s\nwas:\n%s", after, before)
	}
	if err := s.Update(patchOf(t, `{"settings":{"hideTitle":false}}`)); err != nil {
		t.Fatal(err)
	}
	if cfg := s.Config(); cfg.HideTitle || cfg.VlcPassword != "pw" {
		t.Errorf("after the file is readable again: %+v", cfg)
	}
}

// A folder where the file should be is a read error that is not "does not exist": nothing is written, defaults show.
func TestConfigThatIsAFolderGivesDefaults(t *testing.T) {
	s := openStore(t, t.TempDir())
	s.retryWait = 0
	if err := os.Mkdir(s.File, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(patchOf(t, `{"settings":{"hideTitle":true}}`)); err == nil {
		t.Fatal("expected an error")
	}
	if s.Config().HideTitle || !s.App().CheckUpdates {
		t.Error("readers should give the defaults")
	}
}

// A file that does not exist is still the first run: an empty configuration that can be saved.
func TestMissingConfigStillWorks(t *testing.T) {
	s := openStore(t, t.TempDir())
	if !s.FirstRun() {
		t.Fatal("expected first run")
	}
	if s.Config().HideTitle {
		t.Error("defaults expected")
	}
	if err := s.Update(patchOf(t, `{"settings":{"hideTitle":true}}`)); err != nil {
		t.Fatal(err)
	}
	if !s.Config().HideTitle {
		t.Error("not saved")
	}
}

// Text that reaches players' config files must not carry line breaks or other control characters.
func TestControlCharactersAreRefused(t *testing.T) {
	s := openStore(t, t.TempDir())
	if err := s.Update(patchOf(t, `{"settings":{"hideTitle":true}}`)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.File)
	for _, js := range []string{
		`{"settings":{"vlcPassword":"x\nsecond line"}}`,
		`{"settings":{"mpvPipe":"a\rb"}}`,
		`{"settings":{"iinaPipe":"a\u0000b"}}`,
		`{"settings":{"vlcPassword":"a\tb"}}`,
		`{"settings":{"vlcPassword":"a\u007fb"}}`,
		`{"settings":{"hideFiles":["ok","a\nb"]}}`,
		`{"settings":{"hideFiles":["a\u0001b"]}}`,
	} {
		err := s.Update(patchOf(t, js))
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: want a validation error, got %v", js, err)
		}
	}
	after, _ := os.ReadFile(s.File)
	if string(before) != string(after) {
		t.Error("a refused save changed the file")
	}
	ok := `{"settings":{"vlcPassword":"it's a \"pass\" wörd ñ 日本","hideFiles":["Sample Show's Finale"]}}`
	if err := s.Update(patchOf(t, ok)); err != nil {
		t.Fatalf("normal value refused: %v", err)
	}
	if got := s.Config().VlcPassword; !strings.Contains(got, "wörd") {
		t.Errorf("password = %q", got)
	}
}

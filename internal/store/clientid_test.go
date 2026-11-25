package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CptPakundo/MPRP/internal/core"
)

// An install that saved the former default Discord application uses the project's own; one of the user's own stays.
func TestFormerClientIDReadsAsTheDefault(t *testing.T) {
	for _, c := range []struct{ saved, want string }{
		{core.FormerClientID, core.DefaultClientID},
		{"123456789012345678", "123456789012345678"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"clientId": "`+c.saved+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		s := openStore(t, dir)
		if got := s.Config().ClientID; got != c.want {
			t.Errorf("saved %s: engine uses %s, want %s", c.saved, got, c.want)
		}
		if got, _ := s.SettingsObj().M["clientId"].(string); got != c.want {
			t.Errorf("saved %s: the window shows %s, want %s", c.saved, got, c.want)
		}
	}
	if core.DefaultConfig().ClientID != core.DefaultClientID {
		t.Error("the default clientId is not DefaultClientID")
	}
}

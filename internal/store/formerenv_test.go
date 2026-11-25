package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CptPakundo/MPRP/internal/core"
)

// A copy started by the former version with a data folder of its own (MPCRPC_HOME, no MPRP_HOME) uses that folder,
// once main has adopted the variable (core.AdoptFormerEnv), and moves nothing.
func TestDataDirFollowsTheFormerHomeVariable(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("MPRP_HOME", "")
	os.Unsetenv("MPRP_HOME")
	t.Setenv("MPCRPC_HOME", home)
	core.AdoptFormerEnv()
	if DataDir() != home || FormerDataDir() != "" {
		t.Errorf("DataDir() = %q, FormerDataDir() = %q; want %q and none", DataDir(), FormerDataDir(), home)
	}
}

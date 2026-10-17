package potplayer

import (
	"os"
	"strings"
	"testing"
)

// TestLivePotPlayer asks a real PotPlayer on this computer (opt-in: MPCRPC_LIVE_POTPLAYER=1, with a local file playing)
// and checks that it sends the file's path, so the Never show list can match folders.
func TestLivePotPlayer(t *testing.T) {
	if os.Getenv("MPCRPC_LIVE_POTPLAYER") == "" {
		t.Skip("set MPCRPC_LIVE_POTPLAYER=1 with PotPlayer playing a local file")
	}
	found := Query()
	for _, in := range found {
		t.Logf("PotPlayer: file %q, folder known %v, state %d, %d of %d ms, speed %v", in.File, in.FullDir != "", in.State, in.Position, in.Duration, in.Rate)
	}
	if len(found) == 0 {
		t.Fatal("no PotPlayer window found")
	}
	in := found[0]
	if in.State != 2 || in.Duration <= 0 || in.File == "" {
		t.Fatalf("not playing a file: %+v", in)
	}
	if in.FullDir == "" || strings.Contains(in.FullDir, "PotPlayer") {
		t.Fatalf("PotPlayer did not send the file's path (folder %q)", in.FullDir)
	}
}

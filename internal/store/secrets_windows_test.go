package store

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The real DPAPI, in memory, for the current user: nothing is stored and nothing on the computer changes.
func TestDPAPIRoundTrip(t *testing.T) {
	plain := []byte("sample-token")
	sealed, err := dpapi{}.seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, plain) {
		t.Error("the encrypted bytes hold the plain text")
	}
	back, err := dpapi{}.open(sealed)
	if err != nil || !bytes.Equal(back, plain) {
		t.Fatalf("open = %q, %v", back, err)
	}
	if _, err := (dpapi{}).open([]byte("not encrypted")); err == nil {
		t.Error("bytes DPAPI did not make cannot be opened")
	}
	// the store uses it by itself on Windows
	s := openStore(t, t.TempDir())
	if err := s.SetHidden([]string{"plexToken"}, map[string]any{"plexToken": "sample-token"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.File); strings.Contains(string(b), "sample-token") || !strings.Contains(string(b), sealedPrefix) {
		t.Errorf("config.json:\n%s", b)
	}
	if openStore(t, s.Dir).Config().PlexToken != "sample-token" {
		t.Error("read back")
	}
}

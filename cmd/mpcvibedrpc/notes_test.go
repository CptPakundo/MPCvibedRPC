package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The release text (.github/release-notes.md, the body of the GitHub release) must be the one for the version being
// built: nothing else reads the file before it is published, and a forgotten rewrite would publish the previous
// release's text.
func TestReleaseNotesAreForThisVersion(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "release-notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(strings.TrimLeft(string(b), "\xef\xbb\xbf"), "\n")
	if want := "## MPCvibedRPC " + version; strings.TrimRight(first, "\r") != want {
		t.Fatalf("release-notes.md starts with %q; want %q (rewrite it for this release)", first, want)
	}
}

package store

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/CptPakundo/MPRP/internal/core"
)

// The release workflow refuses to publish from a repository whose name is not the one the code updates from: its first
// step reads DefaultUpdateRepo out of store.go with sed. That step must stay, and must find the constant as it is
// written here. The player logos are fetched from the same repository.
func TestReleaseChecksTheRepositoryName(t *testing.T) {
	wf, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	const sedExpr = `s/^const DefaultUpdateRepo = "\(.*\)"$/\1/p`
	if !strings.Contains(string(wf), sedExpr+`' internal/store/store.go`) || !strings.Contains(string(wf), `"$GITHUB_REPOSITORY" != "$want"`) {
		t.Fatal("release.yml no longer compares the repository's name with DefaultUpdateRepo")
	}
	src, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	// the sed expression as a Go regular expression, on the file as CI checks it out (LF)
	m := regexp.MustCompile(`(?m)^const DefaultUpdateRepo = "(.*)"$`).FindAllStringSubmatch(strings.ReplaceAll(string(src), "\r\n", "\n"), -1)
	if len(m) != 1 || m[0][1] != DefaultUpdateRepo {
		t.Fatalf("the workflow would read %v from store.go, want %q once", m, DefaultUpdateRepo)
	}
	if !strings.Contains(core.PlayerIconBase, "/"+DefaultUpdateRepo+"/") {
		t.Errorf("the player logos (%s) come from another repository than the updates (%s)", core.PlayerIconBase, DefaultUpdateRepo)
	}
}

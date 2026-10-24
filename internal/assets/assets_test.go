package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The page's script must parse: no Go test runs it, and one stray apostrophe would leave the window blank.
func TestPageScriptParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the page's script cannot be checked here")
	}
	start := strings.Index(UI, "<script>")
	end := strings.LastIndex(UI, "</script>")
	if start < 0 || end < start || strings.Count(UI, "<script") != 1 {
		t.Fatal("the page should hold exactly one <script> block")
	}
	js := filepath.Join(t.TempDir(), "ui.js")
	if err := os.WriteFile(js, []byte(UI[start+len("<script>"):end]), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", js).CombinedOutput(); err != nil {
		t.Fatalf("node --check: %v\n%s", err, out)
	}
}

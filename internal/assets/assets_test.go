package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The page's script must parse: no Go test runs it, and one stray apostrophe would leave the window blank.
func TestPageScriptParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the page's script cannot be checked here")
	}
	js := filepath.Join(t.TempDir(), "ui.js")
	if err := os.WriteFile(js, []byte(JS), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", js).CombinedOutput(); err != nil {
		t.Fatalf("node --check: %v\n%s", err, out)
	}
}

// The page policy (script-src 'self', style-src 'self') allows no inline script or style: the markup must hold none,
// and must load the two files the server serves. The script reads the token from the meta tag.
func TestPageHasNoInlineCode(t *testing.T) {
	if strings.Count(UI, "<script") != 1 || !strings.Contains(UI, `<script src="/ui.js" defer></script>`) {
		t.Error("the page should load /ui.js and hold no other <script>")
	}
	if strings.Contains(UI, "<style") || !strings.Contains(UI, `<link rel="stylesheet" href="/ui.css">`) {
		t.Error("the page should load /ui.css and hold no <style> block")
	}
	if re := regexp.MustCompile(`(?i)\s(style|on[a-z]+)\s*=`); re.MatchString(UI) {
		t.Errorf("the page has an inline style or event attribute: %q", re.FindString(UI))
	}
	if strings.Count(UI, "__TOKEN__") != 1 || !strings.Contains(UI, `<meta name="token" content="__TOKEN__">`) {
		t.Error("the token belongs in the page's meta tag only")
	}
	if strings.Contains(JS, "__TOKEN__") || !strings.Contains(JS, `meta[name="token"]`) {
		t.Error("the script should read the token from the meta tag")
	}
	if re := regexp.MustCompile(`setAttribute\(\s*['"]style['"]|\.cssText|style\s*=`); re.MatchString(JS) {
		t.Errorf("the script sets an inline style attribute: %q", re.FindString(JS))
	}
}

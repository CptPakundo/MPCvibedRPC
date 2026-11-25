package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

func TestDiscNamesItself(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stream := filepath.Join(root, "BDMV", "STREAM")
	meta := filepath.Join(root, "BDMV", "META", "DL")
	for _, d := range []string{stream, meta} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dirs := []string{"STREAM", "BDMV"} // a disc at a drive's root: nothing above BDMV
	cfg := core.DefaultConfig()

	// No bdmt file (and a temporary folder is no volume root): nothing is added, the number stays junk.
	if got := discDirs(dirs, stream, nil); len(got) != 2 {
		t.Fatalf("no disc name expected, got %q", got)
	}

	write := func(name, title string) {
		x := `<?xml version="1.0" encoding="utf-8"?><disclib><di:discinfo><di:title><di:name>` + title + `</di:name></di:title></di:discinfo></disclib>`
		if err := os.WriteFile(filepath.Join(meta, name), []byte(x), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("bdmt_fra.xml", "Exemple de film")
	write("bdmt_eng.xml", "Sample Movie &amp; Friends")
	got := discDirs(dirs, stream, nil)
	if len(got) != 3 || got[2] != "Sample Movie & Friends" {
		t.Fatalf("the English bdmt title should be added: %q", got)
	}
	if m := core.ParseMedia("00062.m2ts", &cfg, got); m.Title != "Sample Movie & Friends" || m.Junk {
		t.Fatalf("the disc's name should be the title: %+v", m)
	}

	// The engine keeps the name for a while, so an optical drive is not read at every check.
	var f folderNames
	if f.discName(root) != "Sample Movie & Friends" {
		t.Fatal("first read")
	}
	write("bdmt_eng.xml", "Another Title")
	if f.discName(root) != "Sample Movie & Friends" {
		t.Fatal("the name should be kept for a minute")
	}

	// A DVD, and folders that are not a disc's root, are handled too.
	if r, ok := discRoot([]string{"VIDEO_TS"}, filepath.Join(root, "VIDEO_TS")); !ok || r != root {
		t.Fatalf("a DVD's root: %q %v", r, ok)
	}
	if _, ok := discRoot([]string{"STREAM", "BDMV", "Sample Movie"}, stream); ok {
		t.Fatal("a disc inside a named folder is named by that folder, not read")
	}
}

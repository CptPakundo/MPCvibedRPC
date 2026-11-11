package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// seasonFolder makes a folder of episodes named with a running production code, from..to.
func seasonFolder(t *testing.T, from, to int) string {
	t.Helper()
	dir := t.TempDir()
	for n := from; n <= to; n++ {
		addEpisode(t, dir, n)
	}
	return dir
}

func addEpisode(t *testing.T, dir string, n int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("Sample Show S02 - AB%03d.mkv", n)), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// touchFolder moves the folder's change time on by hand: a file system may give two changes made within a few
// milliseconds the same time, which a test must not depend on.
func touchFolder(t *testing.T, dir string, by time.Duration) {
	t.Helper()
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, time.Time{}, st.ModTime().Add(by)); err != nil {
		t.Fatal(err)
	}
}

// The folder is read once per file, not at every status of the same file; another file, another folder and a
// folder whose entries changed are read again.
func TestFolderIsReadOncePerFile(t *testing.T) {
	t.Parallel()
	dir := seasonFolder(t, 50, 56)
	var f folderNames
	for range 3 {
		if got := f.list(dir, "Sample Show S02 - AB052.mkv"); len(got) != 7 {
			t.Fatalf("%d names, want 7", len(got))
		}
	}
	if f.reads != 1 {
		t.Errorf("the same file read the folder %d times, want 1", f.reads)
	}
	f.list(dir, "Sample Show S02 - AB053.mkv")
	if f.reads != 2 {
		t.Errorf("another file: %d reads, want 2", f.reads)
	}
	addEpisode(t, dir, 57)
	touchFolder(t, dir, 2*time.Second)
	if got := f.list(dir, "Sample Show S02 - AB053.mkv"); len(got) != 8 || f.reads != 3 {
		t.Errorf("after a file was added: %d names in %d reads, want 8 in 3", len(got), f.reads)
	}
	other := seasonFolder(t, 1, 2)
	if got := f.list(other, "Sample Show S02 - AB053.mkv"); len(got) != 2 || f.reads != 4 {
		t.Errorf("another folder: %d names in %d reads, want 2 in 4", len(got), f.reads)
	}
	f.forget()
	if f.list(other, "Sample Show S02 - AB053.mkv"); f.reads != 5 {
		t.Errorf("after forgetting: %d reads, want 5", f.reads)
	}
	if got := f.list(filepath.Join(dir, "missing"), "a.mkv"); got != nil {
		t.Errorf("a folder that is not there gave %v", got)
	}
	if got := (*folderNames)(nil).list(dir, "a.mkv"); len(got) != 8 {
		t.Errorf("without a memory: %d names, want 8", len(got))
	}
}

// folderEpisodeNumbers shows the file's place in its season folder, the same with and without the kept listing,
// and a status sent again for the same file does not read the folder again.
func TestFolderEpisodeNumberComesFromTheKeptListing(t *testing.T) {
	t.Parallel()
	dir := seasonFolder(t, 50, 56)
	cfg := core.DefaultConfig()
	cfg.FolderEpisodeNumbers = true
	file := "Sample Show S02 - AB052.mkv"
	info := core.NewInfo(file, filepath.Join(dir, file), dir, 2, 60000, 1500000, 1)
	var f folderNames
	want := buildActivity(info, &cfg, nil, nil, time.Now()).activity
	if want == nil || want.Assets == nil || want.Assets.LargeText != "Season 2, Episode 3" {
		t.Fatalf("without a memory: %+v", want)
	}
	for range 3 {
		got := buildActivity(info, &cfg, nil, &f, time.Now()).activity
		if got == nil || got.Assets == nil || got.Assets.LargeText != want.Assets.LargeText {
			t.Fatalf("with the kept listing: %+v, want %q", got, want.Assets.LargeText)
		}
	}
	if f.reads != 1 {
		t.Errorf("three statuses of one file read the folder %d times, want 1", f.reads)
	}
	cfg.FolderEpisodeNumbers = false
	if buildActivity(info, &cfg, nil, &f, time.Now()); f.reads != 1 {
		t.Error("the folder was read with the setting off")
	}
}

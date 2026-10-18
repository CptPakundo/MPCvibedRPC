package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func rel(tag string, counts ...any) release {
	r := release{Tag: tag}
	for i := 0; i < len(counts); i += 2 {
		r.Assets = append(r.Assets, asset{Name: counts[i].(string), Count: counts[i+1].(int)})
	}
	return r
}

// People are what is left of the program files' counts after the baseline and the development downloads; checksums
// and notices never count, drafts are left out, and a recorded download GitHub has not counted yet never goes below 0.
func TestCount(t *testing.T) {
	rels := []release{
		rel("v0.9.1", "MPCvibedRPC.exe", 9, "MPCvibedRPC.exe.sha256", 5, "LICENSE", 4),
		rel("v0.9.2", "MPCvibedRPC.exe", 7, "MPCvibedRPC-macos.zip", 3, "MPCvibedRPC-linux-amd64", 2, "MPCvibedRPC.exe.sha256", 6),
		{Tag: "v0.9.3", Draft: true, Assets: []asset{{Name: "MPCvibedRPC.exe", Count: 2}}},
	}
	base := baseline{At: "2026-01-01T00:00:00Z", Counts: map[string]int{"v0.9.1/MPCvibedRPC.exe": 4, "v0.9.1/LICENSE": 4}}
	dev := map[string]int{
		"v0.9.1/MPCvibedRPC.exe":         3, // 9 - 4 - 3 = 2 people
		"v0.9.2/MPCvibedRPC.exe":         4, // a release after the baseline: 7 - 4 = 3 people
		"v0.9.2/MPCvibedRPC-macos.zip":   3,
		"v0.9.2/MPCvibedRPC-linux-amd64": 5, // GitHub has counted 2 of 5 so far
		"v0.9.2/MPCvibedRPC.exe.sha256":  6,
	}
	s := count(rels, base, dev)
	if s.People != 5 || s.Development != 12 || s.NotCountedYet != 3 {
		t.Fatalf("people %d, development %d, not counted yet %d; want 5, 12, 3", s.People, s.Development, s.NotCountedYet)
	}
	if s.ByFile["MPCvibedRPC.exe"] != 5 || s.ByRelease["v0.9.1"] != 2 || s.ByRelease["v0.9.2"] != 3 || len(s.ByFile) != 1 {
		t.Errorf("by file %v, by release %v", s.ByFile, s.ByRelease)
	}
	if b := badge(s); b["message"] != "5" || b["schemaVersion"] != 1 {
		t.Errorf("badge %v", b)
	}
}

// The files written by the commands read back: a baseline from the releases now, gh's paginated output (one array per
// page), and every ledger file added up.
func TestFiles(t *testing.T) {
	dir := t.TempDir()
	pages := `[{"tag_name":"v0.9.1","assets":[{"name":"MPCvibedRPC.exe","download_count":4}]}]
[{"tag_name":"v0.9.2","assets":[{"name":"MPCvibedRPC.exe","download_count":6}]}]`
	relFile := filepath.Join(dir, "releases.json")
	os.WriteFile(relFile, []byte(pages), 0o644)
	rels, err := readReleases(relFile)
	if err != nil || len(rels) != 2 {
		t.Fatalf("releases %v, %v", rels, err)
	}
	b := newBaseline(rels[:1], time.Unix(0, 0))
	if b.Counts["v0.9.1/MPCvibedRPC.exe"] != 4 {
		t.Fatalf("baseline %v", b)
	}
	os.MkdirAll(filepath.Join(dir, "ledger"), 0o755)
	os.WriteFile(filepath.Join(dir, "ledger", "a.json"), []byte(`{"by":"release check","at":"2026-01-02T10:00:00Z","files":{"v0.9.2/MPCvibedRPC.exe":2}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "ledger", "b.json"), []byte(`{"by":"agent","at":"2026-01-03T10:00:00Z","files":{"v0.9.2/MPCvibedRPC.exe":1}}`), 0o644)
	// before the baseline: already in its counts
	os.WriteFile(filepath.Join(dir, "ledger", "c.json"), []byte(`{"by":"agent","at":"2026-01-01T10:00:00Z","files":{"v0.9.2/MPCvibedRPC.exe":5}}`), 0o644)
	dev, err := ledger(filepath.Join(dir, "ledger"), "2026-01-02T00:00:00Z")
	if err != nil || dev["v0.9.2/MPCvibedRPC.exe"] != 3 {
		t.Fatalf("ledger %v, %v", dev, err)
	}
	if s := count(rels, b, dev); s.People != 3 || s.Development != 3 {
		t.Errorf("people %d, development %d; want 3, 3", s.People, s.Development)
	}
}

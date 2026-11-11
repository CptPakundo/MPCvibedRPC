package artwork

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// cacheTestFinder makes a finder on a temp file whose clock is the returned day.
func cacheTestFinder(t *testing.T, file string, day *int) *Artwork {
	t.Helper()
	cfg := core.DefaultConfig()
	a := New(&cfg, Options{CacheFile: file})
	a.today = func() int { return *day }
	return a
}

// readCacheFile returns the saved answers and the days under "@used".
func readCacheFile(t *testing.T, file string) (map[string]json.RawMessage, map[string]int) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	days := map[string]int{}
	if v, ok := raw[usedKey]; ok {
		if err := json.Unmarshal(v, &days); err != nil {
			t.Fatal(err)
		}
	}
	return raw, days
}

// writeCacheFile writes a cache file with the given answers (key -> name) and, when days is not nil, "@used".
func writeCacheFile(t *testing.T, file string, names map[string]string, days map[string]int) {
	t.Helper()
	out := map[string]any{}
	for k, n := range names {
		art := core.NewArt()
		art.Name = n
		out[k] = art
	}
	if days != nil {
		out[usedKey] = days
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func putArt(a *Artwork, key, name string) {
	art := core.NewArt()
	art.Name = name
	a.mu.Lock()
	a.put(key, &diskEntry{art: art})
	a.mu.Unlock()
}

func TestCacheSavesTheDayOfUse(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c.json")
	day := 20000
	a := cacheTestFinder(t, file, &day)
	putArt(a, keyVersion+"s|sample show", "Sample Show")
	a.save()
	raw, days := readCacheFile(t, file)
	if _, ok := raw[keyVersion+"s|sample show"]; !ok {
		t.Fatal("the answer is not in the file")
	}
	if days[keyVersion+"s|sample show"] != day {
		t.Errorf("saved day = %d, want %d", days[keyVersion+"s|sample show"], day)
	}
}

func TestCacheForgetsAfterNinetyDays(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c.json")
	day := 20000
	old, edge, young := keyVersion+"s|old show", keyVersion+"s|edge show", "ep|"+keyVersion+"s|young show|1"
	writeCacheFile(t, file, map[string]string{old: "Old", edge: "Edge", young: "Young"},
		map[string]int{old: day - 91, edge: day - 90, young: day - 1})
	a := cacheTestFinder(t, file, &day)
	// New loaded with the real clock (about today's real date), so reload with the test clock.
	a.disk = map[string]*diskEntry{}
	a.load()
	if _, ok := a.disk[old]; ok {
		t.Error("an answer last used 91 days ago should be gone after load")
	}
	if _, ok := a.disk[edge]; !ok {
		t.Error("an answer last used 90 days ago should stay")
	}
	if _, ok := a.disk[young]; !ok {
		t.Error("a recently used episode answer should stay")
	}
	// a save with the clock moved on forgets the one that has now aged out
	day++
	putArt(a, keyVersion+"s|new show", "New")
	a.save()
	raw, days := readCacheFile(t, file)
	if _, ok := raw[edge]; ok {
		t.Error("the answer that reached 91 days should be gone from the saved file")
	}
	if _, ok := days[edge]; ok {
		t.Error("its day should be gone from @used too")
	}
	if _, ok := raw[young]; !ok {
		t.Error("the recent answer should still be in the file")
	}
}

func TestCacheFileWithoutDaysCountsAsUsedToday(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c.json")
	day := 20000
	k1, k2 := keyVersion+"s|sample show", "ep|"+keyVersion+"s|sample show|1"
	writeCacheFile(t, file, map[string]string{k1: "Sample Show", k2: "Ep"}, nil) // as an older version wrote it
	a := cacheTestFinder(t, file, &day)
	a.disk = map[string]*diskEntry{}
	a.load()
	if len(a.disk) != 2 {
		t.Fatalf("loaded %d answers, want 2", len(a.disk))
	}
	putArt(a, keyVersion+"s|other show", "Other")
	a.save()
	raw, days := readCacheFile(t, file)
	for _, k := range []string{k1, k2, keyVersion + "s|other show"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("%q missing from the saved file", k)
		}
		if days[k] != day {
			t.Errorf("day of %q = %d, want %d", k, days[k], day)
		}
	}
}

func TestCacheHitOnANewDayIsSavedInTheBackground(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c.json")
	day := 20000
	key := keyVersion + "s|sample show"
	writeCacheFile(t, file, map[string]string{key: "Sample Show"}, map[string]int{key: day})
	a := cacheTestFinder(t, file, &day)
	a.disk = map[string]*diskEntry{}
	a.load()

	day += 5
	a.mu.Lock()
	got := a.getBase(key)
	a.mu.Unlock()
	if got == nil {
		t.Fatal("expected a hit")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, days := readCacheFile(t, file)
		if days[key] == day {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the new day never reached the file (it has %d, want %d)", days[key], day)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// a second hit on the same day changes nothing and starts no save
	a.mu.Lock()
	a.getBase(key)
	dirty := a.dirty
	a.mu.Unlock()
	if dirty {
		t.Error("a hit on the same day should not mark the cache dirty")
	}
}

func TestCacheTrimDropsTheLongestUnused(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c.json")
	day := 20000
	names := map[string]string{}
	days := map[string]int{}
	for i := 0; i < 2100; i++ {
		k := fmt.Sprintf("%ssample show %04d", keyVersion, i)
		names[k] = "Sample Show"
		days[k] = day - 80 + i%10 // 80 to 71 days ago
	}
	// the oldest by day, though it sorts last by key
	oldest := keyVersion + "zzz oldest"
	names[oldest], days[oldest] = "Oldest", day-85
	writeCacheFile(t, file, names, days)
	a := cacheTestFinder(t, file, &day)
	a.disk = map[string]*diskEntry{}
	a.load()
	putArt(a, keyVersion+"just played", "Just Played")
	a.save()
	raw, savedDays := readCacheFile(t, file)
	if _, ok := raw[usedKey]; !ok {
		t.Fatal("@used missing")
	}
	if n := len(savedDays); n != 2000 {
		t.Errorf("%d entries saved, want 2000", n)
	}
	if _, ok := raw[keyVersion+"just played"]; !ok {
		t.Error("the title played now was dropped")
	}
	if _, ok := raw[oldest]; ok {
		t.Error("the entry used longest ago should have been dropped first")
	}
	// every entry of the day-71..80 group that stayed is at least as recent as every one that went
	minKept, maxDropped := 1<<30, -1
	for k, d := range days {
		if _, kept := raw[k]; kept {
			minKept = min(minKept, d)
		} else {
			maxDropped = max(maxDropped, d)
		}
	}
	if maxDropped > minKept {
		t.Errorf("dropped an entry (day %d) newer than one kept (day %d)", maxDropped, minKept)
	}
}

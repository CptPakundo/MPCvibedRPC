// downloads works out how often the program was downloaded by people: GitHub's count of each release file, minus
// the count when counting started (baseline.json) and minus every download made by development (the release checks
// and Claude agents, each recorded as a file in ledger/ by tools/ci/dev-download.sh). Only the program files count
// (the exe, the Linux programs and the macOS zip; not checksums or notices). It reads the releases as the GitHub API
// lists them and writes into the stats folder (the stats branch, kept by .github/workflows/downloads.yml):
//
//	go run ./tools/downloads count -releases releases.json -stats stats      (downloads.json for the badge, summary.json)
//	go run ./tools/downloads baseline -releases releases.json -stats stats   (starts counting from the counts now)
//
// GitHub updates its counts a few minutes late, so a download just recorded in the ledger may not be counted yet:
// such a file is taken as 0 and shown under "notCountedYet" until GitHub catches up.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type release struct {
	Tag    string  `json:"tag_name"`
	Draft  bool    `json:"draft"`
	Assets []asset `json:"assets"`
}

type asset struct {
	Name  string `json:"name"`
	Count int    `json:"download_count"`
}

// baseline is the count of every release file when counting started ("v0.9.38/MPCvibedRPC.exe": 3).
type baseline struct {
	At     string         `json:"at"`
	Counts map[string]int `json:"counts"`
}

// entry is one recording of development downloads.
type entry struct {
	By    string         `json:"by"`
	At    string         `json:"at"`
	Files map[string]int `json:"files"`
}

type summary struct {
	Since         string         `json:"since"`
	People        int            `json:"people"`
	Development   int            `json:"development"`
	NotCountedYet int            `json:"notCountedYet"`
	ByFile        map[string]int `json:"byFile"`
	ByRelease     map[string]int `json:"byRelease"`
}

// isProgram: the files people download to run the program.
func isProgram(name string) bool {
	return strings.HasPrefix(name, "MPCvibedRPC") && !strings.HasSuffix(name, ".sha256")
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// ledger adds up every recording in dir made at or after since (the baseline's time): downloads before a new
// baseline are already in its counts.
func ledger(dir, since string) (map[string]int, error) {
	out := map[string]int{}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		var e entry
		if err := readJSON(f, &e); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if e.At < since {
			continue
		}
		for k, n := range e.Files {
			out[k] += n
		}
	}
	return out, nil
}

// count works out the summary from the releases, the baseline and the ledger.
func count(rels []release, base baseline, dev map[string]int) summary {
	s := summary{Since: base.At, ByFile: map[string]int{}, ByRelease: map[string]int{}}
	for _, r := range rels {
		if r.Draft {
			continue
		}
		for _, a := range r.Assets {
			if !isProgram(a.Name) {
				continue
			}
			key := r.Tag + "/" + a.Name
			since := a.Count - base.Counts[key]
			d := dev[key]
			n := since - d
			if n < 0 { // GitHub has not counted some recorded downloads yet
				s.NotCountedYet -= n
				d, n = since, 0
			}
			s.Development += d
			s.People += n
			if n > 0 {
				s.ByFile[a.Name] += n
				s.ByRelease[r.Tag] += n
			}
		}
	}
	return s
}

// badge is shields.io's endpoint format.
func badge(s summary) map[string]any {
	return map[string]any{"schemaVersion": 1, "label": "downloads", "message": strconv.Itoa(s.People), "color": "444"}
}

func newBaseline(rels []release, now time.Time) baseline {
	b := baseline{At: now.UTC().Format(time.RFC3339), Counts: map[string]int{}}
	for _, r := range rels {
		for _, a := range r.Assets {
			b.Counts[r.Tag+"/"+a.Name] = a.Count
		}
	}
	return b
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: downloads count|baseline -releases FILE -stats DIR")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	relFile := fs.String("releases", "", "the releases as the GitHub API lists them (a JSON array, or arrays one per page)")
	dir := fs.String("stats", "stats", "the stats folder")
	fs.Parse(os.Args[2:])
	rels, err := readReleases(*relFile)
	if err != nil {
		fail(err)
	}
	now := time.Now()
	switch cmd {
	case "baseline":
		fail(writeJSON(filepath.Join(*dir, "baseline.json"), newBaseline(rels, now)))
	case "count":
		var base baseline
		if err := readJSON(filepath.Join(*dir, "baseline.json"), &base); err != nil {
			fail(err)
		}
		dev, err := ledger(filepath.Join(*dir, "ledger"), base.At)
		if err != nil {
			fail(err)
		}
		s := count(rels, base, dev)
		fail(writeJSON(filepath.Join(*dir, "downloads.json"), badge(s)))
		fail(writeJSON(filepath.Join(*dir, "summary.json"), s))
		fmt.Printf("people %d, development %d, not counted by GitHub yet %d\n", s.People, s.Development, s.NotCountedYet)
	default:
		fail(fmt.Errorf("unknown command %q", cmd))
	}
}

// readReleases reads `gh api --paginate` output: one JSON array per page, one after the other.
func readReleases(path string) ([]release, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []release
	dec := json.NewDecoder(f)
	for dec.More() {
		var page []release
		if err := dec.Decode(&page); err != nil {
			return nil, err
		}
		all = append(all, page...)
	}
	return all, nil
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

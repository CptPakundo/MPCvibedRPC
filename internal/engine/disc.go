package engine

import (
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A Blu-ray or DVD at the root of a drive (E:\BDMV\STREAM\00062.m2ts) has no folder above BDMV or VIDEO_TS to name
// it (core.DiscTitle). The disc names itself: many Blu-rays carry their title in BDMV\META\DL\bdmt_<lang>.xml, and
// every disc has a volume label. discDirs adds that name as the missing folder, so the parser treats it like a rip's
// folder. Only this computer's disc is read; the name goes where any title goes (the catalogs, Discord).

const discKeep = time.Minute // a disc is read again after this, so a swapped disc is noticed

var bdmtName = regexp.MustCompile(`<di:name>\s*([^<]{1,200}?)\s*</di:name>`)

// discDirs returns dirs with the disc's own name appended when the file plays from a disc at a drive's root, else
// dirs unchanged.
func discDirs(dirs []string, fullDir string, f *folderNames) []string {
	root, ok := discRoot(dirs, fullDir)
	if !ok {
		return dirs
	}
	name := f.discName(root)
	if name == "" {
		return dirs
	}
	return append(append([]string(nil), dirs...), name)
}

// discRoot is the disc's root folder when dirs are exactly a disc's folders (nothing above them).
func discRoot(dirs []string, fullDir string) (string, bool) {
	if fullDir == "" {
		return "", false
	}
	switch {
	case len(dirs) == 2 && (strings.EqualFold(dirs[0], "STREAM") || strings.EqualFold(dirs[0], "PLAYLIST")) && strings.EqualFold(dirs[1], "BDMV"):
		return filepath.Dir(filepath.Dir(filepath.Clean(fullDir))), true
	case len(dirs) == 1 && strings.EqualFold(dirs[0], "VIDEO_TS"):
		return filepath.Dir(filepath.Clean(fullDir)), true
	}
	return "", false
}

// discName is the disc's title from its bdmt file, else its volume label ("" when it has neither), kept for a minute.
func (f *folderNames) discName(root string) string {
	if f == nil {
		return readDiscName(root)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.discRoot == root && time.Since(f.discAt) < discKeep {
		return f.disc
	}
	f.discRoot, f.discAt, f.disc = root, time.Now(), readDiscName(root)
	return f.disc
}

func readDiscName(root string) string {
	if n := bdmtTitle(filepath.Join(root, "BDMV", "META", "DL")); n != "" {
		return n
	}
	return volumeLabel(root)
}

// bdmtTitle reads the disc title from the bdmt_<lang>.xml files in dir, English first.
func bdmtTitle(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var files []string
	for _, en := range ents {
		n := strings.ToLower(en.Name())
		if strings.HasPrefix(n, "bdmt_") && strings.HasSuffix(n, ".xml") {
			files = append(files, en.Name())
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		return strings.EqualFold(files[i], "bdmt_eng.xml") && !strings.EqualFold(files[j], "bdmt_eng.xml")
	})
	for _, name := range files {
		fh, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(fh, 64<<10))
		_ = fh.Close()
		if m := bdmtName.FindSubmatch(b); m != nil {
			if t := strings.TrimSpace(html.UnescapeString(string(m[1]))); t != "" {
				return t
			}
		}
	}
	return ""
}

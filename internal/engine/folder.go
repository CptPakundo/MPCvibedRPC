package engine

import (
	"os"
	"sync"
	"time"
)

// folderNames remembers the file names of the folder last listed for folderEpisodeNumbers. A status goes out again
// at every pause, resume and seek, and the folder (often on a slow or sleeping disk) is the same then: it is read
// again only for another file, another folder, or when the folder's own change time says its entries changed. The
// names stay in memory and are never logged. A nil *folderNames reads the folder every time.
type folderNames struct {
	mu        sync.Mutex
	dir, file string
	mod       time.Time
	names     []string
	reads     int // how often a folder was read (tests)
}

// list returns the names in dir for the file that plays (nil when the folder cannot be read).
func (f *folderNames) list(dir, file string) []string {
	if f == nil {
		return readNames(dir)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st, err := os.Stat(dir)
	if err != nil {
		f.reads++
		f.dir, f.file, f.names = "", "", nil // without the folder's change time nothing is kept
		return readNames(dir)
	}
	if f.names != nil && f.dir == dir && f.file == file && f.mod.Equal(st.ModTime()) {
		return f.names
	}
	// the change time was read first: a folder that changes while it is read is read again the next time
	f.reads++
	f.dir, f.file, f.mod, f.names = dir, file, st.ModTime(), readNames(dir)
	return f.names
}

// forget drops the remembered names.
func (f *folderNames) forget() {
	f.mu.Lock()
	f.dir, f.file, f.names = "", "", nil
	f.mu.Unlock()
}

func readNames(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, len(ents))
	for i, en := range ents {
		names[i] = en.Name()
	}
	return names
}

package engine

import (
	"strings"
	"testing"
)

// The log names the file the title came from (a wrong title can only be traced back to it), but never its folder path.
func TestLogNamesTheFile(t *testing.T) {
	e, m, d, l := setup(t)
	e.Start()
	eventually(t, "shown", func() bool { return len(d.list()) >= 1 && l.has("Playing: ") })
	if !l.has(`(MPC-HC: "Show.Name.S01E01.720p.mkv")`) {
		t.Fatalf("the log should name the file:\n%s", strings.Join(l.l, "\n"))
	}

	// a file that is just an episode number takes the show's name from its folder: the log says so
	n := len(d.list())
	m.set(func(m *mpc) { m.file, m.filepath = "E03.mkv", `C:\TV\Sample Show\Season 1\E03.mkv` })
	eventually(t, "the next file shown", func() bool { return len(d.list()) > n && l.has(`"E03.mkv"`) })
	if !l.has(`(MPC-HC: "E03.mkv", show name from its folder)`) {
		t.Fatalf("the log should say the name came from the folder:\n%s", strings.Join(l.l, "\n"))
	}
	l.mu.Lock()
	all := strings.Join(l.l, "\n")
	l.mu.Unlock()
	if strings.Contains(all, `C:\`) || strings.Contains(all, `TV\`) {
		t.Fatalf("the log must not record the folder path:\n%s", all)
	}
}

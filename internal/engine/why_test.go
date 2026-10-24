package engine

import (
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// "Why this title?" names the file and what was read from it while it shows, never the folder path, and is gone
// once nothing plays.
func TestStatusExplainsTheTitle(t *testing.T) {
	t.Parallel()
	e, m, d, _ := setup(t)
	e.Start()
	eventually(t, "shown", func() bool { return len(d.list()) >= 1 && len(e.Status().Why) > 0 })
	why := strings.Join(e.Status().Why, "\n")
	for _, want := range []string{`MPC-HC: "Show.Name.S01E01.720p.mkv"`, `Read as: title "Show Name"`, "season 1", "episode 1"} {
		if !strings.Contains(why, want) {
			t.Errorf("Why is missing %q:\n%s", want, why)
		}
	}

	m.set(func(m *mpc) { m.file, m.filepath = "E03.mkv", `C:\TV\Sample Show\Season 1\E03.mkv` })
	eventually(t, "the next file", func() bool { return strings.Contains(strings.Join(e.Status().Why, "\n"), `"E03.mkv"`) })
	why = strings.Join(e.Status().Why, "\n")
	if !strings.Contains(why, "show name from its folder") || strings.Contains(why, `C:\`) || strings.Contains(why, `TV\`) {
		t.Fatalf("Why should say the name came from the folder, without the path:\n%s", why)
	}

	m.set(func(m *mpc) { m.state = 0 })
	eventually(t, "nothing playing", func() bool { return e.Status().NowPlaying == nil })
	if w := e.Status().Why; len(w) != 0 {
		t.Fatalf("Why after playback stopped: %q", w)
	}
}

// A hidden title explains nothing: no file name, no title.
func TestHideTitleExplainsNothing(t *testing.T) {
	t.Parallel()
	e, _, d, _, _ := privacySetup(t, func(c *core.Config) { c.HideTitle = true })
	e.Start()
	eventually(t, "a status is sent", func() bool { return len(d.list()) >= 1 })
	if w := e.Status().Why; len(w) != 0 {
		t.Fatalf("Why with hideTitle: %q", w)
	}
}

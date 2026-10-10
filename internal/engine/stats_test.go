package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

func TestStatsCountWhatIsShownAndStayOnThisComputer(t *testing.T) {
	e, m, d, _ := setup(t)
	file := filepath.Join(t.TempDir(), "stats.json")
	e.opts.StatsFile = file
	e.stats = loadStats(file, time.Now())
	e.Start()
	eventually(t, "shown", func() bool { return len(d.list()) > 0 && !isClear(d) })
	eventually(t, "time counted", func() bool { return e.Stats().WatchedMs >= 250 })
	s := e.Stats()
	if s.Videos != 1 || s.Players["MPC-HC"] != 1 || s.Matched != 0 || s.Since != time.Now().Format("2006-01-02") {
		t.Fatalf("after one video: %+v", s)
	}
	// pausing and seeking is the same video
	m.set(func(m *mpc) { m.state = 1 })
	eventually(t, "paused", func() bool { return e.Status().Paused })
	paused := e.Stats().WatchedMs
	m.set(func(m *mpc) { m.state = 2; m.pos = 90000 })
	eventually(t, "playing", func() bool { return !e.Status().Paused })
	if s := e.Stats(); s.Videos != 1 {
		t.Errorf("pause or seek counted again: %+v", s)
	}
	if e.Stats().WatchedMs < paused {
		t.Error("watched time went back")
	}
	// another file is another video
	m.set(func(m *mpc) { m.file = "Sample.Show.S01E02.mkv"; m.filepath = `C:\Videos\Sample.Show.S01E02.mkv` })
	eventually(t, "second video", func() bool { return e.Stats().Videos == 2 })
	// a file on the Never show list is not counted
	cfg := e.Config()
	cfg.HideFiles = []string{"Hidden"}
	e.ApplySettings(cfg)
	m.set(func(m *mpc) { m.file = "Hidden Clip.mkv"; m.filepath = `C:\Videos\Hidden Clip.mkv` })
	eventually(t, "hidden", func() bool { return e.Status().Hidden })
	if s := e.Stats(); s.Videos != 2 {
		t.Errorf("a hidden file was counted: %+v", s)
	}
	e.Stop()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Sample", "Show", "Hidden", "Videos", ".mkv"} {
		if strings.Contains(string(b), leak) {
			t.Errorf("stats.json holds %q:\n%s", leak, b)
		}
	}
	// a new run picks the counts up again, and Reset starts over
	again := New(e.Config(), Options{StatsFile: file})
	if s := again.Stats(); s.Videos != 2 || s.Players["MPC-HC"] != 2 || s.WatchedMs < paused {
		t.Errorf("reloaded: %+v", s)
	}
	again.ResetStats()
	if s := New(e.Config(), Options{StatsFile: file}).Stats(); s.Videos != 0 || s.WatchedMs != 0 || len(s.Players) != 0 {
		t.Errorf("after reset: %+v", s)
	}
}

func TestStatsFileThatIsBrokenStartsOver(t *testing.T) {
	file := filepath.Join(t.TempDir(), "stats.json")
	os.WriteFile(file, []byte("{not json"), 0o644)
	if s := New(core.DefaultConfig(), Options{StatsFile: file}).Stats(); s.Videos != 0 || s.Since == "" || s.Players == nil {
		t.Errorf("%+v", s)
	}
}

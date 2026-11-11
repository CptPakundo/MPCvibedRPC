package engine

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

func TestStatsCountWhatIsShownAndStayOnThisComputer(t *testing.T) {
	t.Parallel()
	e, m, d, _ := setup(t)
	file := filepath.Join(t.TempDir(), "stats.json")
	e.opts.StatsFile = file
	e.stats = loadStats(file, time.Now())
	e.Start()
	eventually(t, "shown", func() bool { return len(d.list()) > 0 && !isClear(d) })
	// A real player's position advances; a fake that stands still reads as a seek after three seconds, which a slow
	// run of this test reached (that was the flake).
	stop, begun := make(chan struct{}), time.Now()
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				m.set(func(m *mpc) { m.pos = 60000 + int(time.Since(begun).Milliseconds()) })
			}
		}
	}()
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
	t.Parallel()
	file := filepath.Join(t.TempDir(), "stats.json")
	os.WriteFile(file, []byte("{not json"), 0o644)
	if s := New(core.DefaultConfig(), Options{StatsFile: file}).Stats(); s.Videos != 0 || s.Since == "" || s.Players == nil {
		t.Errorf("%+v", s)
	}
}

// The file is written after the engine's lock is released, so two writes can arrive in the wrong order: the older
// counts must not replace the newer ones.
func TestStatsFileNeverGoesBackToOlderCounts(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "stats.json")
	e := New(core.DefaultConfig(), Options{StatsFile: file})
	snapshot := func(videos int) *statWrite {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.stats.Videos, e.statDirty = videos, true
		return e.statSnapshot(time.Now())
	}
	older, newer := snapshot(1), snapshot(2)
	e.writeStats(newer)
	e.writeStats(older)
	if s := New(core.DefaultConfig(), Options{StatsFile: file}).Stats(); s.Videos != 2 {
		t.Errorf("the file holds %d videos, want the newer 2", s.Videos)
	}
	e.mu.Lock()
	again := e.statSnapshot(time.Now())
	e.mu.Unlock()
	if again != nil {
		t.Error("counts that were written are due again")
	}
}

// A write that fails (here: the data folder is missing) is not waited for under the engine's lock, leaves the
// counts due, and the next write brings them to the file.
func TestStatsWriteThatFailsIsTriedAgain(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "gone")
	file := filepath.Join(dir, "stats.json")
	e := New(core.DefaultConfig(), Options{StatsFile: file})
	e.ResetStats() // fails: there is no such folder
	e.mu.Lock()
	e.stats.Videos = 3
	due := e.statDirty
	e.mu.Unlock()
	if !due {
		t.Fatal("after a failed write the counts are not due")
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	save := e.statSnapshot(time.Now())
	e.mu.Unlock()
	e.writeStats(save)
	if s := New(core.DefaultConfig(), Options{StatsFile: file}).Stats(); s.Videos != 3 {
		t.Errorf("the file holds %d videos, want 3", s.Videos)
	}
}

// failingCatalogs answers no catalog request (the fake player on this computer is reached as usual).
type failingCatalogs struct {
	mu sync.Mutex
	n  int
}

func (f *failingCatalogs) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Hostname() == "127.0.0.1" {
		return http.DefaultTransport.RoundTrip(req)
	}
	f.mu.Lock()
	f.n++
	f.mu.Unlock()
	return nil, errors.New("no network in this test")
}

// An episode shown without its title (the catalogs did not answer) is sent again a few times while it plays, so a
// title that comes later reaches Discord; then it stops. Without catalog lookups nothing is repeated.
func TestEpisodeWithoutTitleIsAskedAgainWhilePlaying(t *testing.T) {
	t.Parallel()
	m := newMPC(t)
	e, d, _ := newTestEngine(t, withPort(m.port), shortPause, func(_ *core.Config, o *Options) { o.RecheckAfter = []float64{1, 1, 1} })
	cfg := e.Config()
	cfg.ShowArtwork, cfg.ArtworkWaitMs = true, 50
	e.cfg = cfg
	e.opts.HTTPClient = &http.Client{Transport: &failingCatalogs{}}
	m.set(func(m *mpc) { m.file = "Sample.Show.S01E02.mkv"; m.filepath = `C:\Videos\Sample.Show.S01E02.mkv` })
	// A real player's position advances. A fake that stands still reads as a seek after three seconds (a send of
	// its own), which a slow run of this test reached: that was the flake.
	stop, begun := make(chan struct{}), time.Now()
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			m.set(func(m *mpc) { m.pos = 60000 + int(time.Since(begun).Milliseconds()) })
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()
	e.Start()
	eventually(t, "shown", func() bool { return len(d.list()) > 0 && !isClear(d) })
	sent := func() int { return len(d.list()) }
	first := sent()
	eventually(t, "asked again three times", func() bool { return sent() >= first+3 })
	time.Sleep(time.Second)
	if n := sent(); n != first+3 {
		t.Errorf("sent %d times after the first, want 3", n-first)
	}
}

// A stop brings a state another goroutine took but has not written yet (statLast), and writing a state twice is
// harmless.
func TestStatsStopFlushesAStateStillOnItsWay(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "stats.json")
	e := New(core.DefaultConfig(), Options{StatsFile: file})
	e.mu.Lock()
	e.stats.Videos, e.statDirty = 4, true
	taken := e.statSnapshot(time.Now()) // taken, not yet written
	again := e.statSnapshot(time.Now()) // what stop() sees: nothing new...
	last := e.statLast                  // ...so it writes the newest state taken
	e.mu.Unlock()
	if again != nil || last != taken {
		t.Fatalf("again=%v last==taken:%v", again, last == taken)
	}
	e.writeStats(last)
	e.writeStats(taken) // the other goroutine arrives later: nothing to do
	if s := New(core.DefaultConfig(), Options{StatsFile: file}).Stats(); s.Videos != 4 {
		t.Errorf("the file holds %d videos, want 4", s.Videos)
	}
}

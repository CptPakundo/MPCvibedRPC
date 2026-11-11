package engine

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
)

// count is how many log lines contain sub.
func (l *logs) count(sub string) (n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.l {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

// statuses is how many of the requests the fake Discord got carried a status (the others cleared it).
func statuses(d *disc) (n int) {
	for _, a := range d.list() {
		if _, has := a["activity"]; has {
			n++
		}
	}
	return n
}

// dropConnections closes every connection of the fake Discord, as a Discord that restarts does.
func dropConnections(d *disc) {
	d.mu.Lock()
	for _, c := range d.conns {
		c.Close()
	}
	d.mu.Unlock()
}

// A status that was cleared after a long pause stays cleared when Discord restarts (it does so for every update of
// its own): only the player brings it back.
func TestReconnectKeepsAPauseClearedStatusCleared(t *testing.T) {
	t.Parallel()
	e, m, d, l := setup(t)
	cfg := e.Config()
	cfg.PauseClearMinutes = 1
	e.ApplySettings(cfg)
	e.Start()
	eventually(t, "playing update", func() bool { return len(d.list()) >= 1 })
	pauseUntilCleared(t, e, m, d)
	before := statuses(d)

	dropConnections(d)
	eventually(t, "the connection lost and made again", func() bool {
		return l.has("Lost connection to Discord") && l.count("Connected to Discord") >= 2
	})
	time.Sleep(3 * 300 * time.Millisecond) // three of the test's "minutes"
	if n := statuses(d); n != before {
		t.Fatalf("%d statuses were sent after the reconnect: the paused status came back", n-before)
	}
	if s := e.Status(); !s.PauseCleared || len(d.showing()) != 0 {
		t.Fatalf("after the reconnect: pauseCleared=%v, Discord shows %v", s.PauseCleared, d.showing())
	}
	m.set(func(m *mpc) { m.state = 2 })
	eventually(t, "the status back on resume", func() bool { return len(d.showing()) == 1 && !e.Status().PauseCleared })
}

// A reconnect during a pause that is still shown sends the status again, but does not start the pause limit anew.
func TestReconnectDoesNotRestartThePauseLimit(t *testing.T) {
	t.Parallel()
	e, m, d, l := setup(t)
	cfg := e.Config()
	cfg.PauseClearMinutes = 3 // 900 ms in the test
	e.ApplySettings(cfg)
	e.Start()
	eventually(t, "playing update", func() bool { return len(d.list()) >= 1 })
	pause(t, m, d)
	paused := time.Now()
	time.Sleep(400 * time.Millisecond)
	dropConnections(d)
	eventually(t, "the paused status back on the new connection", func() bool {
		return l.count("Connected to Discord") >= 2 && len(d.showing()) == 1
	})
	eventually(t, "cleared after the limit", func() bool { return e.Status().PauseCleared && len(d.showing()) == 0 })
	if took := time.Since(paused); took > 1300*time.Millisecond+10*testPoll {
		t.Errorf("the status was cleared %v after the pause: the reconnect restarted the 900 ms limit", took)
	}
}

// A title Discord refuses as a Watching activity gets the basic one; the next title is tried in full again.
func TestOneRefusedTitleDoesNotCostTheNext(t *testing.T) {
	t.Parallel()
	e, m, d, l := setup(t)
	d.set(func(d *disc) { d.reject = true })
	e.Start()
	eventually(t, "the basic status", func() bool {
		s := d.showing()
		return l.has("Falling back to basic mode") && len(s) == 1 && s[0]["type"] == nil
	})
	d.set(func(d *disc) { d.reject = false })
	m.set(func(m *mpc) {
		m.file, m.filepath = "Sample.Show.S01E02.mkv", `C:\TV\Sample Show\Season 1\Sample.Show.S01E02.mkv`
	})
	eventually(t, "the next title as a Watching activity", func() bool {
		s := d.showing()
		return len(s) == 1 && s[0]["type"] == float64(3)
	})
	_ = e
}

// A clear after a long pause that Discord does not take is named once in the log, the window goes on showing what
// Discord still shows, and the clear is tried until it works.
func TestFailedPauseClearIsNamedOnceAndRetried(t *testing.T) {
	t.Parallel()
	e, m, d, l := setup(t)
	cfg := e.Config()
	cfg.PauseClearMinutes = 1
	e.ApplySettings(cfg)
	e.Start()
	eventually(t, "playing update", func() bool { return len(d.list()) >= 1 })
	d.set(func(d *disc) { d.failClears = true })
	pause(t, m, d)
	eventually(t, "a clear that failed", func() bool { return l.has("did not take the clearing") })
	time.Sleep(20 * testPoll)
	if n := l.count("did not take the clearing"); n != 1 {
		t.Errorf("the failed clear was logged %d times", n)
	}
	if s := e.Status(); s.Preview == nil || s.PauseCleared || len(d.showing()) != 1 {
		t.Errorf("while the clear fails: preview=%v pauseCleared=%v, Discord shows %d", s.Preview != nil, s.PauseCleared, len(d.showing()))
	}
	d.set(func(d *disc) { d.failClears = false })
	eventually(t, "the clear once Discord takes it", func() bool { return e.Status().PauseCleared && len(d.showing()) == 0 })
}

// A panic while connecting to Discord is logged, and the next check connects.
func TestConnectingSurvivesAPanic(t *testing.T) {
	t.Parallel()
	m := newMPC(t)
	calls := 0
	var path string
	e, d, l := newTestEngine(t, withPort(m.port), func(_ *core.Config, o *Options) {
		path = o.DiscordPaths[0]
		o.NewDiscord = func() *discord.Client {
			if calls++; calls == 1 { // only the connect goroutine of one run calls this at a time
				panic("a connect that goes wrong")
			}
			return discord.New([]string{path})
		}
	})
	e.Start()
	eventually(t, "the panic in the log", func() bool { return l.has("stopped after an error") })
	eventually(t, "connected at a later check", func() bool { return e.Status().Discord == "connected" && len(d.showing()) == 1 })
}

// A settings change while a status is on its way to Discord leaves no error from the old run in the new one.
func TestSettingsChangeDuringASendLeavesNoError(t *testing.T) {
	t.Parallel()
	e, m, d, l := setup(t)
	e.Start()
	eventually(t, "first status", func() bool { return len(d.showing()) == 1 })
	d.set(func(d *disc) { d.slow = 12 * testPoll })
	m.set(func(m *mpc) { m.pos += 600000 }) // a seek: a status goes out and waits for the slow answer
	n := len(d.list())
	eventually(t, "the seek on its way", func() bool { return len(d.list()) > n })
	cfg := e.Config()
	cfg.TextProgressBar = !cfg.TextProgressBar
	e.ApplySettings(cfg) // ends the run while the send waits
	d.set(func(d *disc) { d.slow = 0 })
	eventually(t, "the new run's status", func() bool {
		s := e.Status()
		return s.Discord == "connected" && s.NowPlaying != nil && len(d.showing()) == 1
	})
	if s := e.Status(); s.LastError != nil {
		t.Errorf("the new run shows the old run's error: %s", *s.LastError)
	}
	if l.has("Failed to update presence") {
		t.Errorf("the ended run's send was logged as a failure:\n%s", strings.Join(l.l, "\n"))
	}
}

// The engine's state is spread over many fields that several goroutines change. Whatever happens in whatever order
// (the player pauses, seeks, changes file, stops or goes away; Discord refuses, answers late, fails clears or
// restarts; the settings change; the engine is stopped and started), once things are quiet Discord must show
// exactly what the player's last state calls for: one status for the file that plays, or none.
//
// Eight fixed orders run every time. MPCRPC_CONVERGE_SEEDS=500 tries that many (for a hunt after a change to the tick).
func TestStatusConverges(t *testing.T) {
	seeds := int64(8)
	if n, err := strconv.ParseInt(os.Getenv("MPCRPC_CONVERGE_SEEDS"), 10, 64); err == nil && n > 0 {
		seeds = n
	}
	for seed := int64(1); seed <= seeds; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewSource(seed))
			m := newMPC(t)
			e, d, l := newTestEngine(t, withPort(m.port), func(_ *core.Config, o *Options) { o.ReclearEvery = 8 * testPoll })
			episode := 1
			m.set(func(m *mpc) {
				m.file, m.filepath = "Sample.Show.S01E01.mkv", `C:\TV\Sample Show\Season 1\Sample.Show.S01E01.mkv`
			})
			steps := []struct {
				name string
				do   func()
			}{
				{"play", func() { m.set(func(m *mpc) { m.state, m.down = 2, false }) }},
				{"pause", func() { m.set(func(m *mpc) { m.state = 1 }) }},
				{"seek", func() { m.set(func(m *mpc) { m.pos = 60000 + rng.Intn(1200000) }) }},
				{"next file", func() {
					episode = episode%9 + 1
					name := fmt.Sprintf("Sample.Show.S01E%02d.mkv", episode)
					m.set(func(m *mpc) { m.file, m.filepath, m.pos = name, `C:\TV\Sample Show\Season 1\`+name, 60000 })
				}},
				{"stop", func() { m.set(func(m *mpc) { m.state = 0 }) }},
				{"player closed", func() { m.set(func(m *mpc) { m.down = true }) }},
				{"player back", func() { m.set(func(m *mpc) { m.down = false }) }},
				{"Discord refuses Watching", func() { d.set(func(d *disc) { d.reject = !d.reject }) }},
				{"Discord refuses everything", func() { d.set(func(d *disc) { d.rejectAll = !d.rejectAll }) }},
				{"Discord fails clears", func() { d.set(func(d *disc) { d.failClears = !d.failClears }) }},
				{"Discord answers late", func() {
					d.set(func(d *disc) {
						if d.slow == 0 {
							d.slow = 3 * testPoll
						} else {
							d.slow = 0
						}
					})
				}},
				{"Discord drops the next request", func() { d.set(func(d *disc) { d.dropNext = true }) }},
				{"Discord restarts", func() { dropConnections(d) }},
				{"settings change", func() {
					cfg := e.Config()
					cfg.TextProgressBar = !cfg.TextProgressBar
					e.ApplySettings(cfg)
				}},
				{"stop and start", func() { e.Stop(); e.Start() }},
				{"wake", func() { e.Wake() }},
			}
			e.Start()
			var done []string
			for i := 0; i < 40; i++ {
				s := steps[rng.Intn(len(steps))]
				done = append(done, s.name)
				s.do()
				time.Sleep(time.Duration(rng.Intn(3*int(testPoll))) + time.Millisecond)
			}
			// quiet: Discord behaves again, the player keeps its last state
			d.set(func(d *disc) { d.reject, d.rejectAll, d.failClears, d.slow, d.dropNext = false, false, false, 0, false })
			e.Start()
			m.mu.Lock()
			active, paused := !m.down && (m.state == 1 || m.state == 2), m.state == 1
			m.mu.Unlock()
			problem := ""
			settled := func() bool {
				shown := d.showing()
				s := e.Status()
				if !active {
					if problem = fmt.Sprintf("nothing plays, but Discord shows %v (status %+v)", shown, s); len(shown) != 0 {
						return false
					}
					return s.NowPlaying == nil && s.Preview == nil
				}
				if problem = fmt.Sprintf("episode %d plays (paused: %v), but Discord shows %v (status %+v)", episode, paused, shown, s); len(shown) != 1 {
					return false
				}
				b, _ := json.Marshal(shown[0])
				txt := string(b)
				// The basic status (sent to a Discord that refused the Watching one for this file) carries the show's
				// name only: its form is pinned by the recorded data (core.LegacyActivity).
				rightFile := strings.Contains(txt, fmt.Sprintf("Episode %d", episode)) || strings.Contains(txt, fmt.Sprintf("E%02d", episode))
				if shown[0]["type"] == nil {
					rightFile = strings.Contains(txt, "Sample Show")
				}
				showsPause := strings.Contains(txt, "Paused") || strings.Contains(txt, "⏸")
				return rightFile && showsPause == paused && s.NowPlaying != nil && s.LastError == nil && s.Paused == paused
			}
			for i := 0; i < 400 && !settled(); i++ {
				time.Sleep(20 * time.Millisecond)
			}
			if !settled() {
				l.mu.Lock()
				tail := l.l[max(0, len(l.l)-25):]
				l.mu.Unlock()
				t.Fatalf("%s\nsteps: %s\nlog tail:\n%s", problem, strings.Join(done, ", "), strings.Join(tail, "\n"))
			}
		})
	}
}

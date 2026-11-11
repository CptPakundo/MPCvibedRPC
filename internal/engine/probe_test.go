package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

func probeNamed(ps []PlayerProbe, part string) *PlayerProbe {
	for i := range ps {
		if strings.Contains(ps[i].Name, part) {
			return &ps[i]
		}
	}
	return nil
}

// The probe says whether a player answers and what it does, and leaves the engine as it was: no status, no log line.
func TestProbeAsksOnceAndChangesNothing(t *testing.T) {
	m := newMPC(t)
	e, _, l := newTestEngine(t, withPort(m.port))
	before, logged := e.Status(), len(l.list())

	ps := e.Probe(context.Background())
	p := probeNamed(ps, "MPC-")
	if p == nil || !p.Answers || p.State != ProbePlaying {
		t.Fatalf("the web interface plays: %+v", ps)
	}
	m.set(func(m *mpc) { m.state = 1 })
	if p := probeNamed(e.Probe(context.Background()), "MPC-"); p == nil || !p.Answers || p.State != ProbePaused {
		t.Fatalf("paused: %+v", p)
	}
	m.set(func(m *mpc) { m.down = true })
	if p := probeNamed(e.Probe(context.Background()), "MPC-"); p == nil || p.Answers || p.State != "" {
		t.Fatalf("down: %+v", p)
	}

	if after := e.Status(); !reflect.DeepEqual(before, after) {
		t.Errorf("the status changed:\n%+v\n%+v", before, after)
	}
	if n := len(l.list()); n != logged {
		t.Errorf("the probe wrote log lines: %v", l.list()[logged:])
	}
	if e.Status().Running {
		t.Error("the probe started the engine")
	}
}

// Accounts are read from the settings only.
func TestProbeReportsAccountsWithoutAsking(t *testing.T) {
	e, _, _ := newTestEngine(t, func(cfg *core.Config, _ *Options) { cfg.PlexToken = "x" })
	ps := e.Probe(context.Background())
	if p := probeNamed(ps, "Plex"); p == nil || !p.Account || !p.Answers {
		t.Errorf("Plex: %+v", p)
	}
	if p := probeNamed(ps, "Stremio"); p == nil || !p.Account || p.Answers {
		t.Errorf("Stremio: %+v", p)
	}
}

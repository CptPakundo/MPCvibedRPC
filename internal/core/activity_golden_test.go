package core

import (
	"encoding/json"
	"reflect"
	"testing"
)

type goldenInfo struct {
	File     string   `json:"file"`
	Dir      []string `json:"dir"`
	State    int      `json:"state"`
	Position int      `json:"position"`
	Duration int      `json:"duration"`
	Rate     float64  `json:"rate"`
}

func canon(t *testing.T, v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var o any
	json.Unmarshal(b, &o)
	return o
}

func TestActivityAgainstJS(t *testing.T) {
	var g struct {
		Now   int64 `json:"now"`
		Cases []struct {
			Info   goldenInfo      `json:"info"`
			Cfg    map[string]any  `json:"cfg"`
			Art    *Art            `json:"art"`
			Badge  bool            `json:"badge"`
			Act    json.RawMessage `json:"act"`
			Legacy json.RawMessage `json:"legacy"`
			Error  string          `json:"error"`
		} `json:"cases"`
		NU []struct {
			Prev *struct {
				File     string  `json:"file"`
				Dir      string  `json:"dir"`
				State    int     `json:"state"`
				Position int     `json:"position"`
				Rate     float64 `json:"rate"`
				At       int64   `json:"at"`
			} `json:"prev"`
			Info goldenInfo         `json:"info"`
			Now  int64              `json:"now"`
			Res  [2]json.RawMessage `json:"res"`
		} `json:"nu"`
		FT []struct {
			Ms  int    `json:"ms"`
			Out string `json:"out"`
		} `json:"ft"`
		PB []struct {
			P, D, W int
			Out     string `json:"out"`
		} `json:"pb"`
	}
	loadGolden(t, "activity.json.gz", &g)
	bad := 0
	for i, c := range g.Cases {
		cc := ConfigFrom(c.Cfg)
		info := &Info{File: c.Info.File, Dir: c.Info.Dir, State: c.Info.State, Position: c.Info.Position, Duration: c.Info.Duration, Rate: c.Info.Rate}
		var media *Media
		if c.Badge {
			media = ParseMedia(info.File, &cc, info.Dir)
			media.BadgeEpisode = 7
		}
		act := BuildActivity(info, &cc, g.Now, media, c.Art)
		var want, wantL any
		json.Unmarshal(c.Act, &want)
		json.Unmarshal(c.Legacy, &wantL)
		ok := reflect.DeepEqual(canon(t, act), want)
		var leg any
		if act != nil {
			leg = canon(t, LegacyActivity(act, &cc))
		}
		okL := reflect.DeepEqual(leg, wantL)
		if !ok || !okL {
			bad++
			if bad <= 12 {
				t.Errorf("#%d %q cfg=%v state=%d\n got  %s\n want %s\n legacy ok=%v", i, info.File, c.Cfg, info.State, mustJSON(act), c.Act, okL)
			}
		}
	}
	t.Logf("%d activity cases, %d mismatches", len(g.Cases), bad)
	for _, n := range g.NU {
		var prev *Prev
		if n.Prev != nil {
			prev = &Prev{File: n.Prev.File, Dir: n.Prev.Dir, State: n.Prev.State, Position: n.Prev.Position, Rate: n.Prev.Rate, At: n.Prev.At}
		}
		info := &Info{File: n.Info.File, Dir: n.Info.Dir, State: n.Info.State, Position: n.Info.Position, Rate: n.Info.Rate}
		upd, _ := NeedsUpdate(prev, info, n.Now)
		var want bool
		json.Unmarshal(n.Res[0], &want)
		if upd != want {
			t.Errorf("NeedsUpdate now=%d: got %v want %v", n.Now, upd, want)
		}
	}
	for _, f := range g.FT {
		if got := FmtTime(f.Ms); got != f.Out {
			t.Errorf("FmtTime(%d)=%q want %q", f.Ms, got, f.Out)
		}
	}
	for _, p := range g.PB {
		if got := ProgressBar(p.P, p.D, p.W); got != p.Out {
			t.Errorf("ProgressBar(%d,%d,%d)=%q want %q", p.P, p.D, p.W, got, p.Out)
		}
	}
	if bad > 0 {
		t.Fail()
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

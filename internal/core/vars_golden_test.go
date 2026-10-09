package core

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestParseVariablesAgainstJS(t *testing.T) {
	f, err := os.Open("testdata/vars.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		HTML string
		Out  *struct {
			File     string
			Dir      []string
			FullDir  string
			State    *int
			Position int
			Duration int
			Rate     float64
		}
	}
	if err := json.NewDecoder(zr).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	bad, parsed := 0, 0
	for i, c := range cases {
		got := ParseVariables(c.HTML)
		if c.Out == nil {
			if got != nil {
				bad++
				t.Errorf("case %d: want nil, got %+v\n%q", i, got, c.HTML)
			}
			continue
		}
		parsed++
		if got == nil {
			bad++
			t.Errorf("case %d: got nil\n%q", i, c.HTML)
			continue
		}
		state := NA
		if c.Out.State != nil {
			state = *c.Out.State
		}
		want := &Info{File: c.Out.File, Dir: c.Out.Dir, FullDir: c.Out.FullDir, State: state, Position: c.Out.Position, Duration: c.Out.Duration, Rate: c.Out.Rate}
		if want.Dir == nil {
			want.Dir = []string{}
		}
		g := *got
		if g.Dir == nil {
			g.Dir = []string{}
		}
		if !reflect.DeepEqual(&g, want) {
			bad++
			if bad < 8 {
				t.Errorf("case %d:\n got %+v\nwant %+v\n%q", i, g, *want, c.HTML)
			}
		}
	}
	if parsed < 1000 {
		t.Fatalf("only %d parsed cases", parsed)
	}
	t.Logf("%d cases, %d parsed, %d mismatches", len(cases), parsed, bad)
}

package artwork

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

type fuzzCand struct {
	Titles   []*string `json:"titles"`
	Year     float64   `json:"year"`
	Poster   string    `json:"poster"`
	ViaAlias bool      `json:"viaAlias"`
	Runtime  float64   `json:"runtime"`
	Eps      float64   `json:"eps"`
}

func (f fuzzCand) cand() *Cand {
	var t []string
	for _, x := range f.Titles {
		if x != nil && *x != "" {
			t = append(t, *x)
		}
	}
	return &Cand{Titles: t, Year: int(f.Year), Poster: f.Poster, ViaAlias: f.ViaAlias, Runtime: int(f.Runtime), Eps: int(f.Eps)}
}

func candsOf(fs []fuzzCand) []*Cand {
	out := make([]*Cand, len(fs))
	for i, f := range fs {
		out[i] = f.cand()
	}
	return out
}

func TestFuzzAgainstJS(t *testing.T) {
	f, err := os.Open("testdata/fuzz.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, _ := gzip.NewReader(f)
	var d struct {
		Pick []struct {
			Cands  []fuzzCand `json:"cands"`
			Q      string     `json:"q"`
			Year   *float64   `json:"year"`
			Strict bool       `json:"strict"`
			O      struct {
				Earliest, ExactOnly, TrustAlias, Ranked bool
				Runtime                                 float64
				Contain                                 []string
			} `json:"o"`
			Res *struct {
				Poster  string `json:"poster"`
				ViaRank bool   `json:"viaRank"`
			} `json:"res"`
		} `json:"pick"`
		Season []struct {
			Cands []fuzzCand `json:"cands"`
			Base  string     `json:"base"`
			Spec  struct {
				Season     *float64 `json:"season"`
				CodePrefix *string  `json:"codePrefix"`
			} `json:"spec"`
			Res *struct {
				Poster     string `json:"poster"`
				SeasonName string `json:"seasonName"`
			} `json:"res"`
		} `json:"season"`
		Chain []struct {
			Cands  []fuzzCand `json:"cands"`
			Base   string     `json:"base"`
			Prefix string     `json:"prefix"`
			N      *float64   `json:"n"`
			Res    *struct {
				Poster     string `json:"poster"`
				Shift      int    `json:"shift"`
				SeasonName string `json:"seasonName"`
			} `json:"res"`
		} `json:"chain"`
		Mark []struct {
			Rem []string `json:"rem"`
			Res struct {
				N     *float64 `json:"n"`
				Final bool     `json:"final"`
			} `json:"res"`
		} `json:"mark"`
		EpPick []struct {
			List []struct {
				Season, Number float64
				Name           string
				Entry          bool
			} `json:"list"`
			Media struct {
				Episode *float64 `json:"episode"`
				Season  *float64 `json:"season"`
			} `json:"media"`
			O struct {
				Split     bool     `json:"split"`
				SeasonNo  *float64 `json:"seasonNo"`
				CodeBlock bool     `json:"codeBlock"`
			} `json:"o"`
			Res *struct {
				Name     string `json:"name"`
				Absolute bool   `json:"absolute"`
			} `json:"res"`
		} `json:"epPick"`
	}
	if err := json.NewDecoder(z).Decode(&d); err != nil {
		t.Fatal(err)
	}
	bad := 0
	fail := func(format string, a ...any) {
		bad++
		if bad <= 20 {
			t.Errorf(format, a...)
		}
	}
	for i, c := range d.Pick {
		y := 0
		if c.Year != nil {
			y = int(*c.Year)
		}
		got := pickMatch(candsOf(c.Cands), c.Q, y, c.Strict, PickOpts{Earliest: c.O.Earliest, ExactOnly: c.O.ExactOnly, TrustAlias: c.O.TrustAlias, Ranked: c.O.Ranked, Runtime: int(c.O.Runtime), Contain: c.O.Contain})
		switch {
		case c.Res == nil && got != nil:
			fail("pick #%d %q year=%d strict=%v opts=%+v: got %s want null", i, c.Q, y, c.Strict, c.O, got.Poster)
		case c.Res != nil && (got == nil || got.Poster != c.Res.Poster || got.ViaRank != c.Res.ViaRank):
			fail("pick #%d %q year=%d strict=%v opts=%+v cands=%s: got %v want %+v", i, c.Q, y, c.Strict, c.O, mustJSON2(c.Cands), got, c.Res)
		}
	}
	for i, c := range d.Season {
		season, prefix := core.NA, ""
		if c.Spec.Season != nil {
			season = int(*c.Spec.Season)
		}
		if c.Spec.CodePrefix != nil {
			prefix = *c.Spec.CodePrefix
		}
		got := pickSeasonEntry(candsOf(c.Cands), c.Base, season, prefix)
		switch {
		case c.Res == nil && got != nil:
			fail("season #%d %q: got %s want null", i, c.Base, got.Cand.Poster)
		case c.Res != nil && (got == nil || got.Cand.Poster != c.Res.Poster || got.SeasonName != c.Res.SeasonName):
			fail("season #%d %q spec=%+v cands=%s: got %+v want %+v", i, c.Base, c.Spec, mustJSON2(c.Cands), got, c.Res)
		}
	}
	for i, c := range d.Chain {
		cs := candsOf(c.Cands)
		n := core.NA
		if c.N != nil {
			n = int(*c.N)
		}
		got := chainEntry(cs, c.Base, c.Prefix, cs[0], n)
		switch {
		case c.Res == nil && got != nil:
			fail("chain #%d: got %+v want null", i, got)
		case c.Res != nil && (got == nil || got.Cand.Poster != c.Res.Poster || got.Shift != c.Res.Shift || got.SeasonName != c.Res.SeasonName):
			fail("chain #%d %q n=%d: got %+v want %+v cands=%s", i, c.Base, n, got, c.Res, mustJSON2(c.Cands))
		}
	}
	for i, c := range d.Mark {
		got := parseSeasonMark(c.Rem)
		wn := core.NA
		if c.Res.N != nil {
			wn = int(*c.Res.N)
		}
		if got.N != wn || got.Final != c.Res.Final {
			fail("mark #%d %v: got %+v want %+v", i, c.Rem, got, c.Res)
		}
	}
	for i, c := range d.EpPick {
		var items []EpItem
		for _, x := range c.List {
			items = append(items, EpItem{Season: x.Season, Number: x.Number, Name: x.Name, Entry: x.Entry})
		}
		m := core.NewMedia()
		if c.Media.Episode != nil {
			m.Episode = int(*c.Media.Episode)
		}
		if c.Media.Season != nil {
			m.Season = int(*c.Media.Season)
		}
		po := pickOpts{split: c.O.Split, seasonNo: core.NA, codeBlock: c.O.CodeBlock}
		if c.O.SeasonNo != nil {
			po.seasonNo = int(*c.O.SeasonNo)
		}
		got := pickEpisode(items, m, po)
		switch {
		case c.Res == nil && got != nil:
			fail("epPick #%d: got %+v want null", i, got)
		case c.Res != nil && (got == nil || got.e.Name != c.Res.Name || got.absolute != c.Res.Absolute):
			fail("epPick #%d media=%+v o=%+v: got %+v want %+v", i, c.Media, c.O, got, c.Res)
		}
	}
	t.Logf("%d pick, %d season, %d chain, %d mark, %d epPick scenarios; %d mismatches", len(d.Pick), len(d.Season), len(d.Chain), len(d.Mark), len(d.EpPick), bad)
}

func mustJSON2(v any) string { b, _ := json.Marshal(v); return string(b) }

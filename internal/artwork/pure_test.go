package artwork

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

type jsCand struct {
	Name     string   `json:"name"`
	Titles   []string `json:"titles"`
	Year     float64  `json:"year"`
	Poster   string   `json:"poster"`
	ViaAlias bool     `json:"viaAlias"`
	Runtime  float64  `json:"runtime"`
	Eps      float64  `json:"eps"`
	URL      string   `json:"url"`
}

func (j jsCand) cand() *Cand {
	t := j.Titles
	if t == nil && j.Name != "" {
		t = []string{j.Name}
	}
	var tt []string
	for _, x := range t {
		if x != "" {
			tt = append(tt, x)
		}
	}
	return &Cand{Titles: tt, Year: int(j.Year), Poster: j.Poster, ViaAlias: j.ViaAlias, Runtime: int(j.Runtime), Eps: int(j.Eps), URL: j.URL}
}

func decodeArgs(t *testing.T, args []json.RawMessage, into ...any) {
	for i, p := range into {
		if i < len(args) {
			if err := json.Unmarshal(args[i], p); err != nil {
				t.Fatalf("arg %d: %v", i, err)
			}
		}
	}
}

func TestPureFunctionsAgainstJS(t *testing.T) {
	tr := loadTranscripts(t)
	n := 0
	for _, c := range tr.Calls {
		n++
		switch c.Fn {
		case "normTitle":
			var s, want string
			decodeArgs(t, c.Args, &s)
			json.Unmarshal(c.Result, &want)
			if got := normTitle(s); got != want {
				t.Errorf("normTitle(%q)=%q want %q", s, got, want)
			}
		case "usable":
			var s string
			var want bool
			decodeArgs(t, c.Args, &s)
			json.Unmarshal(c.Result, &want)
			if got := usable(s); got != want {
				t.Errorf("usable(%q)=%v want %v", s, got, want)
			}
		case "pickMatch":
			var cs []jsCand
			var title string
			var year *float64
			var strict bool
			var o struct {
				Earliest, ExactOnly, TrustAlias, Ranked bool
				Runtime                                 float64
				Contain                                 []string
			}
			decodeArgs(t, c.Args, &cs, &title, &year, &strict, &o)
			var cands []*Cand
			for _, j := range cs {
				cands = append(cands, j.cand())
			}
			y := 0
			if year != nil {
				y = int(*year)
			}
			got := pickMatch(cands, title, y, strict, PickOpts{Earliest: o.Earliest, ExactOnly: o.ExactOnly, TrustAlias: o.TrustAlias, Ranked: o.Ranked, Runtime: int(o.Runtime), Contain: o.Contain})
			var want *jsCand
			json.Unmarshal(c.Result, &want)
			switch {
			case want == nil && got != nil:
				t.Errorf("pickMatch(%q) = %v want null", title, got.Titles)
			case want != nil && got == nil:
				t.Errorf("pickMatch(%q) = nil want %v", title, want)
			case want != nil && (got.Poster != want.Poster || got.Year != int(want.Year)):
				t.Errorf("pickMatch(%q) = %v want %v", title, got, want)
			}
		case "parseSeasonMark":
			var rem []string
			decodeArgs(t, c.Args, &rem)
			var want struct {
				N     *float64 `json:"n"`
				Final bool     `json:"final"`
			}
			json.Unmarshal(c.Result, &want)
			got := parseSeasonMark(rem)
			wn := core.NA
			if want.N != nil {
				wn = int(*want.N)
			}
			if got.N != wn || got.Final != want.Final {
				t.Errorf("parseSeasonMark(%v)=%+v want %+v", rem, got, want)
			}
		case "matchSeasonByCode":
			var seasons []any
			var prefix string
			var raw []json.RawMessage = c.Args
			json.Unmarshal(raw[0], &seasons)
			json.Unmarshal(raw[1], &prefix)
			v, _ := parseJSON(string(raw[0]))
			got := matchSeasonByCode(asArr(v), prefix)
			var want map[string]any
			json.Unmarshal(c.Result, &want)
			switch {
			case want == nil && got != nil:
				t.Errorf("matchSeasonByCode(%q) = %v want null", prefix, got)
			case want != nil && (got == nil || str(get(got, "name")) != want["name"]):
				t.Errorf("matchSeasonByCode(%q) = %v want %v", prefix, got, want)
			}
		case "titleVariants":
			var media map[string]any
			var query string
			decodeArgs(t, c.Args, &media, &query)
			m := mediaFromJSON(media)
			got := titleVariants(m, query)
			var want []struct {
				Query   string   `json:"query"`
				Contain []string `json:"contain"`
				First   bool     `json:"first"`
				Year    float64  `json:"year"`
			}
			json.Unmarshal(c.Result, &want)
			if len(got) != len(want) {
				t.Errorf("titleVariants(%q): %d variants want %d", query, len(got), len(want))
				continue
			}
			for i := range got {
				if got[i].query != want[i].Query || got[i].first != want[i].First || !reflect.DeepEqual(got[i].contain, want[i].Contain) {
					t.Errorf("titleVariants(%q)[%d] = %+v want %+v", query, i, got[i], want[i])
				}
			}
		case "pickEpisode":
			var list []struct {
				Season, Number float64
				Name           string
				Entry          bool
			}
			var media map[string]any
			var o struct {
				Split     bool     `json:"split"`
				SeasonNo  *float64 `json:"seasonNo"`
				CodeBlock bool     `json:"codeBlock"`
			}
			decodeArgs(t, c.Args, &list, &media, &o)
			var items []EpItem
			for _, x := range list {
				items = append(items, EpItem{Season: x.Season, Number: x.Number, Name: x.Name, Entry: x.Entry})
			}
			po := pickOpts{split: o.Split, seasonNo: core.NA, codeBlock: o.CodeBlock}
			if o.SeasonNo != nil {
				po.seasonNo = int(*o.SeasonNo)
			}
			got := pickEpisode(items, mediaFromJSON(media), po)
			var want *struct {
				E struct {
					Season, Number float64
					Name           string
				}
				Absolute bool
			}
			json.Unmarshal(c.Result, &want)
			switch {
			case want == nil && got != nil:
				t.Errorf("pickEpisode = %+v want null", got)
			case want != nil && (got == nil || got.e.Name != want.E.Name || got.absolute != want.Absolute):
				t.Errorf("pickEpisode = %+v want %+v", got, want)
			}
		case "parseEpisodeList":
			var s string
			decodeArgs(t, c.Args, &s)
			var want map[string]string
			json.Unmarshal(c.Result, &want)
			got := parseEpisodeList(s)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("parseEpisodeList = %v want %v", got, want)
			}
		default:
			n--
		}
	}
	t.Logf("%d pure calls checked", n)
}

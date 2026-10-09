package jsre

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"unicode/utf16"
)

type goldenFile struct {
	Pool []string `json:"pool"`
	Regs []struct {
		Pattern string `json:"pattern"`
		Flags   string `json:"flags"`
		File    string `json:"file"`
		Results map[string]struct {
			M     [][]json.RawMessage `json:"m"`
			Rep   string              `json:"rep"`
			Split []*string           `json:"split"`
		} `json:"results"`
	} `json:"regs"`
}

func utf16Index(in []rune, runeIdx int) int { return len(utf16.Encode(in[:runeIdx])) }

// Every pattern the reference implementation uses must behave exactly like JavaScript's own RegExp.
func TestAgainstJavaScript(t *testing.T) {
	f, err := os.Open("testdata/golden.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := gzip.NewReader(f)
	var g goldenFile
	if err := json.NewDecoder(zr).Decode(&g); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, r := range g.Regs {
		re, err := Compile(r.Pattern, r.Flags)
		if err != nil {
			t.Errorf("/%s/%s (%s): %v", r.Pattern, r.Flags, r.File, err)
			continue
		}
		for idx, s := range g.Pool {
			want, has := r.Results[strconv.Itoa(idx)]
			in := []rune(s)
			ms := re.FindAll(s)
			var got [][]interface{}
			for _, m := range ms {
				groups := []interface{}{}
				for gi := 0; gi <= re.NumGroups(); gi++ {
					if m.Has(gi) {
						groups = append(groups, m.Str(gi))
					} else {
						groups = append(groups, nil)
					}
				}
				got = append(got, []interface{}{float64(utf16Index(in, m.Index())), groups})
			}
			if !has {
				if len(ms) != 0 {
					t.Errorf("/%s/%s on %q: Go matched, JS did not", r.Pattern, r.Flags, s)
				}
				continue
			}
			var wantM [][]interface{}
			for _, wm := range want.M {
				var idxv float64
				var groups []interface{}
				_ = json.Unmarshal(wm[0], &idxv)
				_ = json.Unmarshal(wm[1], &groups)
				wantM = append(wantM, []interface{}{idxv, groups})
			}
			if len(got) == 0 && len(wantM) == 0 {
				// fine
			} else if !reflect.DeepEqual(got, wantM) {
				t.Errorf("/%s/%s on %q:\n go: %v\n js: %v", r.Pattern, r.Flags, s, got, wantM)
				continue
			}
			rep := re.ReplaceStr(s, "<$&|$1|$2|$`|$'>")
			if rep != want.Rep {
				t.Errorf("/%s/%s replace on %q:\n go: %q\n js: %q", r.Pattern, r.Flags, s, rep, want.Rep)
			}
			noG := MustCompile(r.Pattern, stripG(r.Flags))
			sp := noG.Split(s)
			var wsp []string
			for _, p := range want.Split {
				if p == nil {
					wsp = append(wsp, "")
				} else {
					wsp = append(wsp, *p)
				}
			}
			if fmt.Sprint(sp) != fmt.Sprint(wsp) {
				t.Errorf("/%s/%s split on %q:\n go: %q\n js: %q", r.Pattern, r.Flags, s, sp, wsp)
			}
			checked++
		}
	}
	t.Logf("%d pattern/string pairs compared", checked)
}

func stripG(f string) string {
	out := ""
	for _, c := range f {
		if c != 'g' {
			out += string(c)
		}
	}
	return out
}

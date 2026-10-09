package core

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

type goldenParse struct {
	File  string         `json:"file"`
	Dirs  []string       `json:"dirs"`
	Cfg   map[string]any `json:"cfg"`
	Media map[string]any `json:"media"`
	Clean string         `json:"clean"`
}

func loadGolden(t testing.TB, name string, v any) {
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(z).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func numOrNA(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return NA
}

func mediaToMap(m *Media) map[string]any {
	r := map[string]any{}
	r["title"] = m.Title
	r["year"] = m.Year
	r["season"] = m.Season
	r["episode"] = m.Episode
	r["parts"] = m.Parts
	r["epTitle"] = m.EpTitle
	r["isEpisode"] = m.IsEpisode
	r["animeHint"] = m.AnimeHint
	r["code"] = m.Code
	r["epTitleBase"] = m.EpTitleBase
	r["codePrefix"] = m.CodePrefix
	r["label"] = m.Label
	r["display"] = m.Display
	r["ext"] = m.Ext
	r["index"] = m.Index
	r["fromFolder"] = m.FromFolder
	r["anime"] = m.Anime
	r["junk"] = m.Junk
	r["indexTitle"] = m.IndexTitle
	return r
}

// normalise a JS media object into the same shape as mediaToMap.
func normJS(j map[string]any) map[string]any {
	r := map[string]any{}
	str := func(k string) string { s, _ := j[k].(string); return s }
	boo := func(k string) bool { b, _ := j[k].(bool); return b }
	r["title"] = str("title")
	for _, k := range []string{"year", "season", "episode", "index"} {
		r[k] = numOrNA(j[k])
	}
	var parts []string
	if a, ok := j["parts"].([]any); ok {
		for _, p := range a {
			parts = append(parts, fmt.Sprint(p))
		}
	}
	r["parts"] = parts
	for _, k := range []string{"epTitle", "code", "epTitleBase", "codePrefix", "label", "display", "ext", "indexTitle"} {
		r[k] = str(k)
	}
	for _, k := range []string{"isEpisode", "animeHint", "fromFolder", "anime", "junk"} {
		r[k] = boo(k)
	}
	return r
}

func TestParseAgainstJS(t *testing.T) {
	var cases []goldenParse
	loadGolden(t, "parse.json.gz", &cases)
	bad := 0
	for _, c := range cases {
		cc := ConfigFrom(c.Cfg)
		cfg := &cc
		got := ParseMedia(c.File, cfg, c.Dirs)
		gm := mediaToMap(got)
		want := normJS(c.Media)
		if len(gm["parts"].([]string)) == 0 {
			gm["parts"] = []string(nil)
		}
		if p, _ := want["parts"].([]string); len(p) == 0 {
			want["parts"] = []string(nil)
		}
		if !reflect.DeepEqual(gm, want) {
			bad++
			if bad <= 25 {
				for k := range want {
					if !reflect.DeepEqual(gm[k], want[k]) {
						t.Errorf("%q dirs=%v cfg=%v: %s: got %#v want %#v", c.File, c.Dirs, c.Cfg, k, gm[k], want[k])
					}
				}
			}
		}
		if cn := CleanName(c.File, cfg); cn != c.Clean {
			bad++
			if bad <= 25 {
				t.Errorf("clean %q: got %q want %q", c.File, cn, c.Clean)
			}
		}
	}
	t.Logf("%d cases, %d mismatches", len(cases), bad)
	if bad > 0 {
		t.Fail()
	}
}

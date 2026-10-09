package store

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
)

type goldenStep struct {
	Patch  json.RawMessage `json:"patch"`
	Err    *string         `json:"err"`
	File   *string         `json:"file"`
	Values map[string]any  `json:"values"`
	App    App             `json:"app"`
}

// The recorded reference output carries a different default update source; that setting is checked on its own.
var reUpdateRepo = regexp.MustCompile(`"updateRepo": "[^"]*"`)

// openWindow is not part of the recorded output.
var reOpenWindow = regexp.MustCompile(`,\r?\n\s+"openWindow": (true|false)`)

func TestStoreAgainstJS(t *testing.T) {
	f, err := os.Open("testdata/store.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, _ := gzip.NewReader(f)
	var g struct {
		Cases []struct {
			Start json.RawMessage `json:"start"`
			Steps []goldenStep    `json:"steps"`
		} `json:"cases"`
	}
	if err := json.NewDecoder(z).Decode(&g); err != nil {
		t.Fatal(err)
	}
	for ci, c := range g.Cases {
		dir := t.TempDir()
		if string(c.Start) != "null" {
			var v any
			json.Unmarshal(c.Start, &v)
			// the JS wrote it with JSON.stringify(start, null, 2); do the same through jsonx to keep key order
			pv, _ := jsonx.Parse(string(c.Start))
			os.WriteFile(filepath.Join(dir, "config.json"), []byte(jsonx.Stringify(pv, 2)), 0o644)
		}
		st, err := New(dir)
		if err != nil {
			t.Fatal(err)
		}
		for si, s := range c.Steps {
			patch, perr := jsonx.Parse(string(s.Patch))
			if perr != nil {
				t.Fatal(perr)
			}
			uerr := st.Update(patch.(*jsonx.Obj))
			switch {
			case s.Err == nil && uerr != nil:
				t.Errorf("case %d step %d %s: unexpected error %v", ci, si, s.Patch, uerr)
			case s.Err != nil && uerr == nil:
				t.Errorf("case %d step %d %s: want error %q", ci, si, s.Patch, *s.Err)
			case s.Err != nil && uerr.Error() != *s.Err:
				t.Errorf("case %d step %d: error %q want %q", ci, si, uerr.Error(), *s.Err)
			}
			b, _ := os.ReadFile(st.File)
			if s.File == nil {
				if len(b) != 0 {
					t.Errorf("case %d step %d: file written but JS wrote none", ci, si)
				}
			} else if want := reUpdateRepo.ReplaceAllString(*s.File, `"updateRepo": "-"`); reUpdateRepo.ReplaceAllString(reOpenWindow.ReplaceAllString(string(b), ""), `"updateRepo": "-"`) != want {
				t.Errorf("case %d step %d %s: file differs\n got  %s\n want %s", ci, si, s.Patch, b, want)
			}
			wantApp := s.App
			wantApp.OpenWindow = true // not part of the recorded output
			wantApp.UpdateRepo = st.App().UpdateRepo // see reUpdateRepo
			if a := st.App(); !reflect.DeepEqual(a, wantApp) {
				t.Errorf("case %d step %d: app %+v want %+v", ci, si, a, wantApp)
			}
			got := st.Values()
			gb, _ := json.Marshal(got)
			var gv map[string]any
			json.Unmarshal(gb, &gv)
			if !reflect.DeepEqual(gv, s.Values) {
				for k, v := range s.Values {
					if !reflect.DeepEqual(gv[k], v) {
						t.Errorf("case %d step %d: value %s = %#v want %#v", ci, si, k, gv[k], v)
					}
				}
			}
		}
	}
}

func TestReset(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	patch, _ := jsonx.Parse(`{"settings":{"titleAsName":false},"app":{"autoStart":true,"startPresence":false}}`)
	if err := s.Update(patch.(*jsonx.Obj)); err != nil {
		t.Fatal(err)
	}
	if a := s.App(); !a.AutoStart || a.StartPresence {
		t.Fatalf("update not applied: %+v", a)
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if a := s.App(); a.AutoStart || !a.StartPresence || a.UpdateRepo != DefaultUpdateRepo {
		t.Fatalf("app not back to defaults: %+v", a)
	}
	if v := s.Values()["titleAsName"]; v != true {
		t.Fatalf("setting not back to default: %v", v)
	}
	if s.FirstRun() {
		t.Fatal("a reset must not bring the first-run welcome back")
	}
}

func TestOpenWindowSetting(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !s.App().OpenWindow {
		t.Fatal("the window should open by default")
	}
	patch, _ := jsonx.Parse(`{"app":{"openWindow":false}}`)
	if err := s.Update(patch.(*jsonx.Obj)); err != nil {
		t.Fatal(err)
	}
	if s.App().OpenWindow {
		t.Fatal("openWindow=false was not saved")
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if !s.App().OpenWindow {
		t.Fatal("a reset should bring the default back")
	}
}

func TestUpdateRepoSetting(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := s.App().UpdateRepo; got != DefaultUpdateRepo {
		t.Fatalf("default update source = %q", got)
	}
	ok, _ := jsonx.Parse(`{"app":{"updateRepo":"someone/else"}}`)
	if err := s.Update(ok.(*jsonx.Obj)); err != nil || s.App().UpdateRepo != "someone/else" {
		t.Fatalf("valid source not saved: %v %q", err, s.App().UpdateRepo)
	}
	bad, _ := jsonx.Parse(`{"app":{"updateRepo":"not a repo"}}`)
	if err := s.Update(bad.(*jsonx.Obj)); err == nil {
		t.Fatal("an invalid source must be refused")
	}
	if got := s.App().UpdateRepo; got != "someone/else" {
		t.Fatalf("a refused value must not change the saved one: %q", got)
	}
}
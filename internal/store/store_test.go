package store

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
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

// openWindow and welcomeSeen are not part of the recorded output.
var reOpenWindow = regexp.MustCompile(`,\r?\n\s+"(openWindow|welcomeSeen)": (true|false)`)

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
			wantApp.OpenWindow = true                // not part of the recorded output
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

func TestSourcesFieldClean(t *testing.T) {
	f, ok := ByKey("disabledSources")
	if !ok || f.Type != "sources" || len(f.Sources) == 0 {
		t.Fatalf("disabledSources should be a sources field with the service list: %+v", f)
	}
	// duplicates collapse and the catalog's order is kept
	got, err := Clean(f, []any{"wikipedia", "imdb", "wikipedia", "tmdb"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []any{"tmdb", "imdb", "wikipedia"}) {
		t.Errorf("got %#v", got)
	}
	// nothing switched off
	if got, err := Clean(f, nil); err != nil || !reflect.DeepEqual(got, []any{}) {
		t.Errorf("nil should mean an empty list, got %#v %v", got, err)
	}
	if got, err := Clean(f, []any{}); err != nil || !reflect.DeepEqual(got, []any{}) {
		t.Errorf("empty list: %#v %v", got, err)
	}
	// unknown ids and wrong shapes are refused
	if _, err := Clean(f, []any{"imdb", "no-such-service"}); err == nil {
		t.Error("an unknown service must be refused")
	}
	if _, err := Clean(f, "imdb"); err == nil {
		t.Error("a bare string is not a list")
	}
	if _, err := Clean(f, []any{42.0}); err == nil {
		t.Error("a number is not a service")
	}
}

func TestDisabledSourcesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Values()["disabledSources"]; !reflect.DeepEqual(got, []any{}) {
		t.Fatalf("default should be an empty list, got %#v", got)
	}
	patch, _ := jsonx.Parse(`{"settings":{"disabledSources":["tvmaze","anilist"],"pauseClearMinutes":45}}`)
	if err := s.Update(patch.(*jsonx.Obj)); err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	if cfg.SourceOn("tvmaze") || cfg.SourceOn("anilist") || !cfg.SourceOn("imdb") {
		t.Errorf("config does not reflect the switches: %v", cfg.DisabledSources)
	}
	if cfg.PauseClearMinutes != 45 {
		t.Errorf("pauseClearMinutes = %d", cfg.PauseClearMinutes)
	}
	// a fresh store reading the same folder sees the same thing, and the window gets the list back
	s2, _ := New(dir)
	if got := s2.Values()["disabledSources"]; !reflect.DeepEqual(got, []any{"tvmaze", "anilist"}) {
		t.Errorf("after reload: %#v", got)
	}
	if got := s2.Values()["pauseClearMinutes"]; got != float64(45) {
		t.Errorf("after reload pauseClearMinutes = %#v", got)
	}
	// Restore defaults brings everything back
	if err := s2.Reset(); err != nil {
		t.Fatal(err)
	}
	if c := s2.Config(); !c.SourceOn("tvmaze") || c.PauseClearMinutes != 30 {
		t.Errorf("reset should restore the defaults: %v %d", c.DisabledSources, c.PauseClearMinutes)
	}
}

func TestPauseClearMinutesValidation(t *testing.T) {
	f, ok := ByKey("pauseClearMinutes")
	if !ok {
		t.Fatal("pauseClearMinutes missing from the schema")
	}
	for in, want := range map[any]int{0.0: 0, 5.0: 5, "30": 30, 1440.0: 1440, 12.4: 12} {
		got, err := Clean(f, in)
		if err != nil || got != want {
			t.Errorf("Clean(%v) = %v, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []any{-1.0, 1441.0, "abc", "-5"} {
		if _, err := Clean(f, in); err == nil {
			t.Errorf("Clean(%v) should be refused", in)
		}
	}
}

func TestPrivacySectionLayout(t *testing.T) {
	var ids []string
	for _, s := range Sections {
		ids = append(ids, s.ID)
	}
	want := []string{"look", "titles", "privacy", "advanced"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("sections = %v, want %v", ids, want)
	}
	// showArtwork lives on the privacy tab only, once
	n := 0
	for _, s := range Sections {
		for _, f := range s.Fields {
			if f.Key == "showArtwork" {
				n++
				if s.ID != "privacy" {
					t.Errorf("showArtwork should be in the privacy section, found in %s", s.ID)
				}
			}
		}
	}
	if n != 1 {
		t.Errorf("showArtwork appears %d times", n)
	}
}

func TestLinesFieldClean(t *testing.T) {
	f, ok := ByKey("hideFiles")
	if !ok || f.Type != "lines" {
		t.Fatalf("hideFiles should be a lines field: %+v", f)
	}
	// text from the window: trimmed, blanks and duplicates dropped, order kept
	got, err := Clean(f, "  Private \r\n\r\n*.xyz\nPrivate\n   \nD:\\Videos\\Secret")
	if err != nil || !reflect.DeepEqual(got, []any{"Private", "*.xyz", `D:\Videos\Secret`}) {
		t.Errorf("from text: %#v %v", got, err)
	}
	// an array works too, and nothing means an empty list
	if got, err := Clean(f, []any{"a", " b ", "a"}); err != nil || !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Errorf("from array: %#v %v", got, err)
	}
	if got, err := Clean(f, nil); err != nil || !reflect.DeepEqual(got, []any{}) {
		t.Errorf("nil: %#v %v", got, err)
	}
	// limits and wrong shapes are refused
	long := strings.Repeat("x", maxLineLen+1)
	if _, err := Clean(f, long); err == nil {
		t.Error("an over-long entry must be refused")
	}
	var many []string
	for i := 0; i <= maxLines; i++ {
		many = append(many, fmt.Sprintf("entry %d", i))
	}
	if _, err := Clean(f, strings.Join(many, "\n")); err == nil {
		t.Error("too many entries must be refused")
	}
	if _, err := Clean(f, []any{"ok", 5.0}); err == nil {
		t.Error("a number is not an entry")
	}
	if _, err := Clean(f, true); err == nil {
		t.Error("a bool is not a list")
	}
}

func TestPrivacySettingsRoundTripAndHeadings(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	vals := s.Values()
	if _, has := vals[""]; has {
		t.Error("headings carry no value")
	}
	if vals["hideTitle"] != false || !reflect.DeepEqual(vals["hideFiles"], []any{}) {
		t.Errorf("defaults: hideTitle=%v hideFiles=%#v", vals["hideTitle"], vals["hideFiles"])
	}
	patch, _ := jsonx.Parse(`{"settings":{"hideTitle":true,"hideFiles":["Private","*.xyz"]}}`)
	if err := s.Update(patch.(*jsonx.Obj)); err != nil {
		t.Fatal(err)
	}
	c := New2(t, dir).Config()
	if !c.HideTitle || !reflect.DeepEqual(c.HideFiles, []string{"Private", "*.xyz"}) {
		t.Errorf("after reload: %+v %v", c.HideTitle, c.HideFiles)
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if c := s.Config(); c.HideTitle || len(c.HideFiles) != 0 {
		t.Errorf("reset: %v %v", c.HideTitle, c.HideFiles)
	}
	// every heading is a pure label: no key, never a field a client can set
	for _, sec := range Sections {
		for _, f := range sec.Fields {
			if (f.Type == "heading") != (f.Key == "") {
				t.Errorf("field %+v: headings (and only headings) have no key", f)
			}
		}
	}
	// the window cannot save a heading
	bad, _ := jsonx.Parse(`{"settings":{"":"x"}}`)
	if err := s.Update(bad.(*jsonx.Obj)); err != nil {
		t.Errorf("an unknown key is ignored, got %v", err)
	}
}

// New2 reopens the store in the same folder (a fresh instance reading the saved file).
func New2(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWelcomeSeenSetting(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.App().WelcomeSeen {
		t.Fatal("a new install has not seen the welcome yet")
	}
	patch, _ := jsonx.Parse(`{"app":{"welcomeSeen":true}}`)
	if err := s.Update(patch.(*jsonx.Obj)); err != nil {
		t.Fatal(err)
	}
	if !s.App().WelcomeSeen {
		t.Fatal("welcomeSeen was not saved")
	}
	// the other program settings are untouched by it
	if a := s.App(); !a.StartPresence || !a.OpenWindow || !a.CheckUpdates || a.AutoStart {
		t.Errorf("other app settings changed: %+v", a)
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if s.App().WelcomeSeen {
		t.Fatal("Restore defaults brings the welcome back")
	}
}

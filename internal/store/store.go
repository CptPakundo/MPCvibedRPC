package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

// DataDir is where everything lives:
//
//	Windows: %LOCALAPPDATA%\MPCvibedRPC      (override with MPCRPC_HOME)
//	macOS:   ~/Library/Application Support/MPCvibedRPC
//	Linux:   $XDG_DATA_HOME/MPCvibedRPC, else ~/.local/share/MPCvibedRPC
//	config.json  settings    mpcvibedrpc.log  log    artwork-cache.json  cover cache
//	stats.json   local counts    ipc.json  the running copy's port and token    instance.lock  single instance (macOS, Linux)
func DataDir() string {
	if v := os.Getenv("MPCRPC_HOME"); v != "" {
		return v
	}
	const name = "MPCvibedRPC"
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, name)
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", name)
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, name)
}

// App holds the settings that belong to the program itself rather than the presence.
type App struct {
	AutoStart     bool   `json:"autoStart"`     // run at login (the registry value, login item or autostart entry is the source of truth; this mirrors it)
	StartPresence bool   `json:"startPresence"` // begin sending presence as soon as the app opens
	CheckUpdates  bool   `json:"checkUpdates"`  // look for a newer release at start, then daily
	AutoUpdate    bool   `json:"autoUpdate"`    // install a found update by itself at a quiet moment (needs checkUpdates; Windows and Linux)
	UpdateRepo    string `json:"updateRepo"`    // "owner/repo" on GitHub that publishes releases with MPCvibedRPC.exe
	WelcomeSeen   bool   `json:"welcomeSeen"`   // the first-run welcome card was dismissed (existing installs count as having seen it)
	OpenWindow    bool   `json:"openWindow"`    // show the window when the program is started by hand (it always starts in the tray at login)
	SeenVersion   string `json:"seenVersion"`   // the version whose changelog was last shown ("" = none yet: a new install, or one from before changelogs)
}

// DefaultUpdateRepo is where releases of this program are published.
const DefaultUpdateRepo = "CptPakundo/MPCvibedRPC"

var appKeys = []string{"autoStart", "startPresence", "checkUpdates", "autoUpdate", "updateRepo", "openWindow", "welcomeSeen", "seenVersion"}

// defaultApp is the program's own settings before anything is saved.
var defaultApp = App{StartPresence: true, CheckUpdates: true, AutoUpdate: true, UpdateRepo: DefaultUpdateRepo, OpenWindow: true}

// appDefaults is defaultApp as a config.json block (seenVersion is left out: it is written only once a changelog was
// shown).
func appDefaults() *jsonx.Obj {
	o := jsonx.NewObj()
	o.Set("autoStart", defaultApp.AutoStart)
	o.Set("startPresence", defaultApp.StartPresence)
	o.Set("checkUpdates", defaultApp.CheckUpdates)
	o.Set("autoUpdate", defaultApp.AutoUpdate)
	o.Set("updateRepo", defaultApp.UpdateRepo)
	o.Set("openWindow", defaultApp.OpenWindow)
	o.Set("welcomeSeen", defaultApp.WelcomeSeen)
	return o
}

// boolOr reads a boolean setting from the app block, or def when it is missing or not a boolean.
func boolOr(o *jsonx.Obj, key string, def bool) bool {
	if o != nil {
		if b, ok := o.M[key].(bool); ok {
			return b
		}
	}
	return def
}

// strOr reads a text setting from the app block, or def when it is missing or not text.
func strOr(o *jsonx.Obj, key, def string) string {
	if o != nil {
		if s, ok := o.M[key].(string); ok {
			return s
		}
	}
	return def
}

// Store reads and writes config.json.
type Store struct {
	Dir  string
	File string

	mu     sync.Mutex // guards config.json: reads and changes are serialised (the window and a sign-in can save at the same moment)
	broken []byte     // the unreadable config.json last kept as a .broken copy (it is kept once, not at every read)
}

// New opens (and creates) the data folder.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{Dir: dir, File: filepath.Join(dir, "config.json")}, nil
}

func (s *Store) CachePath() string { return filepath.Join(s.Dir, "artwork-cache.json") }
func (s *Store) LogPath() string   { return filepath.Join(s.Dir, "mpcvibedrpc.log") }
func (s *Store) IPCPath() string   { return filepath.Join(s.Dir, "ipc.json") }
func (s *Store) StatsPath() string { return filepath.Join(s.Dir, "stats.json") }

// FirstRun is true when config.json does not exist yet.
func (s *Store) FirstRun() bool {
	_, err := os.Stat(s.File)
	return err != nil
}

// read parses config.json. The caller holds s.mu.
func (s *Store) read() *jsonx.Obj {
	b, err := os.ReadFile(s.File)
	if err != nil {
		return jsonx.NewObj()
	}
	txt := strings.TrimPrefix(string(b), "\ufeff")
	v, perr := jsonx.Parse(txt)
	o, ok := v.(*jsonx.Obj)
	if perr != nil {
		if !bytes.Equal(b, s.broken) {
			_ = os.WriteFile(s.brokenName(), b, 0o600)
			s.broken = b
		}
		return jsonx.NewObj()
	}
	if !ok {
		return jsonx.NewObj()
	}
	return o
}

// brokenName is where an unreadable config.json is kept: config.json.broken, or a numbered name when one exists.
func (s *Store) brokenName() string {
	name := s.File + ".broken"
	for i := 2; i < 1000; i++ {
		if _, err := os.Stat(name); err != nil {
			break
		}
		name = s.File + ".broken" + strconv.Itoa(i)
	}
	return name
}

// write saves config.json. The caller holds s.mu.
func (s *Store) write(o *jsonx.Obj) error {
	tmp := s.File + ".tmp"
	if err := os.WriteFile(tmp, []byte(jsonx.Stringify(o, 2)), 0o600); err != nil {
		return err
	}
	// Atomic: a crash never leaves half a file. On Windows a scanner or a reader can hold the file for a moment
	// ("access denied"), so the rename is tried a few times.
	var err error
	for i := 0; i < 5; i++ {
		if err = os.Rename(tmp, s.File); err == nil {
			return nil
		}
		if runtime.GOOS != "windows" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = os.Remove(tmp)
	return err
}

// Reset puts every setting, including the program's own, back to its default (the welcome card shows again). Which
// version's news was last shown is kept: it is not a setting, and the same news must not pop up again.
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh := jsonx.NewObj()
	if cur, ok := s.read().M["app"].(*jsonx.Obj); ok {
		keep := jsonx.NewObj()
		for _, k := range []string{"seenVersion"} {
			if v, ok := cur.M[k]; ok {
				keep.Set(k, v)
			}
		}
		if len(keep.M) > 0 {
			fresh.Set("app", keep)
		}
	}
	return s.write(fresh)
}

// SettingsObj is the file without the app block.
func (s *Store) SettingsObj() *jsonx.Obj {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settingsObj()
}

func (s *Store) settingsObj() *jsonx.Obj {
	o := s.read()
	out := jsonx.NewObj()
	for _, k := range o.Keys {
		if k != "app" {
			out.Set(k, o.M[k])
		}
	}
	return out
}

// Config is the presence settings with defaults filled in.
func (s *Store) Config() core.Config {
	return core.LoadConfig([]byte(jsonx.Stringify(s.SettingsObj(), 0)))
}

// App returns the program's own settings.
func (s *Store) App() App {
	s.mu.Lock()
	o := s.read()
	s.mu.Unlock()
	cur, _ := o.M["app"].(*jsonx.Obj)
	d := defaultApp
	return App{
		AutoStart:     boolOr(cur, "autoStart", d.AutoStart),
		StartPresence: boolOr(cur, "startPresence", d.StartPresence),
		CheckUpdates:  boolOr(cur, "checkUpdates", d.CheckUpdates),
		AutoUpdate:    boolOr(cur, "autoUpdate", d.AutoUpdate),
		UpdateRepo:    strOr(cur, "updateRepo", d.UpdateRepo),
		WelcomeSeen:   boolOr(cur, "welcomeSeen", d.WelcomeSeen),
		OpenWindow:    boolOr(cur, "openWindow", d.OpenWindow),
		SeenVersion:   strOr(cur, "seenVersion", d.SeenVersion),
	}
}

// Values are the settings as the window shows them (maps as "name = value" text).
func (s *Store) Values() map[string]any {
	raw := s.SettingsObj()
	cfg := core.LoadConfig([]byte(jsonx.Stringify(raw, 0)))
	out := map[string]any{}
	asMap := core.ConfigAsMap(&cfg)
	for _, f := range AllFields() {
		if f.Type == "heading" {
			continue
		}
		if f.Type == "map" {
			if o, ok := raw.M[f.Key].(*jsonx.Obj); ok {
				out[f.Key] = MapToText(o)
			} else {
				out[f.Key] = ""
			}
			continue
		}
		out[f.Key] = asMap[f.Key]
	}
	return out
}

// Update validates and saves a partial update from the window: { settings: {...}, app: {...} }.
func (s *Store) Update(patch *jsonx.Obj) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.read()
	next := jsonx.NewObj()
	for _, k := range cur.Keys {
		next.Set(k, cur.M[k])
	}
	var errs []string
	if ps, ok := patch.M["settings"].(*jsonx.Obj); ok {
		for _, k := range ps.Keys {
			f, known := ByKey(k)
			if !known {
				continue
			}
			v, err := Clean(f, ps.M[k])
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			next.Set(k, v)
		}
	}
	if len(errs) > 0 {
		return &ValidationError{strings.Join(errs, "\n")}
	}
	if pa, ok := patch.M["app"].(*jsonx.Obj); ok {
		app := appDefaults()
		if ca, ok := cur.M["app"].(*jsonx.Obj); ok {
			for _, k := range ca.Keys {
				app.Set(k, ca.M[k])
			}
		}
		for _, k := range appKeys {
			v, present := pa.M[k]
			if !present {
				continue
			}
			if k == "updateRepo" {
				repo := jsTrim(jsString(orEmpty(v)))
				if repo != "" && !reRepo.Test(repo) {
					return &ValidationError{"Update source must look like owner/repo"}
				}
				app.Set(k, repo)
			} else if k == "seenVersion" {
				app.Set(k, jsTrim(jsString(orEmpty(v)))) // not in appDefaults: written only once a changelog was shown
			} else {
				b, _ := v.(bool)
				if str, ok := v.(string); ok {
					b = str == "true"
				}
				app.Set(k, b)
			}
		}
		next.Set("app", app)
	}
	return s.write(next)
}

// SetSettings validates and saves a patch of window settings given as plain values (for example
// {"hideTitle": true}), through the same validation as Update. The values go through JSON first, so they arrive
// the way the window sends them (an int becomes a JSON number).
func (s *Store) SetSettings(values map[string]any) error {
	b, err := json.Marshal(map[string]any{"settings": values})
	if err != nil {
		return err
	}
	patch, err := jsonx.Parse(string(b))
	if err != nil {
		return err
	}
	obj, ok := patch.(*jsonx.Obj)
	if !ok {
		return errors.New("bad settings patch")
	}
	return s.Update(obj)
}

// SetHidden saves settings that are not typed in the window (the sign-ins), in the order given; a nil value
// removes the setting. Only keys of core.Config are taken.
func (s *Store) SetHidden(keys []string, values map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	known := core.ConfigAsMap(&core.Config{})
	cur := s.read()
	for _, k := range keys {
		if _, ok := known[k]; !ok {
			continue
		}
		if v := values[k]; v == nil {
			cur.Delete(k)
		} else {
			cur.Set(k, v)
		}
	}
	return s.write(cur)
}

var reRepo = jsre.MustCompile(`^[\w.-]+\/[\w.-]+$`, "")

func orEmpty(v any) any {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if !x {
			return ""
		}
	case float64:
		if x == 0 {
			return ""
		}
	}
	return v
}

package store

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

// DataDir is where everything lives:
//
//	Windows: %LOCALAPPDATA%\MPCvibedRPC      (override with MPCRPC_HOME)
//	config.json  settings    mpcvibedrpc.log  log    artwork-cache.json  cover cache
func DataDir() string {
	if v := os.Getenv("MPCRPC_HOME"); v != "" {
		return v
	}
	return dataDirNamed("MPCvibedRPC")
}

func dataDirNamed(name string) string {
	home, _ := os.UserHomeDir()
	if isWindows {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, name)
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, name)
}

// App holds the settings that belong to the program itself rather than the presence.
type App struct {
	AutoStart     bool   `json:"autoStart"`     // run at Windows login (the registry entry is the source of truth; this mirrors it)
	StartPresence bool   `json:"startPresence"` // begin sending presence as soon as the app opens
	CheckUpdates  bool   `json:"checkUpdates"`  // look for a newer release now and then
	UpdateRepo    string `json:"updateRepo"`    // "owner/repo" on GitHub that publishes releases with MPCvibedRPC.exe
	OpenWindow    bool   `json:"openWindow"`    // show the window when the program is started by hand (it always starts in the tray at login)
}

// DefaultUpdateRepo is where releases of this program are published.
const DefaultUpdateRepo = "CptPakundo/MPCvibedRPC"

var appKeys = []string{"autoStart", "startPresence", "checkUpdates", "updateRepo", "openWindow"}

func appDefaults() *jsonx.Obj {
	o := jsonx.NewObj()
	o.Set("autoStart", false)
	o.Set("startPresence", true)
	o.Set("checkUpdates", true)
	o.Set("updateRepo", DefaultUpdateRepo)
	o.Set("openWindow", true)
	return o
}

// Store reads and writes config.json.
type Store struct {
	Dir  string
	File string
}

// New opens (and creates) the data folder.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir, File: filepath.Join(dir, "config.json")}, nil
}

func (s *Store) CachePath() string { return filepath.Join(s.Dir, "artwork-cache.json") }
func (s *Store) LogPath() string   { return filepath.Join(s.Dir, "mpcvibedrpc.log") }
func (s *Store) IPCPath() string   { return filepath.Join(s.Dir, "ipc.json") }

// FirstRun is true when config.json does not exist yet.
func (s *Store) FirstRun() bool {
	_, err := os.Stat(s.File)
	return err != nil
}

func (s *Store) read() *jsonx.Obj {
	b, err := os.ReadFile(s.File)
	if err != nil {
		return jsonx.NewObj()
	}
	txt := strings.TrimPrefix(string(b), "\ufeff")
	v, perr := jsonx.Parse(txt)
	o, ok := v.(*jsonx.Obj)
	if perr != nil {
		_ = os.WriteFile(s.File+".broken", b, 0o644)
		return jsonx.NewObj()
	}
	if !ok {
		return jsonx.NewObj()
	}
	return o
}

func (s *Store) write(o *jsonx.Obj) error {
	tmp := s.File + ".tmp"
	if err := os.WriteFile(tmp, []byte(jsonx.Stringify(o, 2)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.File) // atomic: a crash never leaves half a file
}

// Reset puts every setting, including the program's own, back to its default.
func (s *Store) Reset() error {
	return s.write(jsonx.NewObj())
}

// SettingsObj is the file without the app block.
func (s *Store) SettingsObj() *jsonx.Obj {
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
	o := s.read()
	cur, _ := o.M["app"].(*jsonx.Obj)
	a := App{AutoStart: false, StartPresence: true, CheckUpdates: true, UpdateRepo: DefaultUpdateRepo, OpenWindow: true}
	if cur != nil {
		if b, ok := cur.M["autoStart"].(bool); ok {
			a.AutoStart = b
		}
		if b, ok := cur.M["startPresence"].(bool); ok {
			a.StartPresence = b
		}
		if b, ok := cur.M["checkUpdates"].(bool); ok {
			a.CheckUpdates = b
		}
		if b, ok := cur.M["openWindow"].(bool); ok {
			a.OpenWindow = b
		}
		if v, ok := cur.M["updateRepo"].(string); ok {
			a.UpdateRepo = v
		}
	}
	return a
}

// Values are the settings as the window shows them (maps as "name = value" text).
func (s *Store) Values() map[string]any {
	cfg := s.Config()
	raw := s.SettingsObj()
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


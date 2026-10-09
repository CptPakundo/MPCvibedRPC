package winsys

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// usesProfile reports whether a browser command line was started with --user-data-dir pointing at exactly this
// folder. The app starts its settings window with a profile of its own, so this identifies that window's browser
// (and all of its helper processes) and nothing else. Paths compare without regard to case, as on Windows.
func usesProfile(cmdline, profile string) bool {
	if cmdline == "" || profile == "" {
		return false
	}
	profile = strings.TrimRight(profile, `\/`)
	cl := strings.ToLower(cmdline)
	flag := "--user-data-dir=" + strings.ToLower(profile)
	for from := 0; ; {
		i := strings.Index(cl[from:], flag)
		if i < 0 {
			return false
		}
		end := from + i + len(flag)
		// the folder must end here: at a quote, a space, a path separator or the end of the line
		// (so ...\window does not match ...\window2, but does match ...\window\ or "...\window")
		if end >= len(cl) || strings.ContainsRune(`" `, rune(cl[end])) || cl[end] == '\\' || cl[end] == '/' {
			return true
		}
		from = end
	}
}

// prepareProfile makes sure the window's browser profile has Edge's "startup boost" and background mode switched
// off. Without that, closing the window makes Edge relaunch itself in the background (a browser and its helper
// processes that nobody asked for). The settings live in the profile's "Local State" file; everything else in it is
// kept as it is, and nothing is written when the settings are already right.
func prepareProfile(dir string) {
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	file := filepath.Join(dir, "Local State")
	state := map[string]any{}
	if raw, err := os.ReadFile(file); err == nil {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber() // keep large numbers exactly as Edge wrote them
		if dec.Decode(&state) != nil || state == nil {
			state = map[string]any{}
		}
	}
	changed := false
	for _, key := range []string{"startup_boost", "background_mode"} {
		m, _ := state[key].(map[string]any)
		if m == nil {
			m = map[string]any{}
			state[key] = m
		}
		if v, ok := m["enabled"].(bool); !ok || v {
			m["enabled"] = false
			changed = true
		}
	}
	if !changed {
		return
	}
	if out, err := json.Marshal(state); err == nil {
		_ = os.WriteFile(file, out, 0o644)
	}
}

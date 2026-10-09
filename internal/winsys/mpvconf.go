package winsys

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
)

const mpvIPCKey = "input-ipc-server"

// MpvConfValue finds a top-level option in mpv.conf text (before the first [profile]); commented lines are skipped.
func MpvConfValue(text, key string) (string, bool) {
	for _, l := range strings.Split(strings.TrimPrefix(text, "\ufeff"), "\n") {
		l = strings.TrimSpace(strings.TrimSuffix(l, "\r"))
		if strings.HasPrefix(l, "[") {
			break // profiles start here: options after it only apply to that profile
		}
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if ok && strings.TrimSpace(strings.TrimPrefix(k, "--")) == key {
			v = strings.TrimSpace(v)
			if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
				v = v[1 : len(v)-1]
			}
			return v, true
		}
	}
	return "", false
}

// AddMpvConfLine puts key=value at the top of mpv.conf text, keeping a byte-order mark and the line endings.
func AddMpvConfLine(text, key, value string) string {
	bom := ""
	if strings.HasPrefix(text, "\ufeff") {
		bom, text = "\ufeff", text[len("\ufeff"):]
	}
	eol := "\n"
	if strings.Contains(text, "\r\n") || (text == "" && IsWindows) {
		eol = "\r\n"
	}
	return bom + "# added by MPCvibedRPC, so it can see what mpv plays" + eol + key + "=" + value + eol + text
}

// mpvConfPaths lists where mpv reads its settings, most specific first: portable_config next to a known mpv.exe,
// then the user's mpv folder. present tells whether mpv seems to be on this PC at all.
func mpvConfPaths(exeDirs []string) (paths []string, present bool) {
	if !IsWindows {
		home, _ := os.UserHomeDir()
		return mpvConfPathsUnix(home)
	}
	if p, err := exec.LookPath("mpv"); err == nil {
		exeDirs = append(exeDirs, filepath.Dir(p))
	}
	for _, d := range exeDirs {
		present = true
		if st, err := os.Stat(filepath.Join(d, "portable_config")); err == nil && st.IsDir() {
			paths = append(paths, filepath.Join(d, "portable_config", "mpv.conf"))
		}
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		dir := filepath.Join(ad, "mpv")
		if _, err := os.Stat(dir); err == nil {
			present = true
		}
		paths = append(paths, filepath.Join(dir, "mpv.conf"))
	}
	return paths, present
}

// mpvResult is what setting up mpv did.
type mpvResult struct {
	Message string // empty when mpv is not on this PC
	Using   string // the IPC name mpv will use
	Edited  string // the file that was changed
	OK      bool
}

// enableMpvIPC makes mpv listen for us by adding input-ipc-server to the mpv.conf it reads. A name the user already
// set is kept (and reported in Using). mpv is not closed: it reads mpv.conf when it starts.
func enableMpvIPC(name string, exeDirs []string) mpvResult {
	paths, present := mpvConfPaths(exeDirs)
	if !present || len(paths) == 0 || name == "" {
		return mpvResult{}
	}
	file := paths[0]
	b, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return mpvResult{Message: "mpv: could not read " + file + "."}
	}
	if v, ok := MpvConfValue(string(b), mpvIPCKey); ok && v != "" {
		return mpvResult{Message: "mpv was already set up (" + mpvIPCKey + "=" + v + ").", Using: v, OK: true}
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return mpvResult{Message: "mpv: could not create " + filepath.Dir(file) + "."}
	}
	value := name
	if !IsWindows {
		value = pipe.Path(name) // a bare name would be a file in whatever folder mpv was started from
	}
	if err := os.WriteFile(file, []byte(AddMpvConfLine(string(b), mpvIPCKey, value)), 0o644); err != nil {
		return mpvResult{Message: "mpv: could not write " + file + "."}
	}
	return mpvResult{Message: "mpv set up (mpv.conf); it takes effect the next time mpv starts.", Using: name, Edited: file, OK: true}
}

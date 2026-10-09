package winsys

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The macOS and Linux versions of the parts above. They live in an ordinary file (not one for a single system) so that
// their logic is tested everywhere; only the helper programs they start are missing on Windows.

// unixPlayers are the players looked for on macOS and Linux, besides mpv, VLC and MPC-QT (diagnostics only).
var unixPlayers = []player{
	{name: "IINA", procs: []string{"iina"}},
	{name: "Celluloid", procs: []string{"celluloid"}},
	{name: "Haruna", procs: []string{"haruna"}},
	{name: "SMPlayer", procs: []string{"smplayer"}},
	{name: "GNOME Videos", procs: []string{"totem"}},
	{name: "Showtime", procs: []string{"showtime"}},
	{name: "Clapper", procs: []string{"clapper"}},
}

// unixRunningExes lists the running programs of the given players, as ps names them (a full path on macOS, the
// program's name on Linux).
func unixRunningExes(list []player) []string {
	r := run("ps", "-axo", "comm=")
	return matchExes(r.stdout, list)
}

// matchExes picks the lines of a process list that belong to one of the players.
func matchExes(psOut string, list []player) []string {
	var out []string
	for _, l := range strings.Split(psOut, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		for _, p := range list {
			if p.belongs(l) {
				out = append(out, l)
				break
			}
		}
	}
	return out
}

// unixSystemInfo names the system without user or machine names: "macOS 15.1 (arm64)", "Ubuntu 24.04.1 LTS (amd64)".
func unixSystemInfo() string {
	if runtime.GOOS == "darwin" {
		if v := strings.TrimSpace(run("sw_vers", "-productVersion").stdout); v != "" {
			return "macOS " + v + " (" + runtime.GOARCH + ")"
		}
	}
	if f, err := os.Open("/etc/os-release"); err == nil {
		defer f.Close()
		if name := osReleaseName(bufio.NewScanner(f)); name != "" {
			return name + " (" + runtime.GOARCH + ")"
		}
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

// osReleaseName reads PRETTY_NAME from os-release.
func osReleaseName(sc *bufio.Scanner) string {
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// findBrowserUnix finds a browser that can show the settings as an app window (Chrome, Edge, Brave, Vivaldi,
// Chromium). Firefox cannot, and a Snap-packaged browser may not use a profile in a hidden folder, so those are left
// to the default browser.
func findBrowserUnix() string {
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		for _, app := range []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Vivaldi", "Chromium"} {
			for _, base := range []string{"/Applications", filepath.Join(home, "Applications")} {
				p := filepath.Join(base, app+".app", "Contents", "MacOS", app)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
		return ""
	}
	for _, name := range []string{"google-chrome-stable", "google-chrome", "microsoft-edge-stable", "microsoft-edge",
		"brave-browser", "vivaldi-stable", "vivaldi", "chromium", "chromium-browser"} {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		if strings.HasPrefix(p, "/snap/") {
			continue
		}
		return p
	}
	return ""
}

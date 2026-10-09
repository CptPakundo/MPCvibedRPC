// Package winsys is everything that touches Windows itself: run-at-login, the players' web-interface switch, the
// settings window and the tray icon. Each function does nothing
// harmful elsewhere, so the rest of the app can be developed and tested on any OS.
package winsys

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/proc"
)

// IsWindows is true on Windows.
const IsWindows = runtime.GOOS == "windows"

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

type result struct {
	ok     bool
	stdout string
}

// run executes a helper command without a console window, giving up after 15 seconds.
func run(file string, args ...string) result {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := proc.Hide(exec.CommandContext(ctx, file, args...))
	var out bytes.Buffer
	c.Stdout = &out
	err := c.Run()
	return result{ok: err == nil, stdout: out.String()}
}

// ---- run at login ---------------------------------------------------------------

// GetAutoStart reports whether the registry's Run entry points at this program.
func GetAutoStart(exe string) bool {
	if !IsWindows {
		return false
	}
	r := run("reg", "query", runKey, "/v", runName())
	return r.ok && strings.Contains(strings.ToLower(r.stdout), strings.ToLower(filepath.Base(exe)))
}

// SetAutoStart adds or removes the registry entry.
func SetAutoStart(enabled bool, exe string) bool {
	if !IsWindows {
		return false
	}
	if enabled {
		return run("reg", "add", runKey, "/v", runName(), "/t", "REG_SZ", "/d", `"`+exe+`" --background`, "/f").ok
	}
	run("reg", "delete", runKey, "/v", runName(), "/f")
	return true
}

// ---- the players' web interface ---------------------------------------------------

// WebResult is the answer of EnableMpcWebInterface.
type WebResult struct {
	OK         bool     `json:"ok"`
	NeedsClose bool     `json:"needsClose,omitempty"`
	Message    string   `json:"message"`
	Edited     []string `json:"edited,omitempty"`
}

// player describes where one supported player keeps its web-interface switch.
type player struct {
	name        string
	procs       []string // process names without .exe
	regKey      string   // registry key that holds the two values
	section     string   // the same two values inside the player's .ini file
	enableKey   string
	portKey     string
	iniNames    []string
	installDirs []string // under Program Files
	userIniDir  string   // under %APPDATA%, where a player may keep its .ini
	always      bool     // switch the registry values on even when no install is found
}

var players = []player{
	{
		name: "MPC-HC", procs: []string{"mpc-hc", "mpc-hc64"},
		regKey: `HKCU\Software\MPC-HC\MPC-HC\Settings`, section: "Settings", enableKey: "EnableWebServer", portKey: "WebServerPort",
		iniNames:    []string{"mpc-hc.ini", "mpc-hc64.ini"},
		installDirs: []string{"MPC-HC", `K-Lite Codec Pack\MPC-HC64`, `K-Lite Codec Pack\MPC-HC`},
		always:      true,
	},
	{
		name: "MPC-BE", procs: []string{"mpc-be", "mpc-be64"},
		regKey: `HKCU\Software\MPC-BE\WebServer`, section: "WebServer", enableKey: "EnableWebServer", portKey: "Port",
		iniNames:    []string{"mpc-be.ini", "mpc-be64.ini"},
		installDirs: []string{"MPC-BE", "MPC-BE x64"},
		userIniDir:  "MPC-BE",
	},
}

func (p player) iniCandidates(extraDirs []string) []string {
	var dirs []string
	dirs = append(dirs, extraDirs...)
	for _, pf := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if pf != "" {
			for _, d := range p.installDirs {
				dirs = append(dirs, filepath.Join(pf, d))
			}
		}
	}
	if ad := os.Getenv("APPDATA"); ad != "" && p.userIniDir != "" {
		dirs = append(dirs, filepath.Join(ad, p.userIniDir))
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range dirs {
		if seen[d] {
			continue
		}
		seen[d] = true
		for _, n := range p.iniNames {
			f := filepath.Join(d, n)
			if _, err := os.Stat(f); err == nil {
				out = append(out, f)
			}
		}
	}
	return out
}

// installed reports whether the player seems to be on this PC (it is running, has an install folder or settings).
func (p player) installed(running []string, inis []string) bool {
	if len(running) > 0 || len(inis) > 0 {
		return true
	}
	for _, pf := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		for _, d := range p.installDirs {
			if pf != "" {
				if _, err := os.Stat(filepath.Join(pf, d)); err == nil {
					return true
				}
			}
		}
	}
	return run("reg", "query", strings.Join(strings.Split(p.regKey, `\`)[:3], `\`)).ok
}

// belongs reports whether an executable path is one of the player's programs.
func (p player) belongs(exe string) bool {
	base := strings.ToLower(exe[strings.LastIndexAny(exe, `\/`)+1:])
	base = strings.TrimSuffix(base, ".exe")
	for _, n := range p.procs {
		if base == n {
			return true
		}
	}
	return false
}

func runningPlayers() []string {
	if !IsWindows {
		return nil
	}
	var names []string
	for _, p := range players {
		for _, n := range p.procs {
			names = append(names, "'"+n+"'")
		}
	}
	r := run("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"Get-Process -Name "+strings.Join(names, ",")+" -ErrorAction SilentlyContinue | ForEach-Object { try { $_.Path } catch {} }")
	var out []string
	for _, l := range strings.Split(r.stdout, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// EnableMpcWebInterface switches the web interface of MPC-HC and MPC-BE on (whichever is installed). A player
// rewrites its settings when it exits, so it has to be closed while they are changed; without closeMpc the caller
// is told (needsClose) and can ask the user.
func EnableMpcWebInterface(port int, closeMpc bool) WebResult {
	if !IsWindows {
		return WebResult{Message: "Only available on Windows."}
	}
	exes := runningPlayers()
	if len(exes) > 0 && !closeMpc {
		return WebResult{NeedsClose: true, Message: "MPC-HC or MPC-BE is running. It has to be closed while its settings change."}
	}
	if len(exes) > 0 {
		for _, p := range players {
			for _, n := range p.procs {
				run("taskkill", "/IM", n+".exe")
			}
		}
		time.Sleep(3 * time.Second)
		if len(runningPlayers()) > 0 {
			for _, p := range players {
				for _, n := range p.procs {
					run("taskkill", "/F", "/IM", n+".exe")
				}
			}
			time.Sleep(time.Second)
		}
	}
	prt := strconv.Itoa(port)
	ok := true
	edited := []string{}
	var done []string
	for _, p := range players {
		var running, dirs []string
		for _, e := range exes {
			if p.belongs(e) {
				running = append(running, e)
				dirs = append(dirs, filepath.Dir(e))
			}
		}
		inis := p.iniCandidates(dirs)
		if !p.always && !p.installed(running, inis) {
			continue
		}
		a := run("reg", "add", p.regKey, "/v", p.enableKey, "/t", "REG_DWORD", "/d", "1", "/f")
		b := run("reg", "add", p.regKey, "/v", p.portKey, "/t", "REG_DWORD", "/d", prt, "/f")
		ok = ok && a.ok && b.ok
		for _, f := range inis {
			text, u16, err := ReadIni(f)
			if err != nil {
				continue
			}
			t := SetIniValue(text, p.section, p.enableKey, "1")
			t = SetIniValue(t, p.section, p.portKey, prt)
			if WriteIni(f, t, u16) == nil { // read-only install folder: the registry value still applies when no ini exists
				edited = append(edited, f)
			}
		}
		done = append(done, p.name)
	}
	for _, e := range exes {
		c := proc.DetachVisible(exec.Command(e))
		if c.Start() == nil {
			_ = c.Process.Release()
		}
	}
	msg := fmt.Sprintf("%s web interface switched on (port %d).", strings.Join(done, " and "), port)
	if len(exes) > 0 {
		msg += " The player was reopened."
	}
	return WebResult{OK: ok, Message: msg, Edited: edited}
}
// ---- the settings window ---------------------------------------------------------------

// FindBrowser returns Edge/Chrome/Brave/Vivaldi if installed.
func FindBrowser() string {
	var bases []string
	for _, v := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		if p := os.Getenv(v); p != "" {
			bases = append(bases, p)
		}
	}
	rel := [][]string{
		{"Microsoft", "Edge", "Application", "msedge.exe"},
		{"Google", "Chrome", "Application", "chrome.exe"},
		{"BraveSoftware", "Brave-Browser", "Application", "brave.exe"},
		{"Vivaldi", "Application", "vivaldi.exe"},
		{"Chromium", "Application", "chrome.exe"},
	}
	for _, r := range rel {
		for _, b := range bases {
			p := filepath.Join(append([]string{b}, r...)...)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func start(c *exec.Cmd) bool {
	if c.Start() != nil {
		return false
	}
	go c.Wait()
	return true
}

// OpenWindow shows the page in an app-style window (no tabs or address bar) when Edge/Chrome is there, else in the
// default browser. It returns "app", "browser" or "" (nothing could be started). With a profileDir the window
// gets a browser profile of its own, so the user's regular profile is left alone and no background browser outlives
// the window; CloseWindowBrowser(profileDir) ends it.
func OpenWindow(url, profileDir string) string {
	if IsWindows {
		if b := FindBrowser(); b != "" {
			args := []string{"--app=" + url, "--window-size=620,700"}
			if profileDir != "" {
				prepareProfile(profileDir)
				args = append(args, "--user-data-dir="+profileDir, "--no-first-run", "--no-default-browser-check", "--disable-background-mode", "--disable-features=msStartupBoost")
			}
			if start(proc.DetachVisible(exec.Command(b, args...))) {
				return "app"
			}
		}
		if start(proc.Hide(exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url))) {
			return "browser"
		}
		return ""
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	if start(proc.Detach(exec.Command(opener, url))) {
		return "browser"
	}
	return ""
}

// OpenFolder shows a folder in Explorer.
func OpenFolder(dir string) {
	if IsWindows {
		start(proc.DetachVisible(exec.Command("explorer.exe", dir)))
	}
}

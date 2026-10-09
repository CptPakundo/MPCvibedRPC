// Package winsys is everything that touches Windows itself: run-at-login, MPC-HC's web-interface switch, the
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

const (
	runKey  = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	runName = "MPCvibedRPC"
)

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
	r := run("reg", "query", runKey, "/v", runName)
	return r.ok && strings.Contains(strings.ToLower(r.stdout), strings.ToLower(filepath.Base(exe)))
}

// SetAutoStart adds or removes the registry entry.
func SetAutoStart(enabled bool, exe string) bool {
	if !IsWindows {
		return false
	}
	if enabled {
		return run("reg", "add", runKey, "/v", runName, "/t", "REG_SZ", "/d", `"`+exe+`" --background`, "/f").ok
	}
	run("reg", "delete", runKey, "/v", runName, "/f")
	return true
}

// ---- MPC-HC's web interface ------------------------------------------------------

// WebResult is the answer of EnableMpcWebInterface.
type WebResult struct {
	OK         bool     `json:"ok"`
	NeedsClose bool     `json:"needsClose,omitempty"`
	Message    string   `json:"message"`
	Edited     []string `json:"edited,omitempty"`
}

func iniCandidates(extraDirs []string) []string {
	var dirs []string
	dirs = append(dirs, extraDirs...)
	for _, p := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if p != "" {
			dirs = append(dirs, filepath.Join(p, "MPC-HC"), filepath.Join(p, "K-Lite Codec Pack", "MPC-HC64"), filepath.Join(p, "K-Lite Codec Pack", "MPC-HC"))
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range dirs {
		if seen[d] {
			continue
		}
		seen[d] = true
		for _, n := range []string{"mpc-hc.ini", "mpc-hc64.ini"} {
			f := filepath.Join(d, n)
			if _, err := os.Stat(f); err == nil {
				out = append(out, f)
			}
		}
	}
	return out
}

func runningMpc() []string {
	if !IsWindows {
		return nil
	}
	r := run("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"Get-Process -Name 'mpc-hc','mpc-hc64' -ErrorAction SilentlyContinue | ForEach-Object { try { $_.Path } catch {} }")
	var out []string
	for _, l := range strings.Split(r.stdout, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// EnableMpcWebInterface switches MPC-HC's web interface on. MPC-HC rewrites its settings when it exits, so it has
// to be closed while they are changed; without closeMpc the caller is told (needsClose) and can ask the user.
func EnableMpcWebInterface(port int, closeMpc bool) WebResult {
	if !IsWindows {
		return WebResult{Message: "Only available on Windows."}
	}
	exes := runningMpc()
	if len(exes) > 0 && !closeMpc {
		return WebResult{NeedsClose: true, Message: "MPC-HC is running. It has to be closed while its settings change."}
	}
	if len(exes) > 0 {
		run("taskkill", "/IM", "mpc-hc.exe")
		run("taskkill", "/IM", "mpc-hc64.exe")
		time.Sleep(3 * time.Second)
		if len(runningMpc()) > 0 {
			run("taskkill", "/F", "/IM", "mpc-hc.exe")
			run("taskkill", "/F", "/IM", "mpc-hc64.exe")
			time.Sleep(time.Second)
		}
	}
	key := `HKCU\Software\MPC-HC\MPC-HC\Settings`
	p := strconv.Itoa(port)
	a := run("reg", "add", key, "/v", "EnableWebServer", "/t", "REG_DWORD", "/d", "1", "/f")
	b := run("reg", "add", key, "/v", "WebServerPort", "/t", "REG_DWORD", "/d", p, "/f")
	edited := []string{}
	var dirs []string
	for _, e := range exes {
		dirs = append(dirs, filepath.Dir(e))
	}
	for _, f := range iniCandidates(dirs) {
		text, u16, err := ReadIni(f)
		if err != nil {
			continue
		}
		t := SetIniValue(text, "Settings", "EnableWebServer", "1")
		t = SetIniValue(t, "Settings", "WebServerPort", p)
		if WriteIni(f, t, u16) == nil { // read-only install folder: the registry value still applies when no ini exists
			edited = append(edited, f)
		}
	}
	if len(exes) > 0 {
		c := proc.DetachVisible(exec.Command(exes[0]))
		if c.Start() == nil {
			_ = c.Process.Release()
		}
	}
	msg := fmt.Sprintf("MPC-HC web interface switched on (port %d).", port)
	if len(exes) > 0 {
		msg += " MPC-HC was reopened."
	}
	return WebResult{OK: a.ok && b.ok, Message: msg, Edited: edited}
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

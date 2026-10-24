package winsys

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/CptPakundo/MPCvibedRPC/internal/proc"
)

// findBrowser returns Edge/Chrome/Brave/Vivaldi if installed.
func findBrowser() string {
	if !IsWindows {
		return findBrowserUnix()
	}
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
// the window; CloseWindowBrowser(profileDir) ends it. MPCRPC_BROWSER names the browser to use instead, or "none".
func OpenWindow(url, profileDir string) string {
	b := findBrowser()
	switch v := os.Getenv("MPCRPC_BROWSER"); v {
	case "":
	case "none":
		return "" // no window (a computer without a screen, a test)
	default:
		b = v // a browser of the user's choice; it has to understand --app
	}
	if macApp && os.Getenv("MPCRPC_BROWSER") == "" && openNativeWindow(url) {
		return "app" // the macOS app shows the page in a window of its own
	}
	if b != "" {
		args := []string{"--app=" + url, "--window-size=620,700"}
		if profileDir != "" {
			prepareProfile(profileDir)
			args = append(args, "--user-data-dir="+profileDir, "--no-first-run", "--no-default-browser-check", "--disable-background-mode", "--disable-features=msStartupBoost")
		}
		if start(proc.DetachVisible(exec.Command(b, args...))) {
			return "app"
		}
	}
	if OpenURL(url) {
		return "browser"
	}
	return ""
}

// OpenFolder shows a folder in Explorer (macOS: the Finder; Linux: the file manager).
func OpenFolder(dir string) {
	switch {
	case IsWindows:
		start(proc.DetachVisible(exec.Command("explorer.exe", dir)))
	case runtime.GOOS == "darwin":
		start(proc.Detach(exec.Command("open", dir)))
	default:
		start(proc.Detach(exec.Command("xdg-open", dir)))
	}
}

// OpenURL shows a web page in the default browser (the sign-ins of Plex and Stremio, where the user is likely signed in already).
func OpenURL(u string) bool {
	switch {
	case IsWindows:
		return start(proc.Hide(exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", u)))
	case runtime.GOOS == "darwin":
		return start(proc.Detach(exec.Command("open", u)))
	}
	return start(proc.Detach(exec.Command("xdg-open", u)))
}

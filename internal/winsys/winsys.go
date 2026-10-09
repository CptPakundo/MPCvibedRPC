// Package winsys is everything that touches the operating system: run-at-login, the players' web-interface switch
// and connections, the settings window and the tray icon. Windows is the main system; macOS and Linux have their own
// versions of each part where there is one (a LaunchAgent or an autostart entry, mpv.conf, IINA's settings, the
// default browser) and do without the rest (no tray icon or registry).
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

// GetAutoStart reports whether the registry's Run entry (macOS: the LaunchAgent; Linux: the autostart entry) points at
// this program.
func GetAutoStart(exe string) bool {
	if !IsWindows {
		return getLoginItem(exe)
	}
	r := run("reg", "query", runKey, "/v", runName())
	return r.ok && strings.Contains(strings.ToLower(r.stdout), strings.ToLower(filepath.Base(exe)))
}

// SetAutoStart adds or removes the registry entry (or the login item).
func SetAutoStart(enabled bool, exe string) bool {
	if !IsWindows {
		return setLoginItem(enabled, exe)
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
	MpvPipe    string   `json:"mpvPipe,omitempty"` // the IPC name mpv is set up with (may be one the user chose earlier)
	// VLC's web interface as set up (or as the user had it): the caller keeps them in the settings. The password is
	// never sent to the window.
	VlcPort     int    `json:"vlcPort,omitempty"`
	VlcPassword string `json:"-"`
	// IinaPipe is the connection IINA is set up with (macOS; may be one the user chose earlier).
	IinaPipe string `json:"iinaPipe,omitempty"`
	// SettingsChanged tells the window that settings were taken over (it reloads them).
	SettingsChanged bool `json:"settingsChanged,omitempty"`
}

// player describes where one supported player keeps its web-interface switch.
type player struct {
	name        string
	procs       []string // process names without .exe
	regKey      string   // registry key that holds the two values
	section     string   // the same two values inside the player's .ini file
	enableKey   string
	portKey     string
	localKey    string // "answer this PC only"
	iniNames    []string
	installDirs []string // under Program Files
	userIniDir  string   // under %APPDATA%, where a player may keep its .ini
	always      bool     // switch the registry values on even when no install is found
}

var players = []player{
	{
		name: "MPC-HC", procs: []string{"mpc-hc", "mpc-hc64"},
		regKey: `HKCU\Software\MPC-HC\MPC-HC\Settings`, section: "Settings", enableKey: "EnableWebServer", portKey: "WebServerPort", localKey: "WebServerLocalhostOnly",
		iniNames:    []string{"mpc-hc.ini", "mpc-hc64.ini"},
		installDirs: []string{"MPC-HC", `K-Lite Codec Pack\MPC-HC64`, `K-Lite Codec Pack\MPC-HC`},
		always:      true,
	},
	{
		name: "MPC-BE", procs: []string{"mpc-be", "mpc-be64"},
		regKey: `HKCU\Software\MPC-BE\WebServer`, section: "WebServer", enableKey: "EnableWebServer", portKey: "Port", localKey: "LocalhostOnly",
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
	if parts := strings.Split(p.regKey, `\`); len(parts) >= 3 {
		return run("reg", "query", strings.Join(parts[:3], `\`)).ok
	}
	return false
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

// otherPlayers are only looked for (diagnostics, the setup of mpv and VLC): they have no web interface switch of MPC's
// kind. Only VLC is ever closed, and only when its settings have to change.
var otherPlayers = []player{
	{name: "MPC-QT", procs: []string{"mpc-qt"}, installDirs: []string{"MPC-QT"}, userIniDir: "mpc-qt"},
	{name: "mpv", procs: []string{"mpv"}},
	vlcPlayer,
}

var vlcPlayer = player{name: "VLC", procs: []string{"vlc"}, installDirs: []string{"VideoLAN\\VLC"}, userIniDir: "vlc"}

// runningExes lists the paths of the running programs of the given players.
func runningExes(list []player) []string {
	if !IsWindows {
		return unixRunningExes(list)
	}
	var names []string
	for _, p := range list {
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

// runningPlayers lists MPC-HC and MPC-BE executables that are running (the ones closed while settings change).
func runningPlayers() []string { return runningExes(players) }

// RunningPlayerNames lists the supported players that are running right now ("MPC-HC", "MPC-QT", ...), for diagnostics.
func RunningPlayerNames() []string {
	all := append(append([]player{}, players...), otherPlayers...)
	if !IsWindows {
		all = append(all, unixPlayers...)
	}
	exes := runningExes(all)
	var out []string
	for _, p := range all {
		for _, e := range exes {
			if p.belongs(e) {
				out = append(out, p.name)
				break
			}
		}
	}
	return out
}

// SystemInfo describes the operating system for diagnostics (no user or machine names).
func SystemInfo() string {
	if !IsWindows {
		return unixSystemInfo()
	}
	if v := strings.TrimSpace(run("cmd.exe", "/c", "ver").stdout); v != "" {
		return v
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

// EnableMpcWebInterface switches the web interface of MPC-HC and MPC-BE on (whichever is installed), sets mpv up to
// accept our connection (mpvPipe; empty skips mpv) and switches VLC's web interface on (vlcPort, vlcPassword: used
// unless VLC already has its own). MPC-HC, MPC-BE and VLC rewrite their settings when they exit, so they have to be
// closed while those change (VLC only when something changes); without closeMpc the caller is told (needsClose) and
// can ask the user. mpv is never closed: it reads mpv.conf when it next starts. MPC-QT needs nothing. On macOS and
// Linux, enableUnix does the same for the players there (iinaPipe: IINA's connection on macOS).
func EnableMpcWebInterface(port int, closeMpc bool, mpvPipe string, vlcPort int, vlcPassword string, iinaPipe string) WebResult {
	if !IsWindows {
		return enableUnix(port, closeMpc, mpvPipe, vlcPort, vlcPassword, iinaPipe)
	}
	exes := runningPlayers()
	others := runningExes(otherPlayers)
	var vlcExes, vlcDirs []string
	for _, e := range others {
		if vlcPlayer.belongs(e) {
			vlcExes = append(vlcExes, e)
			vlcDirs = append(vlcDirs, filepath.Dir(e))
		}
	}
	vlcPaths, vlcPresent := vlcConfPaths(vlcDirs)
	vlcFile, vlcText := "", ""
	readVlc := func() {
		if b, err := os.ReadFile(vlcFile); err == nil {
			vlcText = string(b)
		}
	}
	if vlcPresent && len(vlcPaths) > 0 {
		vlcFile = vlcPaths[0]
		readVlc()
	}
	closeVlc := vlcFile != "" && !vlcReady(vlcText) && len(vlcExes) > 0
	if (len(exes) > 0 || closeVlc) && !closeMpc {
		var open []string
		if len(exes) > 0 {
			open = append(open, "MPC-HC or MPC-BE")
		}
		if closeVlc {
			open = append(open, "VLC")
		}
		return WebResult{NeedsClose: true, Message: strings.Join(open, " and ") + " is open and has to be closed for a moment (it will reopen)."}
	}
	if !closeVlc {
		vlcExes = nil // left alone
	}
	if len(exes) > 0 || len(vlcExes) > 0 {
		var procs []string
		if len(exes) > 0 {
			for _, p := range players {
				procs = append(procs, p.procs...)
			}
		}
		if len(vlcExes) > 0 {
			procs = append(procs, vlcPlayer.procs...)
		}
		for _, n := range procs {
			run("taskkill", "/IM", n+".exe")
		}
		time.Sleep(3 * time.Second)
		if len(runningPlayers()) > 0 || (len(vlcExes) > 0 && len(runningExes([]player{vlcPlayer})) > 0) {
			for _, n := range procs {
				run("taskkill", "/F", "/IM", n+".exe")
			}
			time.Sleep(time.Second)
		}
		if len(vlcExes) > 0 {
			readVlc() // VLC may have saved its settings on the way out
		}
	}
	prt := strconv.Itoa(port)
	ok := true
	edited := []string{}
	var done []string
	locked := false // a web interface we switched on answers this PC only
	for _, p := range players {
		var running, dirs []string
		for _, e := range exes {
			if p.belongs(e) {
				running = append(running, e)
				dirs = append(dirs, filepath.Dir(e))
			}
		}
		inis := p.iniCandidates(dirs)
		present := p.installed(running, inis)
		if !p.always && !present {
			continue
		}
		// Switched on by us, the web interface only answers this PC: the players' default would also open it (a remote
		// control and file browser) to the whole network. A web interface the user had on already keeps their choice.
		wasOn := regDword(p.regKey, p.enableKey) == 1
		for _, f := range inis {
			if text, _, err := ReadIni(f); err == nil {
				if v, found := GetIniValue(text, p.section, p.enableKey); found && v == "1" {
					wasOn = true
				}
			}
		}
		a := run("reg", "add", p.regKey, "/v", p.enableKey, "/t", "REG_DWORD", "/d", "1", "/f")
		b := run("reg", "add", p.regKey, "/v", p.portKey, "/t", "REG_DWORD", "/d", prt, "/f")
		ok = ok && a.ok && b.ok
		if !wasOn {
			locked = true
			run("reg", "add", p.regKey, "/v", p.localKey, "/t", "REG_DWORD", "/d", "1", "/f")
		}
		for _, f := range inis {
			text, u16, err := ReadIni(f)
			if err != nil {
				continue
			}
			t := SetIniValue(text, p.section, p.enableKey, "1")
			t = SetIniValue(t, p.section, p.portKey, prt)
			if !wasOn {
				t = SetIniValue(t, p.section, p.localKey, "1")
			}
			if WriteIni(f, t, u16) == nil { // read-only install folder: the registry value still applies when no ini exists
				edited = append(edited, f)
			}
		}
		if present {
			done = append(done, p.name)
		}
	}
	for _, e := range exes {
		c := proc.DetachVisible(exec.Command(e))
		if c.Start() == nil {
			_ = c.Process.Release()
		}
	}

	// MPC-QT needs nothing (its own connection is always on); mpv needs input-ipc-server in mpv.conf
	var mpvDirs []string
	qtFound := false
	for _, e := range others {
		if otherPlayers[1].belongs(e) {
			mpvDirs = append(mpvDirs, filepath.Dir(e))
		} else if otherPlayers[0].belongs(e) {
			qtFound = true
		}
	}
	if !qtFound {
		qtFound = otherPlayers[0].installed(nil, nil) || dirExists(filepath.Join(os.Getenv("APPDATA"), "mpc-qt"))
	}
	mpv := enableMpvIPC(mpvPipe, mpvDirs)
	if mpv.Edited != "" {
		edited = append(edited, mpv.Edited)
	}

	// VLC needs its web interface on (extraintf=http) with a password, in the vlcrc it reads
	var vlc vlcResult
	if vlcFile != "" {
		vlc = enableVlcHTTP(vlcFile, vlcText, vlcPort, vlcPassword)
		if vlc.Edited != "" {
			edited = append(edited, vlc.Edited)
			if len(vlcExes) > 0 {
				vlc.Message += " VLC was reopened."
			} else {
				vlc.Message += " It takes effect the next time VLC starts."
			}
		}
		for _, e := range vlcExes {
			c := proc.DetachVisible(exec.Command(e))
			if c.Start() == nil {
				_ = c.Process.Release()
			}
		}
	}

	var parts []string
	if len(done) > 0 || (mpv.Message == "" && vlc.Message == "" && !qtFound) {
		if len(done) == 0 { // nothing found: the MPC-HC values were still written, as they always were
			done = append(done, players[0].name)
		}
		m := fmt.Sprintf("%s web interface switched on (port %d).", strings.Join(done, " and "), port)
		if locked {
			m = fmt.Sprintf("%s web interface switched on (port %d, for this PC only).", strings.Join(done, " and "), port)
		}
		if len(exes) > 0 {
			m += " The player was reopened."
		}
		parts = append(parts, m)
	}
	if qtFound {
		parts = append(parts, "MPC-QT needs no setup.")
	}
	if mpv.Message != "" {
		parts = append(parts, mpv.Message)
		ok = ok && mpv.OK
	}
	if vlc.Message != "" {
		parts = append(parts, vlc.Message)
		ok = ok && vlc.OK
	}
	return WebResult{OK: ok, Message: strings.Join(parts, " "), Edited: edited, MpvPipe: mpv.Using, VlcPort: vlc.Port, VlcPassword: vlc.Password}
}

// regDword reads a REG_DWORD value (-1 when it is not there).
func regDword(key, name string) int {
	r := run("reg", "query", key, "/v", name)
	if !r.ok {
		return -1
	}
	return parseRegDword(r.stdout)
}

// parseRegDword finds the number in reg.exe's answer ("    EnableWebServer    REG_DWORD    0x1").
func parseRegDword(out string) int {
	f := strings.Fields(out)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "REG_DWORD" {
			if n, err := strconv.ParseInt(strings.TrimPrefix(f[i+1], "0x"), 16, 64); err == nil {
				return int(n)
			}
		}
	}
	return -1
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// ---- the settings window ---------------------------------------------------------------

// FindBrowser returns Edge/Chrome/Brave/Vivaldi if installed.
func FindBrowser() string {
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
	b := FindBrowser()
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
	if IsWindows {
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

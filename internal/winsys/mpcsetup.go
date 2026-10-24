package winsys

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/proc"
)

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
var otherPlayers = []player{mpcQTPlayer, mpvPlayer, vlcPlayer}

// readOnlyPlayers need no setup and are only named in diagnostics.
var readOnlyPlayers = []player{{name: "PotPlayer", procs: []string{"potplayermini64", "potplayermini", "potplayer64", "potplayer"}}}

var (
	mpcQTPlayer = player{name: "MPC-QT", procs: []string{"mpc-qt"}, installDirs: []string{"MPC-QT"}, userIniDir: "mpc-qt"}
	mpvPlayer   = player{name: "mpv", procs: []string{"mpv"}}
	vlcPlayer   = player{name: "VLC", procs: []string{"vlc"}, installDirs: []string{"VideoLAN\\VLC"}, userIniDir: "vlc"}
)

// EnableMpcWebInterface switches the web interface of MPC-HC and MPC-BE on (whichever is installed), sets mpv up to
// accept our connection (mpvPipe; empty skips mpv) and switches VLC's web interface on (vlcPort, vlcPassword: used
// unless VLC already has its own). MPC-HC, MPC-BE and VLC rewrite their settings when they exit, so they have to be
// closed while those change (VLC only when something changes); without closeMpc the caller is told (needsClose) and
// can ask the user. mpv is never closed: it reads mpv.conf when it next starts. MPC-QT needs nothing. On macOS and
// Linux, enableUnix does the same for the players there (iinaPipe: IINA's connection on macOS).
//
// The steps run in this order: look for the running players, close them, set up MPC-HC and MPC-BE, reopen them, set
// up mpv, set up VLC (and reopen it), and word the result.
func EnableMpcWebInterface(port int, closeMpc bool, mpvPipe string, vlcPort int, vlcPassword string, iinaPipe string) WebResult {
	if !IsWindows {
		return enableUnix(port, closeMpc, mpvPipe, vlcPort, vlcPassword, iinaPipe)
	}
	exes := runningPlayers()
	others := runningExes(otherPlayers)
	vlcAt := findVlc(others)
	closeVlc := vlcAt.file != "" && !vlcReady(vlcAt.text) && len(vlcAt.exes) > 0
	if (len(exes) > 0 || closeVlc) && !closeMpc {
		return WebResult{NeedsClose: true, Message: openPlayersMessage(len(exes) > 0, closeVlc)}
	}
	if !closeVlc {
		vlcAt.exes = nil // left alone
	}
	closePlayers(exes, &vlcAt)
	mpc := setUpMpcFamily(exes, port)
	reopenPlayers(exes)

	// MPC-QT needs nothing (its own connection is always on); mpv needs input-ipc-server in mpv.conf
	mpv, qtFound := setUpMpv(others, mpvPipe)
	edited := mpc.edited
	if mpv.Edited != "" {
		edited = append(edited, mpv.Edited)
	}

	// VLC needs its web interface on (extraintf=http) with a password, in the vlcrc it reads
	vlc := setUpVlc(vlcAt, vlcPort, vlcPassword)
	if vlc.Edited != "" {
		edited = append(edited, vlc.Edited)
	}

	message, ok := describeSetup(port, mpc, len(exes) > 0, mpv, vlc, qtFound)
	return WebResult{OK: ok, Message: message, Edited: edited, MpvPipe: mpv.Using, VlcPort: vlc.Port, VlcPassword: vlc.Password}
}

// vlcSpot is the VLC found on this PC before the setup: its running programs, the vlcrc it reads and that file's text.
type vlcSpot struct {
	exes []string
	file string // "" when there is no vlcrc to set up
	text string
}

// read loads the vlcrc's text again (VLC may have saved its settings on the way out).
func (v *vlcSpot) read() {
	if b, err := os.ReadFile(v.file); err == nil {
		v.text = string(b)
	}
}

// findVlc looks for VLC among the running programs: where they run from tells which vlcrc it reads.
func findVlc(others []string) vlcSpot {
	var v vlcSpot
	var vlcDirs []string
	for _, e := range others {
		if vlcPlayer.belongs(e) {
			v.exes = append(v.exes, e)
			vlcDirs = append(vlcDirs, filepath.Dir(e))
		}
	}
	vlcPaths, vlcPresent := vlcConfPaths(vlcDirs)
	if vlcPresent && len(vlcPaths) > 0 {
		v.file = vlcPaths[0]
		v.read()
	}
	return v
}

// openPlayersMessage is what the caller is told when the players have to be closed first.
func openPlayersMessage(mpcOpen, vlcOpen bool) string {
	var open []string
	if mpcOpen {
		open = append(open, "MPC-HC or MPC-BE")
	}
	if vlcOpen {
		open = append(open, "VLC")
	}
	return strings.Join(open, " and ") + " is open and has to be closed for a moment (it will reopen)."
}

// closePlayers closes the running MPC-HC/MPC-BE (exes) and the VLC that is to be reopened (vlc.exes, empty when VLC is
// left alone), first politely, then by force; VLC's settings are read again once it is gone.
func closePlayers(exes []string, vlc *vlcSpot) {
	if len(exes) > 0 || len(vlc.exes) > 0 {
		var procs []string
		if len(exes) > 0 {
			for _, p := range players {
				procs = append(procs, p.procs...)
			}
		}
		if len(vlc.exes) > 0 {
			procs = append(procs, vlcPlayer.procs...)
		}
		for _, n := range procs {
			run("taskkill", "/IM", n+".exe")
		}
		time.Sleep(3 * time.Second)
		if len(runningPlayers()) > 0 || (len(vlc.exes) > 0 && len(runningExes([]player{vlcPlayer})) > 0) {
			for _, n := range procs {
				run("taskkill", "/F", "/IM", n+".exe")
			}
			time.Sleep(time.Second)
		}
		if len(vlc.exes) > 0 {
			vlc.read() // VLC may have saved its settings on the way out
		}
	}
}

// mpcSetup is what switching the MPC-HC and MPC-BE web interfaces on did.
type mpcSetup struct {
	ok     bool
	edited []string // the .ini files that were changed
	done   []string // the players found
	locked bool     // a web interface we switched on answers this PC only
}

// setUpMpcFamily switches the web interface of MPC-HC and MPC-BE on, in the registry and in every .ini file found.
func setUpMpcFamily(exes []string, port int) mpcSetup {
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
			if text, _, err := readIni(f); err == nil {
				if v, found := getIniValue(text, p.section, p.enableKey); found && v == "1" {
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
			text, u16, err := readIni(f)
			if err != nil {
				continue
			}
			t := setIniValue(text, p.section, p.enableKey, "1")
			t = setIniValue(t, p.section, p.portKey, prt)
			if !wasOn {
				t = setIniValue(t, p.section, p.localKey, "1")
			}
			if writeIni(f, t, u16) == nil { // read-only install folder: the registry value still applies when no ini exists
				edited = append(edited, f)
			}
		}
		if present {
			done = append(done, p.name)
		}
	}
	return mpcSetup{ok: ok, edited: edited, done: done, locked: locked}
}

// startDetached opens a program again, visible and independent of this one.
func startDetached(exe string) {
	c := proc.DetachVisible(exec.Command(exe))
	if c.Start() == nil {
		_ = c.Process.Release()
	}
}

// reopenPlayers starts the MPC-HC/MPC-BE programs that were closed, once each.
func reopenPlayers(exes []string) {
	reopened := map[string]bool{}
	for _, e := range exes {
		if reopened[strings.ToLower(e)] {
			continue // two windows of one player: it is opened once
		}
		reopened[strings.ToLower(e)] = true
		startDetached(e)
	}
}

// setUpMpv sets mpv up (input-ipc-server in the mpv.conf it reads) and says whether MPC-QT, which needs nothing, is
// on this PC.
func setUpMpv(others []string, mpvPipe string) (mpv mpvResult, qtFound bool) {
	var mpvDirs []string
	for _, e := range others {
		if mpvPlayer.belongs(e) {
			mpvDirs = append(mpvDirs, filepath.Dir(e))
		} else if mpcQTPlayer.belongs(e) {
			qtFound = true
		}
	}
	if !qtFound {
		qtFound = mpcQTPlayer.installed(nil, nil) || dirExists(filepath.Join(os.Getenv("APPDATA"), "mpc-qt"))
	}
	return enableMpvIPC(mpvPipe, mpvDirs), qtFound
}

// setUpVlc switches VLC's web interface on in its vlcrc (when there is one), then reopens the VLC that was closed for it.
func setUpVlc(v vlcSpot, vlcPort int, vlcPassword string) vlcResult {
	var vlc vlcResult
	if v.file != "" {
		vlc = enableVlcHTTP(v.file, v.text, vlcPort, vlcPassword)
		if vlc.Edited != "" {
			if len(v.exes) > 0 {
				vlc.Message += " VLC was reopened."
			} else {
				vlc.Message += " It takes effect the next time VLC starts."
			}
		}
		for _, e := range v.exes {
			startDetached(e)
		}
	}
	return vlc
}

// describeSetup words what was done, and whether all of it worked. mpcReopened: MPC-HC or MPC-BE was open and has been
// started again.
func describeSetup(port int, mpc mpcSetup, mpcReopened bool, mpv mpvResult, vlc vlcResult, qtFound bool) (message string, ok bool) {
	ok = mpc.ok
	done := mpc.done
	var parts []string
	if len(done) > 0 || (mpv.Message == "" && vlc.Message == "" && !qtFound) {
		if len(done) == 0 { // nothing found: the MPC-HC values were still written, as they always were
			done = append(done, players[0].name)
		}
		m := fmt.Sprintf("%s web interface switched on (port %d).", strings.Join(done, " and "), port)
		if mpc.locked {
			m = fmt.Sprintf("%s web interface switched on (port %d, for this PC only).", strings.Join(done, " and "), port)
		}
		if mpcReopened {
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
	return strings.Join(parts, " "), ok
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

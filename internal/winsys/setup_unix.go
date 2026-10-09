package winsys

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/pipe"
)

// "Set up the player connection" on macOS and Linux. There is no MPC-HC or MPC-BE there. mpv gets input-ipc-server
// in its mpv.conf, as on Windows. On macOS, IINA gets the same mpv option in its own settings, and VLC its web
// interface in vlcrc (VLC is closed for a moment when that has to change, after the user agrees). On Linux, VLC and
// the other video players need nothing: they are read through MPRIS. MPC-QT needs nothing anywhere.

// iinaDomain is IINA's settings domain (its bundle identifier).
const iinaDomain = "com.colliderli.iina"

func enableUnix(port int, closeMpc bool, mpvPipe string, vlcPort int, vlcPassword string, iinaPipe string) WebResult {
	mac := runtime.GOOS == "darwin"
	home, _ := os.UserHomeDir()

	// VLC on macOS: find out first whether it has to be closed
	var vlcFile, vlcText string
	var vlcExes []string
	if mac {
		if paths, present := vlcConfPathsMac(home); present {
			vlcFile = paths[0]
			if b, err := os.ReadFile(vlcFile); err == nil {
				vlcText = string(b)
			}
			vlcExes = runningExes([]player{vlcPlayer})
		}
	}
	closeVlc := vlcFile != "" && !vlcReady(vlcText) && len(vlcExes) > 0
	if closeVlc && !closeMpc {
		return WebResult{NeedsClose: true, Message: "VLC is open and has to be closed for a moment (it will reopen)."}
	}
	if closeVlc {
		run("osascript", "-e", `tell application id "org.videolan.vlc" to quit`)
		for i := 0; i < 12 && len(runningExes([]player{vlcPlayer})) > 0; i++ {
			time.Sleep(250 * time.Millisecond)
		}
		if len(runningExes([]player{vlcPlayer})) > 0 {
			run("pkill", "-x", "VLC")
			time.Sleep(time.Second)
		}
		if b, err := os.ReadFile(vlcFile); err == nil {
			vlcText = string(b) // VLC may have saved its settings on the way out
		}
	}

	ok := true
	var parts, edited []string
	mpv := enableMpvIPC(mpvPipe, nil)
	if mpv.Message != "" {
		parts = append(parts, mpv.Message)
		ok = ok && mpv.OK
	}
	if mpv.Edited != "" {
		edited = append(edited, mpv.Edited)
	}

	var iina iinaResult
	if mac {
		iina = enableIINA(home, iinaPipe)
		if iina.Message != "" {
			parts = append(parts, iina.Message)
			ok = ok && iina.OK
		}
		if iina.Edited {
			edited = append(edited, "IINA settings")
		}
	}

	var vlc vlcResult
	if vlcFile != "" {
		vlc = enableVlcHTTP(vlcFile, vlcText, vlcPort, vlcPassword)
		if vlc.Edited != "" {
			edited = append(edited, vlc.Edited)
			if closeVlc {
				vlc.Message += " VLC was reopened."
			} else {
				vlc.Message += " It takes effect the next time VLC starts."
			}
		}
		if closeVlc {
			run("open", "-b", "org.videolan.vlc")
		}
		parts = append(parts, vlc.Message)
		ok = ok && vlc.OK
	}

	if !mac {
		parts = append(parts, "VLC, Celluloid, Haruna, SMPlayer and the other video players that offer MPRIS need no setup.")
	} else if _, err := exec.LookPath("mpc-qt"); err == nil || dirExists("/Applications/mpc-qt.app") {
		parts = append(parts, "MPC-QT needs no setup.")
	}
	if len(parts) == 0 {
		parts = append(parts, "No player that needs setting up was found (mpv, IINA or VLC).")
		ok = false
	}
	return WebResult{OK: ok, Message: strings.Join(parts, " "), Edited: edited, MpvPipe: mpv.Using,
		VlcPort: vlc.Port, VlcPassword: vlc.Password, IinaPipe: iina.Using}
}

// mpvConfPathsUnix is where mpv reads mpv.conf on macOS and Linux ($XDG_CONFIG_HOME/mpv, else ~/.config/mpv).
func mpvConfPathsUnix(home string) (paths []string, present bool) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		if home == "" {
			return nil, false
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "mpv")
	_, err := exec.LookPath("mpv")
	present = err == nil || dirExists(dir) || dirExists("/Applications/mpv.app") || len(runningExes([]player{otherPlayers[1]})) > 0
	return []string{filepath.Join(dir, "mpv.conf")}, present
}

// vlcConfPathsMac is where VLC for macOS keeps vlcrc.
func vlcConfPathsMac(home string) (paths []string, present bool) {
	if home == "" {
		return nil, false
	}
	dir := filepath.Join(home, "Library", "Preferences", "org.videolan.vlc")
	present = dirExists(dir) || dirExists("/Applications/VLC.app") || dirExists(filepath.Join(home, "Applications", "VLC.app"))
	return []string{filepath.Join(dir, "vlcrc")}, present
}

// iinaResult is what setting up IINA did.
type iinaResult struct {
	Message string // empty when IINA is not on this Mac
	Using   string // the connection IINA will offer
	Edited  bool
	OK      bool
}

// iinaSettings are the parts of IINA's settings that matter here.
type iinaSettings struct {
	Advanced bool       // "Enable advanced settings": without it IINA ignores its extra mpv options
	Options  [][]string // the extra mpv options, as name/value pairs
}

// readIINA reads IINA's settings from its exported property list.
func readIINA(export []byte) iinaSettings {
	var s iinaSettings
	v, err := parsePlist(bytes.NewReader(export))
	if err != nil {
		return s
	}
	m, _ := v.(map[string]any)
	s.Advanced, _ = m["enableAdvancedSettings"].(bool)
	rows, _ := m["userOptions"].([]any)
	for _, r := range rows {
		cells, _ := r.([]any)
		var row []string
		for _, c := range cells {
			str, _ := c.(string)
			row = append(row, str)
		}
		if len(row) == 2 {
			s.Options = append(s.Options, row)
		}
	}
	return s
}

// iinaPresent reports whether IINA is on this Mac: in Applications, running, or with settings of its own.
func iinaPresent(home string, hasOwnSettings bool) bool {
	return hasOwnSettings || dirExists("/Applications/IINA.app") || dirExists(filepath.Join(home, "Applications", "IINA.app")) ||
		len(runningExes(unixPlayers[:1])) > 0
}

// hasSettings reports whether an exported property list holds any setting.
func hasSettings(export []byte) bool {
	v, err := parsePlist(bytes.NewReader(export))
	m, _ := v.(map[string]any)
	return err == nil && len(m) > 0
}

// planIINA decides what to change: the options to write (nil = leave them) and the connection IINA will offer. A
// connection the user set up already is kept.
func planIINA(s iinaSettings, name string) (options [][]string, using string) {
	for _, o := range s.Options {
		if o[0] == mpvIPCKey && strings.TrimSpace(o[1]) != "" {
			return nil, o[1]
		}
	}
	return append(append([][]string{}, s.Options...), []string{mpvIPCKey, pipe.Path(name)}), name
}

// enableIINA gives IINA the mpv option that opens its connection (IINA Settings > Advanced > Additional mpv options)
// and switches its advanced settings on, which IINA needs to use the option. IINA reads them when a player window
// opens, so it is not closed: the user is told to reopen it.
func enableIINA(home, name string) iinaResult {
	export := run("defaults", "export", iinaDomain, "-")
	// "defaults export" also succeeds, with an empty list, for an app that was never there
	if !iinaPresent(home, export.ok && hasSettings([]byte(export.stdout))) {
		return iinaResult{}
	}
	if name == "" {
		return iinaResult{Message: "IINA is skipped: its connection name (Advanced) is empty."}
	}
	s := readIINA([]byte(export.stdout))
	options, using := planIINA(s, name)
	if options == nil && s.Advanced {
		return iinaResult{Message: "IINA was already set up (" + mpvIPCKey + "=" + using + ").", Using: using, OK: true}
	}
	if options != nil && !run("defaults", "write", iinaDomain, "userOptions", oldStylePlist(options)).ok {
		return iinaResult{Message: "IINA: could not change its settings."}
	}
	if !s.Advanced && !run("defaults", "write", iinaDomain, "enableAdvancedSettings", "-bool", "true").ok {
		return iinaResult{Message: "IINA: could not switch its advanced settings on."}
	}
	msg := "IINA set up (Settings > Advanced); it takes effect the next time IINA starts."
	if len(runningExes(unixPlayers[:1])) > 0 {
		msg = "IINA set up (Settings > Advanced); quit IINA and open it again to connect."
	}
	return iinaResult{Message: msg, Using: using, Edited: true, OK: true}
}

package winsys

import (
	"html"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CptPakundo/MPRP/internal/core"
)

// Run at login on macOS (a LaunchAgent) and Linux (an XDG autostart entry, which every desktop reads). The file is the
// source of truth, as the registry value is on Windows; a copy with its own MPRP_HOME gets a file of its own.

// loginItemPath is where the login item of this copy lives on goos ("" when the home folder is unknown).
func loginItemPath(goos, home, xdgConfig string) string {
	return loginItemPathNamed(runName(), goos, home, xdgConfig)
}

// loginItemPathNamed is where the login item called name lives on goos.
func loginItemPathNamed(name, goos, home, xdgConfig string) string {
	name = strings.ToLower(name)
	if goos == "darwin" {
		if home == "" {
			return ""
		}
		return filepath.Join(home, "Library", "LaunchAgents", "io.github.cptpakundo."+name+".plist")
	}
	if xdgConfig == "" {
		if home == "" {
			return ""
		}
		xdgConfig = filepath.Join(home, ".config")
	}
	return filepath.Join(xdgConfig, "autostart", name+".desktop")
}

// loginItem is the content of the login item that starts exe in the background (with this copy's data folder).
func loginItem(goos, exe, dataHome string) string {
	if goos == "darwin" {
		x := html.EscapeString
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>io.github.cptpakundo.` + x(strings.ToLower(runName())) + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + x(exe) + `</string>
		<string>--background</string>
	</array>
`)
		if dataHome != "" {
			b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>MPRP_HOME</key>\n\t\t<string>" + x(dataHome) + "</string>\n\t</dict>\n")
		}
		b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n\t<key>ProcessType</key>\n\t<string>Interactive</string>\n</dict>\n</plist>\n")
		return b.String()
	}
	cmd := execQuote(exe) + " --background"
	if dataHome != "" {
		cmd = "env " + execQuote("MPRP_HOME="+dataHome) + " " + cmd
	}
	return "[Desktop Entry]\nType=Application\nName=MPRP\nComment=Shows what your video player is playing on Discord\n" +
		"Exec=" + cmd + "\nTerminal=false\nX-GNOME-Autostart-enabled=true\n"
}

// execQuote quotes one argument for the Exec line of a .desktop file: the quoting rules of the specification, then
// the escaping of its string type (so a backslash ends up doubled twice), and % doubled.
func execQuote(arg string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '"', '`', '$':
			b.WriteString(`\\` + string(r))
		case '\\':
			b.WriteString(`\\\\`)
		case '%':
			b.WriteString("%%")
		case '\n', '\r':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func userLoginItem() string {
	home, _ := os.UserHomeDir()
	return loginItemPath(runtime.GOOS, home, os.Getenv("XDG_CONFIG_HOME"))
}

// getLoginItem reports whether the login item exists and starts this program.
func getLoginItem(exe string) bool {
	p := userLoginItem()
	if p == "" {
		return false
	}
	b, err := os.ReadFile(p)
	return err == nil && strings.Contains(string(b), strings.TrimSuffix(strings.TrimPrefix(loginItemExe(exe), `"`), `"`))
}

// loginItemExe is how exe appears inside the login item.
func loginItemExe(exe string) string {
	if runtime.GOOS == "darwin" {
		return html.EscapeString(exe)
	}
	return execQuote(exe)
}

// translocated reports whether macOS runs the app from a temporary copy: an app opened from the folder it was
// downloaded to, before it was moved (to Applications), runs from a random read-only path that does not last.
func translocated(exe string) bool { return strings.Contains(exe, "/AppTranslocation/") }

// setLoginItem writes or removes the login item.
func setLoginItem(enabled bool, exe string) bool {
	p := userLoginItem()
	if p == "" {
		return false
	}
	if !enabled {
		err := os.Remove(p)
		return err == nil || os.IsNotExist(err)
	}
	if translocated(exe) {
		return false
	}
	if os.MkdirAll(filepath.Dir(p), 0o755) != nil {
		return false
	}
	return os.WriteFile(p, []byte(loginItem(runtime.GOOS, exe, os.Getenv("MPRP_HOME"))), 0o644) == nil
}

// migrateLoginItem replaces the login item of the program's former name (macOS: io.github.cptpakundo.<former
// name>.plist; Linux: <former name>.desktop) with one of the current name that starts exe, and removes the former
// one. A login item of the current name that is there already is kept. When the new one cannot be written (a
// translocated app, an unwritable folder) the former one is left as it is.
func migrateLoginItem(goos, home, xdgConfig, exe, mprpHome string) string {
	from := loginItemPathNamed(nameFor(core.FormerName, mprpHome), goos, home, xdgConfig)
	to := loginItemPathNamed(nameFor(runNameBase, mprpHome), goos, home, xdgConfig)
	if from == "" || to == "" {
		return ""
	}
	if _, err := os.Stat(from); err != nil {
		return ""
	}
	if _, err := os.Stat(to); err != nil {
		if translocated(exe) || os.MkdirAll(filepath.Dir(to), 0o755) != nil ||
			os.WriteFile(to, []byte(loginItem(goos, exe, mprpHome)), 0o644) != nil {
			return "Could not carry the login item " + from + " over to " + to + "; it is left as it was."
		}
	}
	if err := os.Remove(from); err != nil {
		return "Wrote the login item " + to + " but could not remove " + from + ": " + err.Error()
	}
	return "Moved the login item " + from + " to " + to + " (the program's new name)."
}

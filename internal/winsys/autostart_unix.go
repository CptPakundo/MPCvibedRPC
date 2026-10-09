package winsys

import (
	"html"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Run at login on macOS (a LaunchAgent) and Linux (an XDG autostart entry, which every desktop reads). The file is the
// source of truth, as the registry value is on Windows; a copy with its own MPCRPC_HOME gets a file of its own.

// loginItemPath is where the login item of this copy lives on goos ("" when the home folder is unknown).
func loginItemPath(goos, home, xdgConfig string) string {
	name := strings.ToLower(runName())
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
func loginItem(goos, exe, mpcHome string) string {
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
		if mpcHome != "" {
			b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>MPCRPC_HOME</key>\n\t\t<string>" + x(mpcHome) + "</string>\n\t</dict>\n")
		}
		b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n\t<key>ProcessType</key>\n\t<string>Interactive</string>\n</dict>\n</plist>\n")
		return b.String()
	}
	cmd := execQuote(exe) + " --background"
	if mpcHome != "" {
		cmd = "env " + execQuote("MPCRPC_HOME="+mpcHome) + " " + cmd
	}
	return "[Desktop Entry]\nType=Application\nName=MPCvibedRPC\nComment=Shows what your video player is playing on Discord\n" +
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
	return os.WriteFile(p, []byte(loginItem(runtime.GOOS, exe, os.Getenv("MPCRPC_HOME"))), 0o644) == nil
}

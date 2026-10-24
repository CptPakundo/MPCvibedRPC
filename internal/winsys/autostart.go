package winsys

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// runNameBase is the name of the registry value that starts the program at login.
const runNameBase = "MPCvibedRPC"

// runName is the registry value for run-at-login. A copy that uses a data folder of its own (MPCRPC_HOME: a portable
// setup, a test, a development build) gets a value of its own, derived from that folder, so it can never switch the
// installed program's run-at-login on or off.
func runName() string { return runNameFor(os.Getenv("MPCRPC_HOME")) }

// runCommand is the registry value's command line. A copy with its own data folder passes it on with --home, so the
// copy started at login uses that folder too (the registry cannot set MPCRPC_HOME the way a LaunchAgent can).
func runCommand(exe, home string) string {
	cmd := `"` + exe + `" --background`
	if home = strings.TrimRight(strings.TrimSpace(home), `\/`); home != "" {
		cmd += ` --home "` + home + `"` // no trailing backslash: it would escape the closing quote
	}
	return cmd
}

func runNameFor(home string) string {
	home = strings.ToLower(strings.TrimRight(strings.TrimSpace(home), `\/`))
	if home == "" {
		return runNameBase
	}
	sum := sha256.Sum256([]byte(home))
	return runNameBase + "-" + hex.EncodeToString(sum[:4])
}

// runKey is the registry key whose values start programs at login.
const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// GetAutoStart reports whether the registry's Run entry (macOS: the LaunchAgent; Linux: the autostart entry) is there
// and names a program file with the same name as this one (it does not compare the full path).
func GetAutoStart(exe string) bool {
	if !IsWindows {
		return getLoginItem(exe)
	}
	data, ok := registryRunValue(runName())
	return ok && runEntryNames(data, exe)
}

// runEntryNames says whether the Run entry's command line names a program file with the same name as exe (case does
// not matter; the folder is not compared).
func runEntryNames(command, exe string) bool {
	return strings.Contains(strings.ToLower(command), strings.ToLower(filepath.Base(exe)))
}

// SetAutoStart adds or removes the registry entry (or the login item).
func SetAutoStart(enabled bool, exe string) bool {
	if !IsWindows {
		return setLoginItem(enabled, exe)
	}
	if enabled {
		return run("reg", "add", runKey, "/v", runName(), "/t", "REG_SZ", "/d", runCommand(exe, os.Getenv("MPCRPC_HOME")), "/f").ok
	}
	run("reg", "delete", runKey, "/v", runName(), "/f")
	return true
}

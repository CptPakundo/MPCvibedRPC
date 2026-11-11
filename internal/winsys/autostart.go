package winsys

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
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
	return ok && runEntryNames(data, exe) && !startupDisabled(approvedData(runName()))
}

// approvedData is Windows' own startup switch for a Run value (nil when there is none).
func approvedData(name string) []byte {
	d, _ := registryStartupApproved(name)
	return d
}

// startupDisabled reads the value Windows keeps under StartupApproved\Run when the user switches an entry off in Task
// Manager > Startup apps: the first byte is even (0x02, 0x06) when the entry is enabled and odd (0x03, 0x07) when it
// is disabled. No value, or an empty one, means enabled.
func startupDisabled(data []byte) bool {
	return len(data) > 0 && data[0]&1 == 1
}

// startupApprovedKey is the key where Windows keeps that switch, one value per Run value.
const startupApprovedKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`

// runEntryNames says whether the Run entry's command line names a program file with the same name as exe (case does
// not matter; the folder is not compared).
func runEntryNames(command, exe string) bool {
	// the file's name after the last separator of either kind: a Windows path is compared on every system (tests)
	name := exe[strings.LastIndexAny(exe, `\/`)+1:]
	return name != "" && strings.Contains(strings.ToLower(command), strings.ToLower(name))
}

// writeRunValue makes the Run value name start exe. A value that is already right is left alone: no helper process
// at every start of the program.
func writeRunValue(name, exe string) bool {
	want := runCommand(exe, os.Getenv("MPCRPC_HOME"))
	if cur, found := registryRunValue(name); found && cur == want {
		return true
	}
	return run("reg", "add", runKey, "/v", name, "/t", "REG_SZ", "/d", want, "/f").ok
}

// RefreshAutoStart keeps the run-at-login entry pointing at this copy of the program, which may have been moved: the
// program calls it at every start while the setting is on. Unlike SetAutoStart(true, ...) it leaves Windows' own
// switch alone: a user who turned the entry off in Task Manager meant it, and only turning the setting on in the
// window turns it back on.
func RefreshAutoStart(exe string) bool {
	if !IsWindows {
		return setLoginItem(true, exe)
	}
	return writeRunValue(runName(), exe)
}

// SetAutoStart adds or removes the registry entry (or the login item).
func SetAutoStart(enabled bool, exe string) bool {
	if !IsWindows {
		return setLoginItem(enabled, exe)
	}
	name := runName()
	if enabled {
		ok := writeRunValue(name, exe)
		// switched off in Task Manager: whoever turns the setting on here wants it on
		if startupDisabled(approvedData(name)) {
			run("reg", "delete", startupApprovedKey, "/v", name, "/f")
		}
		return ok
	}
	run("reg", "delete", runKey, "/v", name, "/f")
	if _, found := registryStartupApproved(name); found {
		run("reg", "delete", startupApprovedKey, "/v", name, "/f") // nothing of ours is left behind
	}
	return true
}

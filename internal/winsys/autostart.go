package winsys

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strings"

	"github.com/CptPakundo/MPRP/internal/core"
)

// runNameBase is the name of the registry value that starts the program at login.
const runNameBase = core.Name

// runName is the registry value for run-at-login. A copy that uses a data folder of its own (MPRP_HOME: a portable
// setup, a test, a development build) gets a value of its own, derived from that folder, so it can never switch the
// installed program's run-at-login on or off.
func runName() string { return runNameFor(os.Getenv("MPRP_HOME")) }

// runCommand is the registry value's command line. A copy with its own data folder passes it on with --home, so the
// copy started at login uses that folder too (the registry cannot set MPRP_HOME the way a LaunchAgent can).
func runCommand(exe, home string) string {
	cmd := `"` + exe + `" --background`
	if home = strings.TrimRight(strings.TrimSpace(home), `\/`); home != "" {
		cmd += ` --home "` + home + `"` // no trailing backslash: it would escape the closing quote
	}
	return cmd
}

func runNameFor(home string) string { return nameFor(runNameBase, home) }

// formerRunName is the run-at-login value (and login item) of this copy under the program's former name.
func formerRunName() string { return nameFor(core.FormerName, os.Getenv("MPRP_HOME")) }

// nameFor is the name base, for the copy with the data folder home ("": the default one).
func nameFor(base, home string) string {
	home = strings.ToLower(strings.TrimRight(strings.TrimSpace(home), `\/`))
	if home == "" {
		return base
	}
	sum := sha256.Sum256([]byte(home))
	return base + "-" + hex.EncodeToString(sum[:4])
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
	want := runCommand(exe, os.Getenv("MPRP_HOME"))
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

// runRegistry is what the run-at-login migration needs of the registry: the Run values and Windows' own startup
// switch for each (StartupApproved\Run). systemRegistry is the real one; the tests pass a map.
type runRegistry struct {
	run            func(name string) (string, bool)
	approved       func(name string) ([]byte, bool)
	setRun         func(name, command string) bool
	setApproved    func(name string, data []byte) bool
	deleteRun      func(name string) bool
	deleteApproved func(name string) bool
}

var systemRegistry = runRegistry{
	run:      registryRunValue,
	approved: registryStartupApproved,
	setRun: func(name, command string) bool {
		return run("reg", "add", runKey, "/v", name, "/t", "REG_SZ", "/d", command, "/f").ok
	},
	setApproved: func(name string, data []byte) bool {
		return run("reg", "add", startupApprovedKey, "/v", name, "/t", "REG_BINARY", "/d", hex.EncodeToString(data), "/f").ok
	},
	deleteRun:      func(name string) bool { return run("reg", "delete", runKey, "/v", name, "/f").ok },
	deleteApproved: func(name string) bool { return run("reg", "delete", startupApprovedKey, "/v", name, "/f").ok },
}

// MigrateAutoStart carries run-at-login over from the program's former name: the Run value (Windows), the
// LaunchAgent (macOS) or the autostart entry (Linux) of that name is replaced by one of the current name that starts
// this program file. On Windows a start switched off in Task Manager stays switched off. The program calls it at every
// start; it does nothing when there is no entry of the former name. It gives a sentence for the log ("" when there was
// nothing to do).
func MigrateAutoStart(exe string) string {
	home := os.Getenv("MPRP_HOME")
	if !IsWindows {
		h, _ := os.UserHomeDir()
		return migrateLoginItem(runtime.GOOS, h, os.Getenv("XDG_CONFIG_HOME"), exe, home)
	}
	return migrateRunValue(systemRegistry, formerRunName(), runName(), runCommand(exe, home))
}

// migrateRunValue moves the Run value from (and its startup switch) to the name to, with command as its command line.
// A value of that name that is there already is kept. When the new entry cannot be written, the former one is left as
// it is: a start at login that still works beats none.
func migrateRunValue(r runRegistry, from, to, command string) string {
	_, hasRun := r.run(from)
	approved, hasApproved := r.approved(from)
	if !hasRun && !hasApproved {
		return ""
	}
	if !hasRun { // a switch left without its value: Windows' leftover
		r.deleteApproved(from)
		return "Removed the leftover Task Manager entry " + from + " of the program's former name."
	}
	// the switch first: a start the user switched off must never come back on, even for a moment
	wroteApproved := false
	if _, has := r.approved(to); hasApproved && len(approved) > 0 && !has {
		if !r.setApproved(to, approved) {
			return "Could not carry the run-at-login entry " + from + " over to " + to + "; it is left as it was."
		}
		wroteApproved = true
	}
	if _, has := r.run(to); !has && !r.setRun(to, command) {
		if wroteApproved {
			r.deleteApproved(to)
		}
		return "Could not carry the run-at-login entry " + from + " over to " + to + "; it is left as it was."
	}
	r.deleteRun(from)
	if hasApproved {
		r.deleteApproved(from)
	}
	note := "Moved the run-at-login entry " + from + " to " + to + " (the program's new name)"
	if startupDisabled(approved) {
		note += "; it stays switched off in Task Manager"
	}
	return note + "."
}

//go:build darwin

package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/proc"
)

// prepareSystem gives a copy started at login the user's temporary folder: launchd starts programs without TMPDIR,
// yet that folder is where Discord, IINA and MPC-QT open their connections.
func prepareSystem() {
	if os.Getenv("TMPDIR") != "" {
		return
	}
	if out, err := exec.Command("/usr/bin/getconf", "DARWIN_USER_TEMP_DIR").Output(); err == nil {
		if d := strings.TrimSpace(string(out)); d != "" {
			_ = os.Setenv("TMPDIR", d)
		}
	}
}

// detach starts the copy that does the work and reports whether this one should leave. Opened from the Finder or the
// Dock, the program is "running" for macOS for as long as this process lives, and opening it again would only bring
// it to the front (which a program without windows of its own ignores). So this process hands over to a detached copy
// and leaves; opening the app again starts a new short-lived copy, which finds the running one and asks it for its
// window. Started at login (--background) or with MPCRPC_FOREGROUND set, the program simply runs.
func detach() bool {
	if background || serve || afterUpdate || os.Getenv("MPCRPC_FOREGROUND") != "" {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	c := proc.Detach(exec.Command(exe, append([]string{"--serve"}, os.Args[1:]...)...))
	if c.Start() != nil {
		return false // run here instead
	}
	_ = c.Process.Release()
	return true
}

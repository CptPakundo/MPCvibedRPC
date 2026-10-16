package winsys

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/proc"
)

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

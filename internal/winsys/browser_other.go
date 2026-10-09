//go:build !windows

package winsys

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// CloseWindowBrowser ends the browser that shows the settings window, including its helper processes. It only
// touches processes started with this program's own window profile (--user-data-dir=profileDir), never the user's
// regular browser, and returns how many it ended. When the window was opened in the default browser instead, there
// is nothing to end.
func CloseWindowBrowser(profileDir string) int {
	if profileDir == "" {
		return 0
	}
	n := 0
	for _, pid := range profilePids(run("ps", "-axww", "-o", "pid=,args=").stdout, profileDir, os.Getpid()) {
		if syscall.Kill(pid, syscall.SIGTERM) == nil {
			n++
		}
	}
	return n
}

// profilePids picks the processes of a "pid args" list that run with the window profile.
func profilePids(psOut, profileDir string, self int) []int {
	var out []int
	for _, l := range strings.Split(psOut, "\n") {
		pidText, args, ok := strings.Cut(strings.TrimSpace(l), " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid == self || pid <= 1 {
			continue
		}
		if usesProfile(strings.TrimSpace(args), profileDir) {
			out = append(out, pid)
		}
	}
	return out
}

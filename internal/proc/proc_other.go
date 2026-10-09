//go:build !windows

// Package proc starts helper programs the way a background app wants them: detached and without a console window.
package proc

import (
	"os/exec"
	"syscall"
)

func Hide(c *exec.Cmd) *exec.Cmd { return c }

func Detach(c *exec.Cmd) *exec.Cmd {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return c
}

func DetachVisible(c *exec.Cmd) *exec.Cmd { return Detach(c) }

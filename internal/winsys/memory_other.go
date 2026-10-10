//go:build !windows

package winsys

import "runtime/debug"

// TrimMemory hands the Go heap's free pages back to the system. Called when presence starts resting.
func TrimMemory() { debug.FreeOSMemory() }

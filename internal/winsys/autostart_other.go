//go:build !windows

package winsys

// registryRunValue is the Windows registry read (autostart_windows.go); there is no registry elsewhere.
func registryRunValue(string) (string, bool) { return "", false }

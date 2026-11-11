//go:build !windows

package winsys

// registryRunValue is the Windows registry read (autostart_windows.go); there is no registry elsewhere.
func registryRunValue(string) (string, bool) { return "", false }

// registryStartupApproved is the read of Windows' own startup switch (autostart_windows.go); not needed elsewhere.
func registryStartupApproved(string) ([]byte, bool) { return nil, false }

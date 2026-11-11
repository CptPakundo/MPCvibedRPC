package winsys

import (
	"syscall"
)

// runKeyPath is runKey below HKEY_CURRENT_USER.
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// approvedKeyPath is startupApprovedKey below HKEY_CURRENT_USER.
const approvedKeyPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`

// registryValue reads the value name of a key below HKEY_CURRENT_USER without starting a helper process. It gives the
// value's type and raw bytes; ok is false when the key or the value is missing.
func registryValue(keyPath, name string) (typ uint32, data []byte, ok bool) {
	key, err := syscall.UTF16PtrFromString(keyPath)
	if err != nil {
		return 0, nil, false
	}
	value, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, nil, false
	}
	var h syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, key, 0, syscall.KEY_READ, &h) != nil {
		return 0, nil, false
	}
	defer syscall.RegCloseKey(h)
	for try := 0; try < 3; try++ { // the value may grow between asking for its size and reading it
		var size uint32
		if syscall.RegQueryValueEx(h, value, nil, &typ, nil, &size) != nil {
			return 0, nil, false
		}
		if size == 0 {
			return typ, nil, true
		}
		buf := make([]byte, size+2) // two spare bytes: a string always ends in a terminator
		got := size
		err := syscall.RegQueryValueEx(h, value, nil, &typ, &buf[0], &got)
		if err == syscall.ERROR_MORE_DATA {
			continue
		}
		if err != nil {
			return 0, nil, false
		}
		return typ, buf[:got+2], true
	}
	return 0, nil, false
}

// registryRunValue reads the string value name of the Run key for the current user without starting a helper
// process (the settings window asks for it on every state request). ok is false when the key or the value is missing
// or is not a string.
func registryRunValue(name string) (data string, ok bool) {
	typ, raw, found := registryValue(runKeyPath, name)
	if !found || (typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ) {
		return "", false
	}
	if len(raw) == 0 {
		return "", true
	}
	buf := make([]uint16, len(raw)/2)
	for i := range buf {
		buf[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	return syscall.UTF16ToString(buf), true
}

// registryStartupApproved reads the value Windows keeps for name under StartupApproved\Run (Task Manager > Startup
// apps). ok is false when there is none, which means the entry is enabled.
func registryStartupApproved(name string) (data []byte, ok bool) {
	typ, raw, found := registryValue(approvedKeyPath, name)
	if !found || typ != syscall.REG_BINARY {
		return nil, false
	}
	return raw, true
}

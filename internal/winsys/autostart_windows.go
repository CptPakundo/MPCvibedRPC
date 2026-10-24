package winsys

import (
	"syscall"
	"unsafe"
)

// runKeyPath is runKey below HKEY_CURRENT_USER.
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// registryRunValue reads the string value name of the Run key for the current user without starting a helper
// process (the settings window asks for it on every state request). ok is false when the key or the value is missing
// or is not a string.
func registryRunValue(name string) (data string, ok bool) {
	key, err := syscall.UTF16PtrFromString(runKeyPath)
	if err != nil {
		return "", false
	}
	value, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", false
	}
	var h syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, key, 0, syscall.KEY_READ, &h) != nil {
		return "", false
	}
	defer syscall.RegCloseKey(h)
	for try := 0; try < 3; try++ { // the value may grow between asking for its size and reading it
		var typ, size uint32
		if syscall.RegQueryValueEx(h, value, nil, &typ, nil, &size) != nil {
			return "", false
		}
		if typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ {
			return "", false
		}
		if size == 0 {
			return "", true
		}
		buf := make([]uint16, (size+1)/2+1)
		got := size
		err := syscall.RegQueryValueEx(h, value, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &got)
		if err == syscall.ERROR_MORE_DATA {
			continue
		}
		if err != nil {
			return "", false
		}
		return syscall.UTF16ToString(buf), true
	}
	return "", false
}

package engine

import (
	"syscall"
	"unsafe"
)

var pGetVolumeInformation = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumeInformationW")

// volumeLabel is the label of the volume whose root is root ("E:\"), "" for any other folder or on failure.
func volumeLabel(root string) string {
	if len(root) != 3 || root[1] != ':' || (root[2] != '\\' && root[2] != '/') {
		return ""
	}
	p, err := syscall.UTF16PtrFromString(root[:2] + `\`)
	if err != nil {
		return ""
	}
	var buf [261]uint16
	r, _, _ := pGetVolumeInformation.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0, 0, 0, 0)
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:])
}

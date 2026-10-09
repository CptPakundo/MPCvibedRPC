//go:build windows

package winsys

import (
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	ntdll                      = syscall.NewLazyDLL("ntdll.dll")
	pNtQueryInformationProcess = ntdll.NewProc("NtQueryInformationProcess")
)

const (
	processTerminate               = 0x0001
	processQueryLimitedInformation = 0x1000
	processCommandLineInformation  = 60 // available since Windows 8.1
	statusInfoLengthMismatch       = 0xC0000004
	statusBufferTooSmall           = 0xC0000023
)

// browserExes are the programs OpenWindow may start the settings window with.
var browserExes = map[string]bool{"msedge.exe": true, "chrome.exe": true, "brave.exe": true, "vivaldi.exe": true}

// CloseWindowBrowser ends the browser that shows the settings window, including its helper processes. It only
// touches processes started with this program's own window profile (--user-data-dir=profileDir), never the
// user's regular browser. It returns how many processes it ended. Quitting the program calls it so that no
// browser is left running in the background, and the next start calls it to clear up after a crash.
func CloseWindowBrowser(profileDir string) int {
	if profileDir == "" {
		return 0
	}
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(snap)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	n := 0
	for err = syscall.Process32First(snap, &pe); err == nil; err = syscall.Process32Next(snap, &pe) {
		if !browserExes[strings.ToLower(syscall.UTF16ToString(pe.ExeFile[:]))] || int(pe.ProcessID) == os.Getpid() {
			continue
		}
		h, oerr := syscall.OpenProcess(processQueryLimitedInformation|processTerminate, false, pe.ProcessID)
		if oerr != nil {
			continue
		}
		if usesProfile(commandLine(h), profileDir) && syscall.TerminateProcess(h, 0) == nil {
			n++
		}
		syscall.CloseHandle(h)
	}
	return n
}

// commandLine reads the command line of a process ("" when it cannot be read).
func commandLine(h syscall.Handle) string {
	var need uint32
	r, _, _ := pNtQueryInformationProcess.Call(uintptr(h), processCommandLineInformation, 0, 0, uintptr(unsafe.Pointer(&need)))
	if st := uint32(r); (st != statusInfoLengthMismatch && st != statusBufferTooSmall) || need == 0 || need > 1<<20 {
		return ""
	}
	buf := make([]byte, need)
	r, _, _ = pNtQueryInformationProcess.Call(uintptr(h), processCommandLineInformation, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&need)))
	if uint32(r) != 0 || len(buf) < 16 {
		return ""
	}
	// the buffer starts with a UNICODE_STRING (Length, MaximumLength, padding, Buffer) whose text follows it
	length := int(*(*uint16)(unsafe.Pointer(&buf[0])))
	ptrAt := 4
	if unsafe.Sizeof(uintptr(0)) == 8 {
		ptrAt = 8
	}
	addr := *(*uintptr)(unsafe.Pointer(&buf[ptrAt]))
	base := uintptr(unsafe.Pointer(&buf[0]))
	if addr < base || addr+uintptr(length) > base+uintptr(len(buf)) {
		return ""
	}
	off := int(addr - base)
	u := make([]uint16, length/2)
	for i := range u {
		u[i] = uint16(buf[off+2*i]) | uint16(buf[off+2*i+1])<<8
	}
	return string(utf16.Decode(u))
}

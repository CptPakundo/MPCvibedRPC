package capture

import (
	"syscall"
	"time"
	"unsafe"
)

// Discord's threads are read with what Windows offers every program about another program of the same user: its
// threads, and for each its name and when it began. Nothing is read from Discord's memory, and nothing is written.
var (
	ntdll                 = syscall.NewLazyDLL("ntdll.dll")
	pNtGetNextThread      = ntdll.NewProc("NtGetNextThread")         // walks one process's threads
	pGetThreadDescription = kernel32.NewProc("GetThreadDescription") // Windows 10 of 2016 and later
	pGetThreadTimes       = kernel32.NewProc("GetThreadTimes")
	pLocalFree            = kernel32.NewProc("LocalFree")
)

const (
	processQuery       = 0x0400 // PROCESS_QUERY_INFORMATION: to walk the threads
	threadQueryLimited = 0x0800 // THREAD_QUERY_LIMITED_INFORMATION: the name and the times, nothing else
	maxThreadName      = 128    // UTF-16 units read of a name; Discord's are far shorter
	maxThreads         = 8192   // of one process; the walk ends long before
)

// threadsSupported reports whether this Windows gives threads names and lets a process's threads be walked.
func threadsSupported() bool {
	return pGetThreadDescription.Find() == nil && pNtGetNextThread.Find() == nil
}

// discordProcesses lists the running processes of the Discord desktop app, by process ID.
func discordProcesses() map[uint32]bool {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(snap)
	pids := map[uint32]bool{}
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = syscall.Process32First(snap, &pe); err == nil; err = syscall.Process32Next(snap, &pe) {
		if isDiscord(syscall.UTF16ToString(pe.ExeFile[:])) {
			pids[pe.ProcessID] = true
		}
	}
	return pids
}

// readThreads lists the named threads of the given processes (none: nothing is looked at). Only these processes'
// threads are walked: listing every thread on the computer takes many times as long.
func readThreads(pids map[uint32]bool) []thread {
	if len(pids) == 0 || !threadsSupported() {
		return nil
	}
	var out []thread
	for pid := range pids {
		process, err := syscall.OpenProcess(processQuery, false, pid)
		if err != nil {
			continue // it ended meanwhile, or is not this user's
		}
		var cur uintptr // the thread the walk is at; its handle is closed when the walk moves on
		for i := 0; i < maxThreads; i++ {
			var next uintptr
			status, _, _ := pNtGetNextThread.Call(uintptr(process), cur, threadQueryLimited, 0, 0, uintptr(unsafe.Pointer(&next)))
			if cur != 0 {
				syscall.CloseHandle(syscall.Handle(cur))
			}
			cur = next
			if status != 0 || cur == 0 {
				cur = 0
				break // no more threads
			}
			if t, ok := readThread(cur); ok {
				out = append(out, t)
			}
		}
		if cur != 0 {
			syscall.CloseHandle(syscall.Handle(cur))
		}
		syscall.CloseHandle(process)
	}
	return out
}

// readThread reads the name and the start of the thread behind h; false when it has no name.
func readThread(h uintptr) (thread, bool) {
	var desc *uint16 // Windows allocates the name; it is freed below
	if hr, _, _ := pGetThreadDescription.Call(h, uintptr(unsafe.Pointer(&desc))); int32(hr) < 0 || desc == nil {
		return thread{}, false
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(desc)))
	name := make([]uint16, 0, 32)
	for i := 0; i < maxThreadName; i++ {
		c := *(*uint16)(unsafe.Add(unsafe.Pointer(desc), i*2))
		if c == 0 {
			break
		}
		name = append(name, c)
	}
	if len(name) == 0 {
		return thread{}, false
	}
	t := thread{name: syscall.UTF16ToString(name)}
	var created, exited, kernel, user syscall.Filetime
	if ok, _, _ := pGetThreadTimes.Call(h, uintptr(unsafe.Pointer(&created)), uintptr(unsafe.Pointer(&exited)),
		uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); ok != 0 {
		t.start = time.Unix(0, created.Nanoseconds())
	}
	return t, true
}

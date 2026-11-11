package capture

import (
	"context"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// storePath is where Windows keeps the capture record of desktop programs, below HKEY_CURRENT_USER: one key per
// program file (its path with # for the separators), with the values LastUsedTimeStart and LastUsedTimeStop.
const storePath = `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\graphicsCaptureProgrammatic\NonPackaged`

// Supported reports whether this system can tell: it names threads, or it keeps the record where it can be read.
func Supported() bool { return threadsSupported() || recordExists() }

// recordExists reports whether Windows' record of screen captures is there.
func recordExists() bool {
	h, ok := open(storePath)
	if ok {
		syscall.RegCloseKey(h)
	}
	return ok
}

// DiscordSharing reports whether a Discord desktop app is sharing the screen right now: by its threads, or else by
// Windows' record.
func DiscordSharing() bool {
	now := time.Now()
	if streaming(readThreads(discordProcesses()), now) {
		return true
	}
	return sharing(read(), now, func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

func open(path string) (syscall.Handle, bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var h syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, p, 0, syscall.KEY_READ, &h) != nil {
		return 0, false
	}
	return h, true
}

// read lists the records of the Discord programs (the other programs' records are not opened).
func read() []entry {
	h, ok := open(storePath)
	if !ok {
		return nil
	}
	defer syscall.RegCloseKey(h)
	var out []entry
	name := make([]uint16, 1024)
	for i := uint32(0); i < 4096; i++ { // far more programs than a computer has; the loop ends at the last key
		n := uint32(len(name))
		if syscall.RegEnumKeyEx(h, i, &name[0], &n, nil, nil, nil, nil) != nil {
			break
		}
		key := syscall.UTF16ToString(name[:n])
		path := strings.ReplaceAll(key, "#", `\`)
		if !isDiscord(path) {
			continue
		}
		sub, ok := open(storePath + `\` + key)
		if !ok {
			continue
		}
		out = append(out, entry{path: path, start: fileTime(sub, "LastUsedTimeStart"), stop: fileTime(sub, "LastUsedTimeStop")})
		syscall.RegCloseKey(sub)
	}
	return out
}

// fileTime reads a time Windows stores as a 64-bit count of 100 ns steps since 1601 (zero: not set).
func fileTime(h syscall.Handle, value string) time.Time {
	v, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return time.Time{}
	}
	var typ uint32
	var raw uint64
	size := uint32(unsafe.Sizeof(raw))
	if syscall.RegQueryValueEx(h, v, nil, &typ, (*byte)(unsafe.Pointer(&raw)), &size) != nil || size != 8 || raw == 0 {
		return time.Time{}
	}
	ft := syscall.Filetime{LowDateTime: uint32(raw), HighDateTime: uint32(raw >> 32)}
	return time.Unix(0, ft.Nanoseconds())
}

var (
	advapi32          = syscall.NewLazyDLL("advapi32.dll")
	pRegNotifyChange  = advapi32.NewProc("RegNotifyChangeKeyValue")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	pCreateEvent      = kernel32.NewProc("CreateEventW")
	notifyNameAndData = uintptr(0x1 | 0x4) // REG_NOTIFY_CHANGE_NAME | REG_NOTIFY_CHANGE_LAST_SET
)

// threadLook is how often Watch looks at Discord's threads. No look is taken while no Discord runs.
const threadLook = 2 * time.Second

// Watch calls onChange when a share may have begun or ended, until ctx ends, so the status follows a stream at once
// instead of at the next check. It watches both signs: Discord's threads, looked at every threadLook, and Windows'
// record.
func Watch(ctx context.Context, onChange func()) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); watchThreads(ctx, onChange) }()
	go func() { defer wg.Done(); watchRecord(ctx, onChange) }()
	wg.Wait()
}

// watchThreads calls onChange when what Discord's threads say changes. It returns at once where threads have no names.
func watchThreads(ctx context.Context, onChange func()) {
	if !threadsSupported() {
		return
	}
	tick := time.NewTicker(threadLook)
	defer tick.Stop()
	was := streaming(readThreads(discordProcesses()), time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if is := streaming(readThreads(discordProcesses()), time.Now()); is != was {
			was = is
			onChange()
		}
	}
}

// watchRecord calls onChange whenever Windows writes to the capture records (a capture began or ended). It calls it a
// second time once minLive has passed, because a capture only counts from then. It returns at once where there is no
// record to watch.
func watchRecord(ctx context.Context, onChange func()) {
	h, ok := open(storePath)
	if !ok {
		return
	}
	defer syscall.RegCloseKey(h)
	ev, _, _ := pCreateEvent.Call(0, 0, 0, 0) // auto-reset, not signalled
	if ev == 0 {
		return
	}
	event := syscall.Handle(ev)
	defer syscall.CloseHandle(event)
	for ctx.Err() == nil {
		// asks for one notification of a change in the key or below it; the event is set when it comes
		if r, _, _ := pRegNotifyChange.Call(uintptr(h), 1, notifyNameAndData, ev, 1); r != 0 {
			return
		}
		for {
			s, err := syscall.WaitForSingleObject(event, 1000) // wakes every second to see whether ctx ended
			if err != nil || ctx.Err() != nil {
				return
			}
			if s == syscall.WAIT_OBJECT_0 {
				break
			}
		}
		onChange()
		time.AfterFunc(minLive+500*time.Millisecond, func() {
			if ctx.Err() == nil {
				onChange()
			}
		})
	}
}

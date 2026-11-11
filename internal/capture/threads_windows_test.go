package capture

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The reading itself is tried on this test's own process, never on a Discord: a thread is given a name, and the list
// of this process's threads must hold it, with a start that is neither missing nor in the future.
func TestReadsThreadNames(t *testing.T) {
	if !threadsSupported() {
		t.Skip("this Windows does not name threads")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const name = "SampleCaptureThread"
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	current, _, _ := kernel32.NewProc("GetCurrentThread").Call()
	if hr, _, _ := kernel32.NewProc("SetThreadDescription").Call(current, uintptr(unsafe.Pointer(p))); int32(hr) < 0 {
		t.Fatalf("the thread could not be named: %#x", hr)
	}
	began := time.Now()
	threads := readThreads(map[uint32]bool{uint32(os.Getpid()): true})
	t.Logf("%d named threads read in %v", len(threads), time.Since(began))
	for _, th := range threads {
		if th.name != name {
			continue
		}
		if th.start.IsZero() || th.start.After(time.Now()) {
			t.Errorf("the thread's start is %v", th.start)
		}
		return
	}
	t.Errorf("the named thread is not among the %d read", len(threads))
}

// Other processes' threads are not read, and no process means no look at all.
func TestReadsOnlyTheGivenProcesses(t *testing.T) {
	if got := readThreads(nil); got != nil {
		t.Errorf("threads read without a process: %v", got)
	}
	if got := readThreads(map[uint32]bool{0xFFFFFFF0: true}); len(got) != 0 { // no process has this ID
		t.Errorf("threads read of a process that is not there: %v", got)
	}
}

// What the whole look costs on this computer, Discord or not: it runs every two seconds while the watch is on.
func TestSharingLookIsCheap(t *testing.T) {
	began := time.Now()
	on := DiscordSharing()
	took := time.Since(began)
	t.Logf("sharing=%v in %v (Discord processes: %d)", on, took, len(discordProcesses()))
	if took > 2*time.Second {
		t.Errorf("one look took %v", took)
	}
}

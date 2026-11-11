//go:build windows

package pipe

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// A server makes its next instance of a pipe only after it has accepted a client. A second client that comes in that
// moment must wait for it (as mpv's pipe has it when the program's check and its event watch connect together).
func TestDialWaitsForAFreeInstance(t *testing.T) {
	path := `\\?\pipe\mpcvibedrpc-test-busy-` + strconv.Itoa(os.Getpid())
	first := listenPipe(t, path)
	defer first()
	a, err := Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	stop := make(chan func(), 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		stop <- listenPipe(t, path)
	}()
	start := time.Now()
	b, err := Dial(path) // every instance has a client now
	if err != nil {
		t.Fatalf("the second client did not get in: %v", err)
	}
	b.Close()
	if took := time.Since(start); took < 100*time.Millisecond {
		t.Errorf("the second client got in after %v: the pipe was not busy", took)
	}
	(<-stop)()
}

// With no instance coming, the wait ends after busyWait with the error of CreateFile.
func TestDialGivesUpOnABusyPipe(t *testing.T) {
	path := `\\?\pipe\mpcvibedrpc-test-full-` + strconv.Itoa(os.Getpid())
	defer listenPipe(t, path)()
	a, err := Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	start := time.Now()
	if b, err := Dial(path); err == nil {
		b.Close()
		t.Fatal("a pipe with no free instance let a second client in")
	}
	if took := time.Since(start); took < busyWait/2 || took > busyWait+2*time.Second {
		t.Errorf("gave up after %v; want about %v", took, busyWait)
	}
}

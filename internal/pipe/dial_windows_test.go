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
	// the wait is long, so that a slow machine cannot make the second instance come too late; the test ends when it does
	old := busyWait
	busyWait = 10 * time.Second
	t.Cleanup(func() { busyWait = old })

	path := `\\?\pipe\mpcvibedrpc-test-busy-` + strconv.Itoa(os.Getpid())
	defer listenPipe(t, path)()
	a, err := Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	type result struct {
		err  error
		took time.Duration
		at   time.Time // when the dial ended
	}
	started, done := make(chan struct{}), make(chan result, 1)
	go func() {
		start := time.Now()
		close(started)
		b, err := Dial(path) // every instance has a client now
		if err == nil {
			b.Close()
		}
		done <- result{err, time.Since(start), time.Now()}
	}()
	<-started
	time.Sleep(200 * time.Millisecond) // the server is slow to make its next instance
	listening := time.Now() // before the instance exists: a dial that ended earlier got in without waiting
	defer listenPipe(t, path)()
	r := <-done
	if r.err != nil {
		t.Fatalf("the second client did not get in: %v", r.err)
	}
	// Not a timing: a loaded machine may start the helper goroutine late, but a client that got in before the
	// second instance existed found the pipe free.
	if r.at.Before(listening) {
		t.Errorf("the second client got in after %v, before a free instance existed: the pipe was not busy", r.took)
	}
}

// With no instance coming, the wait ends after busyWait with the error of CreateFile.
func TestDialGivesUpOnABusyPipe(t *testing.T) {
	old := busyWait
	busyWait = 300 * time.Millisecond
	t.Cleanup(func() { busyWait = old })

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
	if took := time.Since(start); took < busyWait {
		t.Errorf("gave up after %v, before the wait of %v was over", took, busyWait)
	}
}

//go:build windows

package engine

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// The fake Discord on Windows is a real named pipe, like Discord's own, so the engine tests exercise the same
// dialing and reading code the program uses.

var (
	k32                  = syscall.NewLazyDLL("kernel32.dll")
	pCreateNamedPipe     = k32.NewProc("CreateNamedPipeW")
	pConnectNamedPipe    = k32.NewProc("ConnectNamedPipe")
	pDisconnectNamedPipe = k32.NewProc("DisconnectNamedPipe")
	pPeekNamedPipe       = k32.NewProc("PeekNamedPipe")
	pipeCounter          int
	pipeCounterMu        sync.Mutex
)

const (
	pipeAccessDuplex    = 0x00000003
	pipeUnlimited       = 255
	errorPipeConnected  = syscall.Errno(535)
	invalidHandleValue  = ^uintptr(0)
	pipeBufferBytes     = 64 * 1024
	pipePollMaxInterval = 10 * time.Millisecond
)

// fakePath is a unique pipe name for one test.
func fakePath(string) string {
	pipeCounterMu.Lock()
	defer pipeCounterMu.Unlock()
	pipeCounter++
	return fmt.Sprintf(`\\.\pipe\mpcvibedrpc-test-%d-%d`, os.Getpid(), pipeCounter)
}

type pipeListener struct {
	path    string
	name    *uint16
	mu      sync.Mutex
	closed  bool
	waiting bool // an instance is waiting for a client
}

// listenFake starts the fake Discord's listener. Each Accept creates a pipe instance and waits for a client; the
// engine simply retries until one is there.
func listenFake(path string) (fakeListener, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	return &pipeListener{path: path, name: name}, nil
}

func (l *pipeListener) create() (syscall.Handle, error) {
	r, _, err := pCreateNamedPipe.Call(uintptr(unsafe.Pointer(l.name)), pipeAccessDuplex, 0, pipeUnlimited, pipeBufferBytes, pipeBufferBytes, 0, 0)
	if r == invalidHandleValue {
		return 0, err
	}
	return syscall.Handle(r), nil
}

func (l *pipeListener) Accept() (io.ReadWriteCloser, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, net.ErrClosed
	}
	l.mu.Unlock()
	h, err := l.create()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.waiting = true
	l.mu.Unlock()
	r, _, cerr := pConnectNamedPipe.Call(uintptr(h), 0)
	l.mu.Lock()
	l.waiting = false
	closed := l.closed
	l.mu.Unlock()
	if closed || (r == 0 && !errors.Is(cerr, errorPipeConnected)) {
		syscall.CloseHandle(h)
		return nil, net.ErrClosed
	}
	return &serverPipe{h: h}, nil
}

// Close stops accepting. A client connecting wakes the Accept that is blocked in ConnectNamedPipe.
func (l *pipeListener) Close() error {
	l.mu.Lock()
	l.closed = true
	waiting := l.waiting
	l.mu.Unlock()
	if waiting {
		if h, err := syscall.CreateFile(l.name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, 0, 0); err == nil {
			syscall.CloseHandle(h)
		}
	}
	return nil
}

// serverPipe is the server end of one connection. Like the client it never blocks inside ReadFile (a pending
// synchronous read would stop the same handle from writing), it polls PeekNamedPipe instead.
type serverPipe struct {
	h      syscall.Handle
	mu     sync.Mutex
	closed bool
}

func (p *serverPipe) isClosed() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.closed }

func (p *serverPipe) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	delay := time.Millisecond
	for {
		if p.isClosed() {
			return 0, io.EOF
		}
		var avail uint32
		r, _, _ := pPeekNamedPipe.Call(uintptr(p.h), 0, 0, 0, uintptr(unsafe.Pointer(&avail)), 0)
		if r == 0 {
			return 0, io.EOF // the client went away
		}
		if avail > 0 {
			if uint32(len(b)) > avail {
				b = b[:avail]
			}
			var n uint32
			if err := syscall.ReadFile(p.h, b, &n, nil); err != nil {
				return int(n), io.EOF
			}
			return int(n), nil
		}
		time.Sleep(delay)
		if delay < pipePollMaxInterval {
			delay *= 2
		}
	}
}

func (p *serverPipe) Write(b []byte) (int, error) {
	if p.isClosed() {
		return 0, errors.New("pipe closed")
	}
	total := 0
	for total < len(b) {
		var n uint32
		if err := syscall.WriteFile(p.h, b[total:], &n, nil); err != nil {
			return total, err
		}
		total += int(n)
	}
	return total, nil
}

func (p *serverPipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	pDisconnectNamedPipe.Call(uintptr(p.h))
	return syscall.CloseHandle(p.h)
}

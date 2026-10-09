//go:build windows

package pipe

import (
	"errors"
	"io"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procPeekNamedPipe = kernel32.NewProc("PeekNamedPipe")
)

// pipeConn reads a named pipe without ever blocking inside ReadFile: a synchronous handle serialises reads and
// writes, so a pending read would stop us from answering the other side. Instead we peek and only read what is there.
// Closing the connection also ends a Read that is waiting for data.
type pipeConn struct {
	h      syscall.Handle
	mu     sync.Mutex
	closed bool
}

// Dial opens a named pipe (\\.\pipe\name).
func Dial(path string) (io.ReadWriteCloser, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return &pipeConn{h: h}, nil
}

func (p *pipeConn) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *pipeConn) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	delay := time.Millisecond
	for {
		if p.isClosed() {
			return 0, io.EOF
		}
		var avail uint32
		r, _, _ := procPeekNamedPipe.Call(uintptr(p.h), 0, 0, 0, uintptr(unsafe.Pointer(&avail)), 0)
		if r == 0 {
			return 0, io.EOF // broken pipe: the other side went away
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
		if delay < 20*time.Millisecond {
			delay *= 2
		}
	}
}

func (p *pipeConn) Write(b []byte) (int, error) {
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

func (p *pipeConn) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return syscall.CloseHandle(p.h)
}

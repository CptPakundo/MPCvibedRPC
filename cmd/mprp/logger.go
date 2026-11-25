package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// logger writes to the log file (kept small) and keeps the last lines for the window.
type logger struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64 // of the file, so it is moved to .old once it passes logLimit (also in a copy that runs for weeks)
	ring []string
}

const logLimit = 1024 * 1024

func newLogger(path string) *logger {
	l := &logger{path: path}
	l.open()
	return l
}

// open opens the log file, first moving a full one to .old (l.mu held, or not shared yet).
func (l *logger) open() {
	if st, err := os.Stat(l.path); err == nil && st.Size() > logLimit {
		_ = os.Rename(l.path, l.path+".old")
	}
	l.f, _ = os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	l.size = 0
	if l.f != nil {
		if st, err := l.f.Stat(); err == nil {
			l.size = st.Size()
		}
	}
}

func (l *logger) log(level, msg string) {
	line := fmt.Sprintf("[%s] %s %s", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), level, msg)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ring = append(l.ring, line)
	if len(l.ring) > 300 {
		l.ring = l.ring[len(l.ring)-300:]
	}
	if l.f != nil {
		n, _ := fmt.Fprintln(l.f, line)
		if l.size += int64(n); l.size > logLimit {
			l.f.Close()
			l.open()
		}
	}
}

func (l *logger) last(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.ring) < n {
		n = len(l.ring)
	}
	return append([]string{}, l.ring[len(l.ring)-n:]...)
}

func (l *logger) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}

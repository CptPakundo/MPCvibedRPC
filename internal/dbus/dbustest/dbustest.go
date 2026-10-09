// Package dbustest is a fake session bus for tests: it accepts the login, answers Hello and ListNames itself and
// hands every other method call to the test.
package dbustest

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
)

// Reply is what a handler answers: values with their signature, or an error name.
type Reply struct {
	Sig   string
	Body  []any
	Error string
}

// Handler answers a method call sent to one bus name.
type Handler func(m *dbus.Message) Reply

// Bus is the fake bus.
type Bus struct {
	Address string

	mu       sync.Mutex
	names    map[string]Handler
	calls    []string
	ln       net.Listener
	conns    []net.Conn
	unsorted []string
}

// New starts a fake bus on a Unix socket in the test's folder. It skips the test when Unix sockets are not available.
func New(t *testing.T) *Bus {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "darwin" { // a socket path may only be 104 bytes long there, and the temporary folder is long
		var err error
		if dir, err = os.MkdirTemp("/tmp", "bus"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	path := filepath.Join(dir, "bus")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Skip("no Unix sockets here: " + err.Error())
	}
	b := &Bus{Address: "unix:path=" + path, names: map[string]Handler{}, ln: ln}
	t.Cleanup(b.Close)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			b.mu.Lock()
			b.conns = append(b.conns, c)
			b.mu.Unlock()
			go b.serve(c)
		}
	}()
	return b
}

// Own puts a name on the bus, answered by h (nil removes it).
func (b *Bus) Own(name string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if h == nil {
		delete(b.names, name)
		for i, n := range b.unsorted {
			if n == name {
				b.unsorted = append(b.unsorted[:i], b.unsorted[i+1:]...)
				break
			}
		}
		return
	}
	if _, ok := b.names[name]; !ok {
		b.unsorted = append(b.unsorted, name)
	}
	b.names[name] = h
}

// Calls lists the calls made so far, as "destination interface.member".
func (b *Bus) Calls() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string{}, b.calls...)
}

// Close stops the bus and drops every connection.
func (b *Bus) Close() {
	b.ln.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.conns {
		c.Close()
	}
}

func (b *Bus) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	if nul, err := r.ReadByte(); err != nil || nul != 0 {
		return
	}
	for { // the text part of the login, up to BEGIN
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		switch {
		case strings.HasPrefix(line, "AUTH EXTERNAL"):
			c.Write([]byte("OK 0123456789abcdef0123456789abcdef\r\n"))
		case strings.HasPrefix(line, "BEGIN"):
			goto messages
		default:
			c.Write([]byte("ERROR\r\n"))
		}
	}
messages:
	serial := uint32(1000)
	for {
		m, err := dbus.ReadMessage(r)
		if err != nil {
			return
		}
		if m.Type != dbus.TypeMethodCall {
			continue
		}
		var rep Reply
		b.mu.Lock()
		b.calls = append(b.calls, m.Destination+" "+m.Interface+"."+m.Member)
		h := b.names[m.Destination]
		names := append([]string{"org.freedesktop.DBus", ":1.1"}, b.unsorted...)
		b.mu.Unlock()
		switch {
		case m.Destination == "org.freedesktop.DBus" && m.Member == "Hello":
			rep = Reply{Sig: "s", Body: []any{":1.1"}}
			// a real bus also sends NameAcquired before the answer: the client must skip it
			serial++
			sig, _ := (&dbus.Message{Type: dbus.TypeSignal, Serial: serial, Path: "/org/freedesktop/DBus", Interface: "org.freedesktop.DBus",
				Member: "NameAcquired", Destination: ":1.1", Sender: "org.freedesktop.DBus", Signature: "s", Body: []any{":1.1"}}).Marshal()
			c.Write(sig)
		case m.Destination == "org.freedesktop.DBus" && m.Member == "ListNames":
			rep = Reply{Sig: "as", Body: []any{names}}
		case h != nil:
			rep = h(m)
		default:
			rep = Reply{Error: "org.freedesktop.DBus.Error.ServiceUnknown"}
		}
		serial++
		out := &dbus.Message{Type: dbus.TypeMethodReturn, Serial: serial, ReplySerial: m.Serial, Destination: ":1.1",
			Sender: m.Destination, Signature: rep.Sig, Body: rep.Body}
		if rep.Error != "" {
			out.Type, out.ErrorName, out.Signature, out.Body = dbus.TypeError, rep.Error, "s", []any{"fake error"}
		}
		raw, err := out.Marshal()
		if err != nil {
			panic(err)
		}
		c.Write(raw)
	}
}

// Strings turns a []string into the []any the encoder takes for "as".
func Strings(s ...string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

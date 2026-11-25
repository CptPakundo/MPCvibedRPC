// Package dbus is a minimal D-Bus client: enough to call methods on the session bus and read their answers. It is
// used on Linux to ask video players what they play through MPRIS (see package mpris). Only Unix-socket buses and the
// EXTERNAL login (the one every session bus offers to its own user) are supported.
package dbus

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message types.
const (
	TypeMethodCall   = 1
	TypeMethodReturn = 2
	TypeError        = 3
	TypeSignal       = 4
)

// FlagNoAutoStart stops the bus from starting a program to answer a call: asking about a player must never launch it.
const FlagNoAutoStart = 0x2

// SessionAddress is where the user's session bus is: $DBUS_SESSION_BUS_ADDRESS, or the usual socket in
// $XDG_RUNTIME_DIR. Empty when there is neither.
func SessionAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		p := filepath.Join(d, "bus")
		if _, err := os.Stat(p); err == nil {
			return "unix:path=" + p
		}
	}
	return ""
}

// socketPaths turns a bus address ("unix:path=/run/user/1000/bus;unix:abstract=/tmp/dbus-x") into socket paths that
// net.Dial understands (an abstract socket starts with "@"). Transports other than unix are skipped.
func socketPaths(address string) []string {
	var out []string
	for _, entry := range strings.Split(address, ";") {
		transport, params, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok || transport != "unix" {
			continue
		}
		for _, kv := range strings.Split(params, ",") {
			k, v, _ := strings.Cut(kv, "=")
			v = unescape(v)
			switch k {
			case "path":
				out = append(out, v)
			case "abstract":
				out = append(out, "@"+v)
			}
		}
	}
	return out
}

// unescape decodes the %xx escapes of an address value.
func unescape(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(n))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Conn is a connection to a bus. Calls are made one at a time, unless Receive reads the connection in the background.
type Conn struct {
	mu      sync.Mutex
	c       net.Conn
	r       *bufio.Reader
	serial  uint32
	replies map[uint32]chan *Message // while Receive reads: the calls waiting for their answer
	ended   error                    // why Receive's reading stopped (the connection is then of no more use)
}

// ErrEnded is returned by a call on a connection whose reading (Receive) has stopped.
var ErrEnded = errors.New("dbus: the connection has ended")

// ErrNoAnswer is the error (wrapped) of a call made while Receive reads the connection that got no answer in time. The
// connection itself may still be fine: the other side may only be slow.
var ErrNoAnswer = errors.New("dbus: no answer")

// Dial connects to the bus at address, logs in and says Hello. each step (connecting, logging in, Hello) gets the timeout.
func Dial(address string, timeout time.Duration) (*Conn, error) {
	paths := socketPaths(address)
	if len(paths) == 0 {
		return nil, errors.New("dbus: no usable bus address")
	}
	var lastErr error
	for _, p := range paths {
		c, err := net.DialTimeout("unix", p, timeout)
		if err != nil {
			lastErr = err
			continue
		}
		conn := &Conn{c: c, r: bufio.NewReader(c)}
		if err := conn.login(timeout); err != nil {
			c.Close()
			lastErr = err
			continue
		}
		if _, err := conn.Call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "Hello", timeout, ""); err != nil {
			c.Close()
			lastErr = err
			continue
		}
		return conn, nil
	}
	return nil, lastErr
}

// login runs the EXTERNAL authentication: the bus checks our user id against the socket's peer credentials.
func (c *Conn) login(timeout time.Duration) error {
	_ = c.c.SetDeadline(time.Now().Add(timeout))
	defer c.c.SetDeadline(time.Time{})
	uid := strconv.Itoa(os.Getuid())
	if _, err := io.WriteString(c.c, "\x00AUTH EXTERNAL "+hex.EncodeToString([]byte(uid))+"\r\n"); err != nil {
		return err
	}
	line, err := c.r.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "OK ") {
		return fmt.Errorf("dbus: login refused (%s)", strings.TrimSpace(line))
	}
	_, err = io.WriteString(c.c, "BEGIN\r\n")
	return err
}

// Close ends the connection.
func (c *Conn) Close() error { return c.c.Close() }

// Watch asks the bus for the signals that match rule (AddMatch, such as "type='signal',path='/a/b'") and calls on for
// each one until the connection ends. The connection is then used for nothing else.
func (c *Conn) Watch(rule string, timeout time.Duration, on func(*Message)) error {
	if _, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "AddMatch", timeout, "s", rule); err != nil {
		return err
	}
	for {
		m, err := ReadMessage(c.r)
		if err != nil {
			return err
		}
		if m.Type == TypeSignal {
			on(m)
		}
	}
}

// Error is an error reply.
type Error struct {
	Name    string
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

// Call calls a method and returns the values of its answer. sig is the signature of args ("" for none). The bus does
// not start a program to answer it (FlagNoAutoStart).
func (c *Conn) Call(dest, path, iface, member string, timeout time.Duration, sig string, args ...any) ([]any, error) {
	return c.call(FlagNoAutoStart, dest, path, iface, member, timeout, sig, args)
}

// CallStart is Call for a service that the bus may start to answer (a notification service started on demand).
func (c *Conn) CallStart(dest, path, iface, member string, timeout time.Duration, sig string, args ...any) ([]any, error) {
	return c.call(0, dest, path, iface, member, timeout, sig, args)
}

// Receive reads the connection in the background from now on: answers go to the calls that wait for them, and
// on is called with each signal (on the reading goroutine: it must not block, nor make calls itself). Calls may then
// be made from several goroutines at once. When the connection ends, so does the reading; calls then fail with
// ErrEnded.
func (c *Conn) Receive(on func(*Message)) {
	c.mu.Lock()
	c.replies = map[uint32]chan *Message{}
	c.mu.Unlock()
	go func() {
		for {
			m, err := ReadMessage(c.r)
			if err != nil {
				c.mu.Lock()
				c.ended = err
				for s, ch := range c.replies {
					close(ch)
					delete(c.replies, s)
				}
				c.mu.Unlock()
				return
			}
			switch m.Type {
			case TypeSignal:
				on(m)
			case TypeMethodReturn, TypeError:
				c.mu.Lock()
				ch := c.replies[m.ReplySerial]
				delete(c.replies, m.ReplySerial)
				c.mu.Unlock()
				if ch != nil {
					ch <- m // buffered: never blocks
				}
			}
		}
	}()
}

func (c *Conn) call(flags byte, dest, path, iface, member string, timeout time.Duration, sig string, args []any) ([]any, error) {
	c.mu.Lock()
	if c.replies != nil {
		return c.callReceiving(flags, dest, path, iface, member, timeout, sig, args) // unlocks c.mu
	}
	defer c.mu.Unlock()
	c.serial++
	m := &Message{Type: TypeMethodCall, Flags: flags, Serial: c.serial, Path: path, Interface: iface,
		Member: member, Destination: dest, Signature: sig, Body: args}
	b, err := m.Marshal()
	if err != nil {
		return nil, err
	}
	_ = c.c.SetDeadline(time.Now().Add(timeout))
	defer c.c.SetDeadline(time.Time{})
	if _, err := c.c.Write(b); err != nil {
		return nil, err
	}
	for {
		r, err := ReadMessage(c.r)
		if err != nil {
			return nil, err
		}
		if r.ReplySerial != m.Serial || (r.Type != TypeMethodReturn && r.Type != TypeError) {
			continue // a signal (NameAcquired, ...) or someone else's answer
		}
		return answer(r)
	}
}

// callReceiving makes a call while Receive reads the connection: it waits for the answer the reader hands over.
// c.mu is held on entry and released here.
func (c *Conn) callReceiving(flags byte, dest, path, iface, member string, timeout time.Duration, sig string, args []any) ([]any, error) {
	if c.ended != nil {
		c.mu.Unlock()
		return nil, ErrEnded
	}
	c.serial++
	m := &Message{Type: TypeMethodCall, Flags: flags, Serial: c.serial, Path: path, Interface: iface,
		Member: member, Destination: dest, Signature: sig, Body: args}
	b, err := m.Marshal()
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	ch := make(chan *Message, 1)
	c.replies[m.Serial] = ch
	_ = c.c.SetWriteDeadline(time.Now().Add(timeout))
	_, err = c.c.Write(b)
	_ = c.c.SetWriteDeadline(time.Time{})
	if err != nil {
		delete(c.replies, m.Serial)
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r, ok := <-ch:
		if !ok {
			return nil, ErrEnded
		}
		return answer(r)
	case <-t.C:
		c.mu.Lock()
		delete(c.replies, m.Serial)
		c.mu.Unlock()
		return nil, fmt.Errorf("%w to %s.%s within %v", ErrNoAnswer, iface, member, timeout)
	}
}

// answer turns a method return into its values and an error reply into an *Error.
func answer(r *Message) ([]any, error) {
	if r.Type == TypeError {
		e := &Error{Name: r.ErrorName}
		if len(r.Body) > 0 {
			e.Message, _ = r.Body[0].(string)
		}
		return nil, e
	}
	return r.Body, nil
}

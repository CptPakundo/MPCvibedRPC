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

// Conn is a connection to a bus. Calls are made one at a time.
type Conn struct {
	mu     sync.Mutex
	c      net.Conn
	r      *bufio.Reader
	serial uint32
}

// Dial connects to the bus at address, logs in and says Hello. timeout bounds the whole of it.
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

// Call calls a method and returns the values of its answer. sig is the signature of args ("" for none).
func (c *Conn) Call(dest, path, iface, member string, timeout time.Duration, sig string, args ...any) ([]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.serial++
	m := &Message{Type: TypeMethodCall, Flags: FlagNoAutoStart, Serial: c.serial, Path: path, Interface: iface,
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
		if r.Type == TypeError {
			e := &Error{Name: r.ErrorName}
			if len(r.Body) > 0 {
				e.Message, _ = r.Body[0].(string)
			}
			return nil, e
		}
		return r.Body, nil
	}
}

// Package discord is a minimal Discord Rich Presence client for the local IPC socket (a Unix socket on Linux/macOS,
// a named pipe on Windows). Frames are: uint32 opcode, uint32 length (little-endian), then JSON.
package discord

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Opcodes of the IPC protocol.
const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
	opPing      = 3
	opPong      = 4
)

// Error is an ERROR event answered by Discord.
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

// SocketPaths lists where Discord's IPC endpoint may be, in the order they are tried.
func SocketPaths() []string {
	return socketPaths(os.Getenv, runtime.GOOS)
}

func socketPaths(env func(string) string, goos string) []string {
	var out []string
	for i := 0; i < 10; i++ {
		if goos == "windows" {
			out = append(out, `\\?\pipe\discord-ipc-`+strconv.Itoa(i))
			continue
		}
		dir := "/tmp"
		for _, k := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
			if v := env(k); v != "" {
				dir = v
				break
			}
		}
		out = append(out, path.Join(dir, "discord-ipc-"+strconv.Itoa(i)))
		// Flatpak / Snap installs keep the socket in a subfolder
		if x := env("XDG_RUNTIME_DIR"); x != "" {
			out = append(out,
				path.Join(x, "app", "com.discordapp.Discord", "discord-ipc-"+strconv.Itoa(i)),
				path.Join(x, "snap.discord", "discord-ipc-"+strconv.Itoa(i)))
		}
	}
	return out
}

func encode(op uint32, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 8+len(body))
	binary.LittleEndian.PutUint32(buf[0:], op)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(body)))
	copy(buf[8:], body)
	return buf, nil
}

type reply struct {
	data json.RawMessage
	err  error
}

// Client is one connection to Discord.
type Client struct {
	paths []string

	conn io.ReadWriteCloser
	wmu  sync.Mutex

	mu      sync.Mutex
	pending map[string]chan reply
	closed  bool

	ready chan struct{}
	done  chan struct{}
	once  sync.Once
	nonce atomic.Uint64
}

// New makes a client that will look for Discord at the given paths (nil = the usual places).
func New(paths []string) *Client {
	if paths == nil {
		paths = SocketPaths()
	}
	return &Client{paths: paths, pending: map[string]chan reply{}, ready: make(chan struct{}), done: make(chan struct{})}
}

// Done is closed when the connection is gone.
func (c *Client) Done() <-chan struct{} { return c.done }

// Login connects and performs the handshake. It fails when Discord is not running or does not answer in time.
func (c *Client) Login(clientID string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 6 * time.Second
	}
	var conn io.ReadWriteCloser
	for _, p := range c.paths {
		if cn, err := dial(p); err == nil {
			conn = cn
			break
		}
	}
	if conn == nil {
		return errors.New("Discord is not running (no IPC socket found)")
	}
	c.conn = conn
	go c.readLoop()
	hs, _ := encode(opHandshake, map[string]any{"v": 1, "client_id": clientID})
	if err := c.write(hs); err != nil {
		c.Close()
		return err
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-c.ready:
		return nil
	case <-c.done:
		return errors.New("Discord closed the connection")
	case <-t.C:
		c.Close()
		return errors.New("Discord did not answer the handshake")
	}
}

func (c *Client) write(b []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.conn == nil {
		return errors.New("Not connected to Discord")
	}
	_, err := c.conn.Write(b)
	return err
}

func (c *Client) readLoop() {
	defer c.Close()
	head := make([]byte, 8)
	for {
		if _, err := io.ReadFull(c.conn, head); err != nil {
			return
		}
		op := binary.LittleEndian.Uint32(head[0:])
		n := binary.LittleEndian.Uint32(head[4:])
		if n > 16<<20 {
			return
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(c.conn, body); err != nil {
			return
		}
		var data struct {
			Cmd   string          `json:"cmd"`
			Evt   string          `json:"evt"`
			Nonce string          `json:"nonce"`
			Data  json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(body, &data) // garbage is ignored
		switch op {
		case opPing:
			if pong, err := encode(opPong, json.RawMessage(orEmpty(body))); err == nil {
				_ = c.write(pong)
			}
		case opClose:
			return
		case opFrame:
			if data.Cmd == "DISPATCH" && data.Evt == "READY" {
				c.once.Do(func() { close(c.ready) })
				continue
			}
			if data.Nonce == "" {
				continue
			}
			c.mu.Lock()
			ch := c.pending[data.Nonce]
			delete(c.pending, data.Nonce)
			c.mu.Unlock()
			if ch == nil {
				continue
			}
			if data.Evt == "ERROR" {
				var e struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				}
				_ = json.Unmarshal(data.Data, &e)
				if e.Message == "" {
					e.Message = "Discord error"
				}
				ch <- reply{err: &Error{Code: e.Code, Message: e.Message}}
			} else {
				ch <- reply{data: data.Data}
			}
		}
	}
}

func orEmpty(b []byte) []byte {
	if json.Valid(b) {
		return b
	}
	return []byte("{}")
}

// Request sends a command and waits for Discord's answer.
func (c *Client) Request(cmd string, args any, timeout time.Duration) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	c.mu.Lock()
	if c.closed || c.conn == nil {
		c.mu.Unlock()
		return nil, errors.New("Not connected to Discord")
	}
	n := strconv.FormatInt(time.Now().UnixMilli(), 36) + "-" + strconv.FormatUint(c.nonce.Add(1), 36)
	ch := make(chan reply, 1)
	c.pending[n] = ch
	c.mu.Unlock()

	frame, err := encode(opFrame, map[string]any{"cmd": cmd, "args": args, "nonce": n})
	if err == nil {
		err = c.write(frame)
	}
	if err != nil {
		c.mu.Lock()
		delete(c.pending, n)
		c.mu.Unlock()
		return nil, err
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-t.C:
		c.mu.Lock()
		delete(c.pending, n)
		c.mu.Unlock()
		return nil, errors.New("Discord did not answer in time")
	case <-c.done:
		return nil, errors.New("Connection to Discord closed")
	}
}

// SetActivity shows an activity (any JSON-encodable value); pid is the process Discord ties it to.
func (c *Client) SetActivity(activity any) error {
	_, err := c.Request("SET_ACTIVITY", map[string]any{"pid": os.Getpid(), "activity": activity}, 0)
	return err
}

// ClearActivity removes the presence.
func (c *Client) ClearActivity() error {
	_, err := c.Request("SET_ACTIVITY", map[string]any{"pid": os.Getpid()}, 0)
	return err
}

// Close drops the connection (safe to call repeatedly, from any goroutine).
func (c *Client) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	conn := c.conn
	pend := c.pending
	c.pending = map[string]chan reply{}
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	for _, ch := range pend {
		ch <- reply{err: errors.New("Connection to Discord closed")}
	}
	close(c.done)
}

var _ = fmt.Sprintf

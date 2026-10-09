package discord

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// fakeDiscord is a tiny stand-in for the Discord desktop app.
type fakeDiscord struct {
	path string
	ln   net.Listener
	mu   sync.Mutex
	got  []map[string]any
	// behaviour
	rejectType bool // answer ERROR to activities with type != 0
	silent     bool // never send READY
	pingFirst  bool
	conns      []net.Conn
}

func newFake(t *testing.T) *fakeDiscord {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Discord uses a Unix socket; the Windows named pipe is covered by the smoke test in CI")
	}
	dir := t.TempDir()
	f := &fakeDiscord{path: filepath.Join(dir, "discord-ipc-0")}
	ln, err := net.Listen("unix", f.path)
	if err != nil {
		t.Skip("no unix sockets:", err)
	}
	f.ln = ln
	t.Cleanup(func() {
		ln.Close()
		f.mu.Lock()
		for _, c := range f.conns {
			c.Close()
		}
		f.mu.Unlock()
	})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.conns = append(f.conns, c)
			f.mu.Unlock()
			go f.serve(c)
		}
	}()
	return f
}

func send(c net.Conn, op uint32, v any) {
	b, _ := encode(op, v)
	c.Write(b)
}

func (f *fakeDiscord) serve(c net.Conn) {
	head := make([]byte, 8)
	for {
		if _, err := io.ReadFull(c, head); err != nil {
			return
		}
		op := binary.LittleEndian.Uint32(head)
		body := make([]byte, binary.LittleEndian.Uint32(head[4:]))
		io.ReadFull(c, body)
		var m map[string]any
		json.Unmarshal(body, &m)
		f.mu.Lock()
		f.got = append(f.got, map[string]any{"op": float64(op), "body": m})
		f.mu.Unlock()
		switch op {
		case opHandshake:
			if f.silent {
				continue
			}
			if f.pingFirst {
				send(c, opPing, map[string]any{"n": 1})
			}
			send(c, opFrame, map[string]any{"cmd": "DISPATCH", "evt": "READY", "data": map[string]any{"v": 1}})
		case opFrame:
			args, _ := m["args"].(map[string]any)
			act, _ := args["activity"].(map[string]any)
			if f.rejectType && act != nil && act["type"] != nil {
				send(c, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "evt": "ERROR", "nonce": m["nonce"], "data": map[string]any{"code": 4000, "message": "Invalid payload"}})
				continue
			}
			send(c, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "evt": nil, "nonce": m["nonce"], "data": act})
		case opPong:
		}
	}
}

func (f *fakeDiscord) frames() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.got...)
}

func TestLoginAndActivity(t *testing.T) {
	f := newFake(t)
	f.pingFirst = true
	c := New([]string{"/nonexistent/x", f.path})
	if err := c.Login("123", time.Second); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.SetActivity(map[string]any{"details": "hi", "type": 3}); err != nil {
		t.Fatal(err)
	}
	if err := c.ClearActivity(); err != nil {
		t.Fatal(err)
	}
	fr := f.frames()
	hs := fr[0]["body"].(map[string]any)
	if hs["client_id"] != "123" || hs["v"] != float64(1) {
		t.Fatalf("handshake %v", hs)
	}
	var sawPong bool
	for _, x := range fr {
		if x["op"] == float64(opPong) {
			sawPong = true
		}
	}
	if !sawPong {
		t.Fatal("no pong for the ping")
	}
	set := fr[len(fr)-2]["body"].(map[string]any)
	if set["cmd"] != "SET_ACTIVITY" || set["args"].(map[string]any)["pid"] != float64(os.Getpid()) {
		t.Fatalf("set %v", set)
	}
	clr := fr[len(fr)-1]["body"].(map[string]any)
	if _, has := clr["args"].(map[string]any)["activity"]; has {
		t.Fatalf("clear must not carry an activity: %v", clr)
	}
}

func TestErrorReply(t *testing.T) {
	f := newFake(t)
	f.rejectType = true
	c := New([]string{f.path})
	if err := c.Login("1", time.Second); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	err := c.SetActivity(map[string]any{"type": 3})
	de, ok := err.(*Error)
	if !ok || de.Code != 4000 || de.Message != "Invalid payload" {
		t.Fatalf("got %#v", err)
	}
	if err := c.SetActivity(map[string]any{"details": "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestNoDiscord(t *testing.T) {
	c := New([]string{"/nonexistent/discord-ipc-0"})
	if err := c.Login("1", time.Second); err == nil {
		t.Fatal("expected failure")
	}
}

func TestHandshakeTimeout(t *testing.T) {
	f := newFake(t)
	f.silent = true
	c := New([]string{f.path})
	start := time.Now()
	if err := c.Login("1", 150*time.Millisecond); err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("err=%v", err)
	}
	select {
	case <-c.Done():
	default:
		t.Fatal("client should be closed")
	}
}

func TestServerCloseFailsPending(t *testing.T) {
	f := newFake(t)
	c := New([]string{f.path})
	if err := c.Login("1", time.Second); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	for _, cn := range f.conns {
		cn.Close()
	}
	f.mu.Unlock()
	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("close not noticed")
	}
	if err := c.SetActivity(nil); err == nil {
		t.Fatal("expected an error after close")
	}
}

func TestSocketPaths(t *testing.T) {
	env := map[string]string{"XDG_RUNTIME_DIR": "/run/user/1"}
	p := socketPaths(func(k string) string { return env[k] }, "linux")
	if p[0] != "/run/user/1/discord-ipc-0" || p[1] != "/run/user/1/app/com.discordapp.Discord/discord-ipc-0" || len(p) != 30 {
		t.Fatal(p[:3], len(p))
	}
	w := socketPaths(func(string) string { return "" }, "windows")
	if w[3] != `\\?\pipe\discord-ipc-3` || len(w) != 10 {
		t.Fatal(w)
	}
}

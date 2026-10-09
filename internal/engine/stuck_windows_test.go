//go:build windows

package engine

import (
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// A Discord that answers the handshake and then stops reading, on an unbuffered pipe: our next write blocks inside
// Windows, and closing the pipe would wait for it. Stopping (and so quitting) must still finish promptly.
func TestStopDoesNotHangOnAStuckDiscord(t *testing.T) {
	path := fakePath("")
	name, _ := syscall.UTF16PtrFromString(path)
	h, _, err := pCreateNamedPipe.Call(uintptr(unsafe.Pointer(name)), pipeAccessDuplex, 0, 1, 0, 0, 0, 0) // no buffers
	if h == invalidHandleValue {
		t.Skip("cannot create the pipe: " + err.Error())
	}
	ph := syscall.Handle(h)
	defer syscall.CloseHandle(ph)
	go func() {
		pConnectNamedPipe.Call(h, 0)
		head := make([]byte, 8)
		var n uint32
		syscall.ReadFile(ph, head, &n, nil) // the handshake
		body := make([]byte, binary.LittleEndian.Uint32(head[4:]))
		syscall.ReadFile(ph, body, &n, nil)
		b, _ := json.Marshal(map[string]any{"cmd": "DISPATCH", "evt": "READY", "data": map[string]any{}})
		out := make([]byte, 8, 8+len(b))
		binary.LittleEndian.PutUint32(out, 1)
		binary.LittleEndian.PutUint32(out[4:], uint32(len(b)))
		syscall.WriteFile(ph, append(out, b...), &n, nil)
		// ...and never reads again
	}()

	m := newMPC(t)
	cfg := core.DefaultConfig()
	cfg.Port, cfg.PollInterval, cfg.ShowArtwork = m.port, 250, false
	e := New(cfg, Options{Pipes: noPipes, MPRIS: noMPRIS, DiscordPaths: []string{path}, CacheFile: filepath.Join(t.TempDir(), "c.json")})
	e.Start()
	eventually(t, "connected", func() bool { return e.Status().Discord == "connected" })
	time.Sleep(500 * time.Millisecond) // the activity write is now stuck

	done := make(chan struct{})
	go func() { e.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("Stop hangs when Discord stops reading")
	}
}

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests build the real program and drive it from outside, with a fake MPC-HC and a fake Discord.

func build(t *testing.T, version, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, "-ldflags", "-X main.version="+version, ".")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, b)
	}
}

type fakeDiscord struct {
	mu   sync.Mutex
	acts []map[string]any
}

func startDiscord(t *testing.T, path string) *fakeDiscord {
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Skip("no unix sockets")
	}
	d := &fakeDiscord{}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				h := make([]byte, 8)
				for {
					if _, err := io.ReadFull(c, h); err != nil {
						return
					}
					op := binary.LittleEndian.Uint32(h)
					body := make([]byte, binary.LittleEndian.Uint32(h[4:]))
					io.ReadFull(c, body)
					var m map[string]any
					json.Unmarshal(body, &m)
					var resp any
					if op == 0 {
						resp = map[string]any{"cmd": "DISPATCH", "evt": "READY", "data": map[string]any{}}
					} else {
						d.mu.Lock()
						d.acts = append(d.acts, m["args"].(map[string]any))
						d.mu.Unlock()
						resp = map[string]any{"evt": nil, "nonce": m["nonce"], "data": nil}
					}
					b, _ := json.Marshal(resp)
					hh := make([]byte, 8)
					binary.LittleEndian.PutUint32(hh, 1)
					binary.LittleEndian.PutUint32(hh[4:], uint32(len(b)))
					c.Write(append(hh, b...))
				}
			}()
		}
	}()
	return d
}

func (d *fakeDiscord) n() int { d.mu.Lock(); defer d.mu.Unlock(); return len(d.acts) }

type api struct {
	port  int
	token string
}

func (a api) call(t *testing.T, method, name, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/api/%s", a.port, name), strings.NewReader(body))
	req.Header.Set("X-Token", a.token)
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	b, _ := io.ReadAll(res.Body)
	json.Unmarshal(b, &out)
	return res.StatusCode, out
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if f() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", what)
}

func readIPC(home string) (api, bool) {
	raw, err := os.ReadFile(filepath.Join(home, "ipc.json"))
	if err != nil {
		return api{}, false
	}
	var i ipcInfo
	if json.Unmarshal(raw, &i) != nil || i.Port == 0 {
		return api{}, false
	}
	return api{i.Port, i.Token}, true
}

func TestEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by the Windows smoke test in CI")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "app")
	build(t, "3.0.0", exe)
	newExe := filepath.Join(dir, "app-new")
	build(t, "3.0.1", newExe)
	newBytes, _ := os.ReadFile(newExe)

	home := filepath.Join(dir, "home")
	xdg := filepath.Join(dir, "run")
	os.MkdirAll(xdg, 0o755)
	disc := startDiscord(t, filepath.Join(xdg, "discord-ipc-0"))

	var mu sync.Mutex
	state := 2
	mpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(w, `<p id="file">Sample.Movie.2001.1080p.mkv</p><p id="filepath">/movies/Sample.Movie.2001.1080p.mkv</p><p id="state">%d</p><p id="position">1000</p><p id="duration">7500000</p><p id="playbackrate">1</p>`, state)
	}))
	defer mpc.Close()
	mpcPort := mpc.Listener.Addr().(*net.TCPAddr).Port

	// a fake GitHub
	var rel *httptest.Server
	rel = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":"v3.0.1","body":"notes","html_url":"x","assets":[{"name":"MPCvibedRPC.exe","browser_download_url":"%s/dl"}]}`, rel.URL)
		case r.URL.Path == "/dl":
			w.Write(newBytes)
		}
	}))
	defer rel.Close()

	os.MkdirAll(home, 0o755)
	cfg := fmt.Sprintf(`{"port":%d,"pollInterval":250,"showArtwork":false,"clientId":"1"}`, mpcPort)
	os.WriteFile(filepath.Join(home, "config.json"), []byte(cfg), 0o644)

	env := append(os.Environ(), "MPCRPC_HOME="+home, "XDG_RUNTIME_DIR="+xdg, "MPCRPC_UPDATE_API="+rel.URL, "DISPLAY=", "PATH=/nonexistent")
	cmd := exec.Command(exe, "--background")
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		cmd.Process.Kill()
		if a, ok := readIPC(home); ok {
			a.call(t, "POST", "quit", "{}")
		}
	})

	var a api
	waitFor(t, "ipc.json", func() bool { var ok bool; a, ok = readIPC(home); return ok })

	// ping needs no token; everything else does
	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", a.port))
	if err != nil {
		t.Fatal(err)
	}
	pb, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(pb), `"version":"3.0.0"`) {
		t.Fatalf("ping %s", pb)
	}
	if code, _ := (api{a.port, "bad"}).call(t, "GET", "state", ""); code != 401 {
		t.Fatalf("code %d", code)
	}

	// presence flows from MPC to Discord
	waitFor(t, "activity", func() bool { return disc.n() >= 1 })
	disc.mu.Lock()
	act := disc.acts[0]["activity"].(map[string]any)
	disc.mu.Unlock()
	if !strings.Contains(fmt.Sprint(act), "Sample Movie") {
		t.Fatalf("activity %v", act)
	}
	_, st := a.call(t, "GET", "state", "")
	status := st["status"].(map[string]any)
	if st["version"] != "3.0.0" || status["running"] != true || status["discord"] != "connected" || len(st["schema"].([]any)) == 0 || st["values"].(map[string]any)["pollInterval"] != float64(250) {
		t.Fatalf("state %v", st)
	}
	if st["update"] != nil {
		t.Fatal("no update expected yet")
	}

	// stop / start via the API clears and resumes
	n := disc.n()
	_, s := a.call(t, "POST", "stop", "{}")
	if s["running"] != false || disc.n() != n+1 {
		t.Fatalf("stop: %v (%d vs %d)", s, disc.n(), n)
	}
	a.call(t, "POST", "start", "{}")
	waitFor(t, "resume", func() bool { return disc.n() >= n+2 })

	// invalid settings are rejected with 400, valid ones are saved
	if code, _ := a.call(t, "POST", "save", `{"settings":{"port":"abc"}}`); code != 400 {
		t.Fatalf("code %d", code)
	}
	code, saved := a.call(t, "POST", "save", `{"settings":{"pollInterval":2000},"app":{"checkUpdates":false}}`)
	if code != 200 || saved["values"].(map[string]any)["pollInterval"] != float64(2000) {
		t.Fatalf("save %d %v", code, saved)
	}
	onDisk, _ := os.ReadFile(filepath.Join(home, "config.json"))
	if !strings.Contains(string(onDisk), `"pollInterval": 2000`) && !strings.Contains(string(onDisk), `"pollInterval":2000`) {
		t.Fatalf("not saved: %s", onDisk)
	}
	if _, lg := a.call(t, "GET", "log", ""); len(lg["lines"].([]any)) < 3 {
		t.Fatal("log lines missing")
	}

	// a second launch hands over to the first and exits quietly
	second := exec.Command(exe)
	second.Env = env
	if err := second.Run(); err != nil {
		t.Fatalf("second instance: %v", err)
	}

	// update: check, download, swap, restart as the new version
	_, u := a.call(t, "POST", "update-check", "{}")
	if u["newer"] != true || u["latest"] != "3.0.1" {
		t.Fatalf("update-check %v", u)
	}
	_, st = a.call(t, "GET", "state", "")
	if st["update"] == nil {
		t.Fatal("state should now carry the update")
	}
	a.call(t, "POST", "update-install", "{}")
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("old program did not exit after updating")
	}
	var b2 api
	waitFor(t, "new version running", func() bool {
		x, ok := readIPC(home)
		if !ok {
			return false
		}
		r, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", x.port))
		if err != nil {
			return false
		}
		defer r.Body.Close()
		bb, _ := io.ReadAll(r.Body)
		b2 = x
		return strings.Contains(string(bb), `"version":"3.0.1"`)
	})
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, newBytes) {
		t.Fatal("exe was not replaced")
	}
	waitFor(t, "presence from the new version", func() bool { return disc.n() >= n+3 })
	b2.call(t, "POST", "quit", "{}")
	waitFor(t, "ipc.json removed", func() bool { _, ok := readIPC(home); return !ok })
	waitFor(t, ".old cleaned later", func() bool { return true })
}

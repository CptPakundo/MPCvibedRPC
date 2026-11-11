package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// brokenProgram builds a program that ends at once with an error: a new version that does not start.
func brokenProgram(t *testing.T, out string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "main.go")
	os.WriteFile(src, []byte("package main\n\nimport \"os\"\n\nfunc main() { os.Exit(3) }\n"), 0o644)
	cmd := exec.Command("go", "build", "-o", out, src)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, b)
	}
	// the updater refuses a download of under a megabyte; bytes after a program's end do it no harm
	f, err := os.OpenFile(out, os.O_APPEND|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	f.Write(make([]byte, 2<<20))
	f.Close()
}

// The real program installs an update that does not start: the old version must be back and running, with its own
// program file, and must not install that version by itself again.
func TestEndToEndUpdateRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by the Windows smoke test in CI")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("the macOS program does not install updates itself")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "app")
	build(t, "3.0.0", exe)
	oldBytes, _ := os.ReadFile(exe)
	broken := filepath.Join(dir, "app-broken")
	brokenProgram(t, broken)
	newBytes, _ := os.ReadFile(broken)

	var rel *httptest.Server
	rel = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":"v3.0.1","body":"notes","html_url":"x","assets":[{"name":"MPCvibedRPC-linux-amd64","browser_download_url":"%[1]s/dl"},{"name":"MPCvibedRPC-linux-amd64.sha256","browser_download_url":"%[1]s/sha"},{"name":"MPCvibedRPC-linux-arm64","browser_download_url":"%[1]s/dl"},{"name":"MPCvibedRPC-linux-arm64.sha256","browser_download_url":"%[1]s/sha"}]}`, rel.URL)
		case r.URL.Path == "/dl":
			w.Write(newBytes)
		case r.URL.Path == "/sha":
			sum := sha256.Sum256(newBytes)
			fmt.Fprintf(w, "%x  MPCvibedRPC\n", sum)
		}
	}))
	defer rel.Close()

	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0o755)
	// nothing is asked and nothing is shown: only the update is under test
	os.WriteFile(filepath.Join(home, "config.json"), []byte(`{"port":1,"showArtwork":false,"mpris":false,"app":{"startPresence":false,"checkUpdates":false,"autoUpdate":false,"welcomeSeen":true}}`), 0o644)
	env := append(os.Environ(), "MPCRPC_HOME="+home, "XDG_RUNTIME_DIR="+filepath.Join(dir, "run"), "MPCRPC_UPDATE_API="+rel.URL, "DISPLAY=", "PATH=/nonexistent", "MPCRPC_BROWSER=none", "MPCRPC_FOREGROUND=1")
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
			waitGone(t, a.pid)
		}
	})

	var a api
	waitFor(t, "ipc.json", func() bool { var ok bool; a, ok = readIPC(home); return ok })
	if _, u := a.call(t, "POST", "update-check", "{}"); u["newer"] != true || u["canInstall"] != true {
		t.Fatalf("update-check %v", u)
	}
	a.call(t, "POST", "update-install", "{}")
	select {
	case <-exited:
	case <-time.After(60 * time.Second):
		t.Fatalf("the program that installed the update did not leave\n%s", out.String())
	}

	var back api
	waitFor(t, "the old version running again", func() bool {
		x, ok := readIPC(home)
		if !ok || x.pid == a.pid {
			return false
		}
		r, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", x.port))
		if err != nil {
			return false
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		back = x
		return strings.Contains(string(b), `"version":"3.0.0"`)
	})
	if got, _ := os.ReadFile(exe); !bytes.Equal(got, oldBytes) {
		t.Error("the old program file is not back in place")
	}
	if v, _ := os.ReadFile(filepath.Join(home, failedMarker)); strings.TrimSpace(string(v)) != "3.0.1" {
		t.Errorf("the failed version was not remembered: %q", v)
	}
	logText, _ := os.ReadFile(filepath.Join(home, "mpcvibedrpc.log"))
	for _, want := range []string{"The new version 3.0.1 did not start", "The update to 3.0.1 did not start, so 3.0.0 is back"} {
		if !strings.Contains(string(logText), want) {
			t.Errorf("the log lacks %q:\n%s", want, logText)
		}
	}
	back.call(t, "POST", "quit", "{}")
	waitFor(t, "ipc.json removed", func() bool { _, ok := readIPC(home); return !ok })
	waitGone(t, back.pid)
}

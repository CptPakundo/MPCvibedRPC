package pipe

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestExists(t *testing.T) {
	name := "mpcvibedrpc-test-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var path string
	if runtime.GOOS == "windows" {
		path = `\\?\pipe\` + name
	} else {
		dir, err := os.MkdirTemp("/tmp", "px") // a socket path may only be 104 bytes on macOS
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		path = filepath.Join(dir, name)
	}
	if Exists(path) {
		t.Fatal("a pipe that was never made exists")
	}
	stop := listen(t, path)
	if !Exists(path) {
		t.Fatal("the pipe is not found")
	}
	if Exists(path + "x") {
		t.Fatal("another name matched")
	}
	stop()
	if Exists(path) {
		t.Fatal("the pipe is still found after it was closed")
	}
	if runtime.GOOS != "windows" {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if Exists(path) {
			t.Fatal("a plain file counts as a socket")
		}
	}
}

func listen(t *testing.T, path string) func() {
	t.Helper()
	if runtime.GOOS != "windows" {
		ln, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		return func() { ln.Close() }
	}
	return listenPipe(t, path)
}

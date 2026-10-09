package updater

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() { RequireMZ = true }

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.0.1", "3.0.0", -1}, {"3.0.0", "3.0.0", 0}, {"v3.0.1", "3.0.0", 1}, {"3.0.0-beta", "3.0.0", -1},
		{"3.0.0", "3.0.0-rc1", 1}, {"3.0.0-a", "3.0.0-b", -1}, {"1.10.0", "1.9.0", 1}, {"x", "1.0.0", 0}, {"2.0.1", "2.0.10", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func fakeExe(n int) []byte {
	b := bytes.Repeat([]byte{7}, n)
	b[0], b[1] = 'M', 'Z'
	return b
}

func setup(t *testing.T, exe []byte, sha string, notes string) (*httptest.Server, func()) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "MPCvibedRPC" {
			t.Error("missing UA")
		}
		shaAsset := ""
		if sha != "" {
			shaAsset = fmt.Sprintf(`,{"name":"MPCvibedRPC.exe.sha256","browser_download_url":"%s/sha"}`, srv.URL)
		}
		fmt.Fprintf(w, `{"tag_name":"v3.1.0","body":%q,"html_url":"https://example/rel","assets":[{"name":"other.zip","browser_download_url":"x"},{"name":"MPCvibedRPC.exe","browser_download_url":"%s/exe"}%s]}`, notes, srv.URL, shaAsset)
	})
	mux.HandleFunc("/exe", func(w http.ResponseWriter, r *http.Request) { w.Write(exe) })
	mux.HandleFunc("/sha", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sha+"  MPCvibedRPC.exe\n") })
	srv = httptest.NewServer(mux)
	old := APIBase
	APIBase = srv.URL
	return srv, func() { APIBase = old; srv.Close() }
}

func TestCheckAndDownload(t *testing.T) {
	exe := fakeExe(2 << 20)
	sum := sha256.Sum256(exe)
	_, done := setup(t, exe, hex.EncodeToString(sum[:]), strings.Repeat("é", 2000))
	defer done()

	info, err := Check(nil, "o/r", "2.0.1")
	if err != nil || !info.Newer || info.Latest != "3.1.0" || info.URL == "" || info.ShaURL == "" || info.Page != "https://example/rel" {
		t.Fatalf("%+v %v", info, err)
	}
	if n := len([]rune(info.Notes)); n != 1500 {
		t.Fatalf("notes %d", n)
	}
	if i2, _ := Check(nil, "o/r", "3.1.0"); i2.Newer {
		t.Fatal("same version is not newer")
	}
	if i3, _ := Check(nil, "bad repo", "1.0.0"); i3.Configured {
		t.Fatal("bad repo should be unconfigured")
	}
	if i4, err := Check(nil, "o/missing", "1.0.0"); err != nil || i4.Error == "" {
		t.Fatalf("404: %+v %v", i4, err)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "app.exe")
	os.WriteFile(target, []byte("old"), 0o755)
	next, err := Download(nil, info, target)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(next)
	if !bytes.Equal(got, exe) {
		t.Fatal("content differs")
	}

	var started []string
	exited := false
	err = SwapAndRestart(target, next, func(e string, args ...string) error {
		started = append(started, e)
		started = append(started, args...)
		return nil
	}, func() { exited = true })
	if err != nil || !exited || len(started) != 2 || started[1] != "--after-update" {
		t.Fatalf("%v %v %v", err, exited, started)
	}
	now, _ := os.ReadFile(target)
	oldc, _ := os.ReadFile(target + ".old")
	if !bytes.Equal(now, exe) || string(oldc) != "old" {
		t.Fatal("swap wrong")
	}
	CleanupOld(target)
	if _, err := os.Stat(target + ".old"); err == nil {
		t.Fatal(".old should be removed")
	}
}

func TestDownloadRejects(t *testing.T) {
	good := fakeExe(2 << 20)
	t.Run("checksum", func(t *testing.T) {
		_, done := setup(t, good, strings.Repeat("0", 64), "")
		defer done()
		info, _ := Check(nil, "o/r", "1.0.0")
		if _, err := Download(nil, info, filepath.Join(t.TempDir(), "a.exe")); err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatal(err)
		}
	})
	t.Run("not a program", func(t *testing.T) {
		_, done := setup(t, bytes.Repeat([]byte("<html>"), 400000), "", "")
		defer done()
		info, _ := Check(nil, "o/r", "1.0.0")
		if _, err := Download(nil, info, filepath.Join(t.TempDir(), "a.exe")); err == nil {
			t.Fatal("should reject")
		}
	})
	t.Run("too small", func(t *testing.T) {
		_, done := setup(t, fakeExe(1000), "", "")
		defer done()
		info, _ := Check(nil, "o/r", "1.0.0")
		if _, err := Download(nil, info, filepath.Join(t.TempDir(), "a.exe")); err == nil {
			t.Fatal("should reject")
		}
	})
	t.Run("no asset", func(t *testing.T) {
		if _, err := Download(nil, &Info{}, "x"); err == nil {
			t.Fatal("should reject")
		}
	})
}

func TestSwapRollsBackWhenStartFails(t *testing.T) {
	dir := t.TempDir()
	exe, next := filepath.Join(dir, "a.exe"), filepath.Join(dir, "a.exe.new")
	os.WriteFile(exe, []byte("old"), 0o755)
	os.WriteFile(next, []byte("new"), 0o755)
	err := SwapAndRestart(exe, next, func(string, ...string) error { return fmt.Errorf("boom") }, func() { t.Fatal("must not exit") })
	if err == nil {
		t.Fatal("expected error")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatal("old program must be back")
	}
}

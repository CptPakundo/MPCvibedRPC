package updater

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowedHost(t *testing.T) {
	oldAPI, oldWeb := APIBase, WebBase
	APIBase, WebBase = "http://127.0.0.1:5555", "http://127.0.0.1:5555"
	defer func() { APIBase, WebBase = oldAPI, oldWeb }()
	for addr, want := range map[string]bool{
		"https://github.com/o/r/releases/download/v1/MPCvibedRPC.exe": true,
		"https://GitHub.com/o/r/releases/download/v1/MPCvibedRPC.exe": true,
		"https://objects.githubusercontent.com/x":                     true,
		"https://release-assets.githubusercontent.com/x":              true,
		"http://127.0.0.1:5555/dl":                                    true, // the base itself is plain http
		"http://127.0.0.1:6666/dl":                                    false,
		"https://127.0.0.1:5555/dl":                                   false, // another scheme than the base
		"http://github.com/o/r/x":                                     false, // not https
		"https://evil.example/github.com":                             false,
		"https://github.com.evil.example/x":                           false,
		"https://evilgithubusercontent.com/x":                         false,
		"https://githubusercontent.com/x":                             false,
		"https://github.com:8443/x":                                   false,
		"https://user:pw@github.com/x":                                false,
		"https://api.github.com/repos/o/r/releases/assets/1":          false, // not a base here
	} {
		u, err := url.Parse(addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := allowedHost(u); got != want {
			t.Errorf("allowedHost(%q) = %v, want %v", addr, got, want)
		}
	}
	// the real bases allow their own hosts over https
	APIBase, WebBase = "https://api.github.com", "https://github.com"
	u, _ := url.Parse("https://api.github.com/x")
	if !allowedHost(u) {
		t.Error("the API host of the real base")
	}
}

// A release whose asset points at another host is not downloaded, and the error names that host.
func TestDownloadRefusesForeignHosts(t *testing.T) {
	var hits int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.Write(fakeExe(2 << 20)) }))
	defer other.Close()
	otherHost := strings.TrimPrefix(other.URL, "http://")
	// the "API" lives under a name that differs from the other server's host, though both are local
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v3.1.0","html_url":"https://example/rel","assets":[{"name":"MPCvibedRPC.exe","browser_download_url":"%s/exe"},{"name":"MPCvibedRPC.exe.sha256","browser_download_url":"%s/sha"}]}`, other.URL, other.URL)
	}))
	defer api.Close()
	old := APIBase
	APIBase = api.URL
	defer func() { APIBase = old }()
	info, err := Check(nil, "o/foreign", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Download(nil, info, filepath.Join(t.TempDir(), "a.exe")); err == nil || !strings.Contains(err.Error(), otherHost) {
		t.Fatalf("want an error naming %s, got %v", otherHost, err)
	}
	// one of the two addresses off GitHub is enough
	mixed := &Info{URL: api.URL + "/exe", ShaURL: other.URL + "/sha"}
	if _, err := Download(nil, mixed, filepath.Join(t.TempDir(), "a.exe")); err == nil || !strings.Contains(err.Error(), otherHost) {
		t.Fatalf("checksum on another host: %v", err)
	}
	if hits != 0 {
		t.Errorf("the foreign host was contacted %d times", hits)
	}
}

// A redirect to another host ends the download too.
func TestDownloadRefusesForeignRedirect(t *testing.T) {
	var hits int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.Write(fakeExe(2 << 20)) }))
	defer other.Close()
	// "localhost" and "127.0.0.1" are different hosts for the rule
	otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sha" {
			fmt.Fprint(w, strings.Repeat("a", 64))
			return
		}
		http.Redirect(w, r, otherURL+"/exe", http.StatusFound)
	}))
	defer api.Close()
	old := APIBase
	APIBase = api.URL
	defer func() { APIBase = old }()
	info := &Info{URL: api.URL + "/exe", ShaURL: api.URL + "/sha"}
	_, err := Download(nil, info, filepath.Join(t.TempDir(), "a.exe"))
	if err == nil || !strings.Contains(err.Error(), "redirected to localhost") {
		t.Fatalf("got %v", err)
	}
	if hits != 0 {
		t.Errorf("the redirect target was contacted %d times", hits)
	}
}

// Check sends the ETag of its last answer back; a 304 returns that answer again, judged for the version asked about now.
func TestCheckIsConditional(t *testing.T) {
	var requests, conditional int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("If-None-Match") == `"abc"` {
			conditional++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc"`)
		fmt.Fprint(w, `{"tag_name":"v3.1.0","body":"Notes.","html_url":"https://example/rel","assets":[{"name":"MPCvibedRPC.exe","browser_download_url":"x"},{"name":"MPCvibedRPC.exe.sha256","browser_download_url":"y"}]}`)
	}))
	defer srv.Close()
	old := APIBase
	APIBase = srv.URL
	defer func() { APIBase = old }()

	first, err := Check(nil, "o/etag", "3.0.0")
	if err != nil || !first.Newer || first.Latest != "3.1.0" || conditional != 0 {
		t.Fatalf("first: %+v %v (conditional %d)", first, err, conditional)
	}
	second, err := Check(nil, "o/etag", "3.0.0")
	if err != nil || conditional != 1 || requests != 2 {
		t.Fatalf("second: %+v %v (requests %d, conditional %d)", second, err, requests, conditional)
	}
	if !second.Newer || second.Latest != "3.1.0" || second.Notes != "Notes." || second.Page != "https://example/rel" || !second.CanInstall {
		t.Fatalf("a 304 must return the earlier answer: %+v", second)
	}
	// the kept answer is judged against the version asked about now, and never changed by it
	third, err := Check(nil, "o/etag", "3.1.0")
	if err != nil || third.Newer || third.Current != "3.1.0" {
		t.Fatalf("third: %+v %v", third, err)
	}
	if again, _ := Check(nil, "o/etag", "3.0.0"); !again.Newer || again.Current != "3.0.0" {
		t.Fatalf("again: %+v", again)
	}
	if cur, tag := lastRelease(srv.URL + "/repos/o/other/releases/latest"); cur != nil || tag != "" {
		t.Fatal("nothing is kept for another repository")
	}
}

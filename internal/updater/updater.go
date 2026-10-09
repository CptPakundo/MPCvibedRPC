// Package updater updates the program from GitHub Releases. Publish a release whose tag is a version ("v1.2.0")
// with the program for each system (MPCvibedRPC.exe, MPCvibedRPC-linux-amd64, ...; see AssetFor) and optionally a
// .sha256 next to each; the app offers it, downloads it and swaps itself in. On macOS it only offers the release page.
package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/proc"
)

// Asset is the file a release has to carry for this system to update itself ("" where it does not: on macOS the
// program is an app bundle, and the user is sent to the release page instead).
var Asset = AssetFor(runtime.GOOS, runtime.GOARCH)

// AssetFor names the release file of a system: MPCvibedRPC.exe, MPCvibedRPC-linux-amd64, ...
func AssetFor(goos, goarch string) string {
	switch {
	case goos == "windows":
		return "MPCvibedRPC.exe"
	case goos == "linux" && (goarch == "amd64" || goarch == "arm64"):
		return "MPCvibedRPC-linux-" + goarch
	}
	return ""
}

// APIBase is GitHub's API root (tests point it elsewhere).
var APIBase = "https://api.github.com"

// MinSize is the smallest believable program (guards against saving an error page as the program).
var MinSize int64 = 1024 * 1024

// Magic is how a program file of this system starts ("MZ" on Windows, "\x7fELF" on Linux); Download insists on it.
var Magic = map[string]string{"windows": "MZ", "linux": "\x7fELF"}[runtime.GOOS]

// Info is the answer of Check (the shape the settings window reads).
type Info struct {
	Configured bool   `json:"configured"`
	Error      string `json:"error,omitempty"`
	Current    string `json:"current,omitempty"`
	Latest     string `json:"latest,omitempty"`
	Newer      bool   `json:"newer"`
	Notes      string `json:"notes,omitempty"`
	URL        string `json:"url,omitempty"`
	ShaURL     string `json:"shaUrl,omitempty"`
	Page       string `json:"page,omitempty"`
	// CanInstall is true when the program can install the release itself; otherwise the window links to Page.
	CanInstall bool `json:"canInstall"`
}

type version struct {
	n   [3]int
	pre string
}

var reVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?`)

func parseVersion(v string) *version {
	v = strings.TrimSpace(v)
	if len(v) > 0 && (v[0] == 'v' || v[0] == 'V') {
		v = v[1:]
	}
	m := reVersion.FindStringSubmatch(v)
	if m == nil {
		return nil
	}
	out := &version{pre: m[4]}
	for i := 0; i < 3; i++ {
		out.n[i], _ = strconv.Atoi(m[i+1])
	}
	return out
}

// Compare returns -1 / 0 / 1; a pre-release ("1.2.0-beta") sorts before its release. Unparseable versions are equal.
func Compare(a, b string) int {
	x, y := parseVersion(a), parseVersion(b)
	if x == nil || y == nil {
		return 0
	}
	for i := 0; i < 3; i++ {
		if x.n[i] != y.n[i] {
			if x.n[i] < y.n[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case x.pre == y.pre:
		return 0
	case x.pre == "":
		return 1
	case y.pre == "":
		return -1
	case x.pre < y.pre:
		return -1
	}
	return 1
}

var reRepo = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

func newReq(ctx context.Context, url string, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MPCvibedRPC")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return req, nil
}

// Check asks GitHub for the latest release of repo ("owner/repo").
func Check(client *http.Client, repo, current string) (*Info, error) {
	if !reRepo.MatchString(repo) {
		return &Info{Configured: false}, nil
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := newReq(ctx, APIBase+"/repos/"+repo+"/releases/latest", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return &Info{Configured: true, Error: "No releases published there yet."}, nil
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fmt.Errorf("GitHub answered HTTP %d", res.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	latest := rel.TagName
	if len(latest) > 0 && (latest[0] == 'v' || latest[0] == 'V') {
		latest = latest[1:]
	}
	info := &Info{Configured: true, Current: current, Latest: latest, Newer: Compare(current, latest) < 0, Page: rel.HTMLURL}
	notes := []rune(rel.Body)
	if len(notes) > 1500 {
		notes = notes[:1500]
	}
	info.Notes = string(notes)
	for _, a := range rel.Assets {
		switch {
		case Asset == "":
		case a.Name == Asset:
			info.URL = a.URL
		case a.Name == Asset+".sha256":
			info.ShaURL = a.URL
		}
	}
	info.CanInstall = info.URL != ""
	return info, nil
}

// Download fetches the new program next to exe (as exe+".new") and returns its path.
func Download(client *http.Client, info *Info, exe string) (string, error) {
	if info == nil || info.URL == "" {
		if Asset == "" {
			return "", errors.New("Download the new version from its release page.")
		}
		return "", fmt.Errorf("The release has no %s file.", Asset)
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, err := newReq(ctx, info.URL, "")
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return "", fmt.Errorf("Download failed (HTTP %d)", res.StatusCode)
	}
	buf, err := io.ReadAll(io.LimitReader(res.Body, 512<<20))
	if err != nil {
		return "", err
	}
	if int64(len(buf)) < MinSize || !bytes.HasPrefix(buf, []byte(Magic)) {
		if runtime.GOOS == "linux" {
			return "", errors.New("The downloaded file does not look like a Linux program.")
		}
		return "", errors.New("The downloaded file does not look like a Windows program.")
	}
	if info.ShaURL != "" {
		sreq, err := newReq(ctx, info.ShaURL, "")
		if err != nil {
			return "", err
		}
		sres, err := client.Do(sreq)
		if err != nil {
			return "", err
		}
		defer sres.Body.Close()
		txt, _ := io.ReadAll(io.LimitReader(sres.Body, 4096))
		f := strings.Fields(string(txt))
		if len(f) > 0 {
			sum := sha256.Sum256(buf)
			if strings.ToLower(f[0]) != hex.EncodeToString(sum[:]) {
				return "", errors.New("The download is corrupted (checksum mismatch).")
			}
		}
	}
	next := exe + ".new"
	if err := os.WriteFile(next, buf, 0o755); err != nil {
		return "", err
	}
	return next, nil
}

// SwapAndRestart: current -> .old, new -> current, start it, then call exit. Windows lets a running program be renamed.
// start can be replaced in tests.
func SwapAndRestart(exe, next string, start func(exe string, args ...string) error, exit func()) error {
	old := exe + ".old"
	_ = os.Remove(old) // leftover from a previous update; if it is still locked the rename below fails loudly
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(next, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	if start == nil {
		start = func(exe string, args ...string) error {
			c := proc.Detach(exec.Command(exe, args...))
			if err := c.Start(); err != nil {
				return err
			}
			return c.Process.Release()
		}
	}
	if err := start(exe, "--after-update"); err != nil {
		// the new program could not start; keep the old copy running
		_ = os.Rename(exe, next)
		_ = os.Rename(old, exe)
		return fmt.Errorf("The new version could not be started: %v", err)
	}
	exit()
	return nil
}

// CleanupOld removes the leftovers of an update.
func CleanupOld(exe string) {
	_ = os.Remove(exe + ".old")
	_ = os.Remove(exe + ".new")
}

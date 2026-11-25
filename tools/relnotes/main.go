// relnotes writes the text of a GitHub release from the in-app changelog (internal/assets/changelog.json), so a
// release is described once: the newest entry's lines, an optional hand-written block (.github/release-advisory.md,
// for a security note or anything the one-liners cannot say; delete it after the release that needs it), and the
// same "Get it" footer every release has (with a line on the copies under the program's former name while
// tools/ci/former-names.txt has file lines).
//
//	go run ./tools/relnotes -version X.Y.Z -out notes.md [-changelog path] [-advisory path] [-former-names path]
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

type entry struct {
	Version string   `json:"version"`
	Date    string   `json:"date"`
	Items   []string `json:"items"`
}

func main() {
	version := flag.String("version", "", "the version being released (must be the changelog's newest entry)")
	out := flag.String("out", "", "where to write the release text (stdout when empty)")
	changelog := flag.String("changelog", "internal/assets/changelog.json", "the in-app changelog")
	advisory := flag.String("advisory", ".github/release-advisory.md", "an optional hand-written block (used when the file exists)")
	formerNames := flag.String("former-names", "tools/ci/former-names.txt", "the list of former-name files (used when the file exists)")
	flag.Parse()
	text, err := build(*version, *changelog, *advisory, *formerNames)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relnotes:", err)
		os.Exit(1)
	}
	if *out == "" {
		fmt.Print(text)
		return
	}
	if err := os.WriteFile(*out, []byte(text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "relnotes:", err)
		os.Exit(1)
	}
}

// build makes the release text for version from the changelog file, the advisory file and the list of former-name
// files (both may be missing).
func build(version, changelogPath, advisoryPath, formerPath string) (string, error) {
	if version == "" {
		return "", errors.New("-version is required")
	}
	raw, err := os.ReadFile(changelogPath)
	if err != nil {
		return "", err
	}
	var log []entry
	if err := json.Unmarshal(raw, &log); err != nil {
		return "", fmt.Errorf("%s: %w", changelogPath, err)
	}
	if len(log) == 0 || log[0].Version != version {
		return "", fmt.Errorf("the newest changelog entry is not %s", version)
	}
	extra := ""
	if b, err := os.ReadFile(advisoryPath); err == nil {
		extra = strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n"))
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	former := false
	if b, err := os.ReadFile(formerPath); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if f := strings.Fields(l); len(f) > 0 && f[0] == "file" {
				former = true
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return render(log[0], extra, former), nil
}

// render lays the text out: heading, the changelog's lines as a list, the hand-written block, the footer (with the
// line on the copies under the former name when former).
func render(e entry, extra string, former bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## MPRP %s\n\n", e.Version)
	b.WriteString("### What's new\n")
	for _, it := range e.Items {
		b.WriteString("- " + strings.TrimSpace(it) + "\n")
	}
	if extra != "" {
		b.WriteString("\n" + extra + "\n")
	}
	b.WriteString(footerTop)
	if former {
		b.WriteString(formerLine)
	}
	b.WriteString(footer)
	return b.String()
}

const footerTop = `
Nothing to do: the update installs itself (Windows and Linux, unless you switched that off), and your settings and
sign-ins are kept.
`

// formerLine explains the release files under the program's former name (tools/ci/former-names.txt).
const formerLine = `
The ` + "`MPCvibedRPC`" + ` files are the same program under its former name, so that copies installed before the rename
can update themselves; download the ` + "`MPRP`" + ` files.
`

const footer = `
### Get it
- **Windows:** download **` + "`MPRP.exe`" + `** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **` + "`MPRP-macos.zip`" + `** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **` + "`MPRP-linux-amd64`" + `** (or ` + "`-arm64`" + `), ` + "`chmod +x`" + ` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPRP/blob/master/SECURITY.md).
`

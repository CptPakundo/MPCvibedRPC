// Package doclinks has only a test: every relative link in the repository's Markdown files, and every #anchor in
// one, must resolve. It is part of the tests that tie the texts to the code (docs/development.md > Testing), and
// .github/workflows/docs.yml runs it on every push, because a pull request that only edits documentation skips ci.yml.
package doclinks

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var (
	headingRe = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)[ \t]*#*[ \t]*$`)
	fenceRe   = regexp.MustCompile("^ {0,3}(```|~~~)")
	codeRe    = regexp.MustCompile("`[^`]*`")
	inlineRe  = regexp.MustCompile(`\]\(\s*(<[^>]*>|[^)\s]*)`)
	refDefRe  = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:\s*(<[^>]*>|\S+)`)
	htmlURLRe = regexp.MustCompile(`(?:href|src)="([^"]*)"`)
	htmlIDRe  = regexp.MustCompile(`(?:id|name)="([^"]+)"`)
	linkRe    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	tagRe     = regexp.MustCompile(`<[^>]*>`)
	schemeRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// slug is GitHub's anchor for a heading: formatting removed, lower case, anything but letters, digits, "-" and "_"
// dropped (emoji with it), spaces turned into "-".
func slug(heading string) string {
	h := linkRe.ReplaceAllString(heading, "$1")
	h = codeRe.ReplaceAllStringFunc(h, func(s string) string { return s[1 : len(s)-1] })
	h = tagRe.ReplaceAllString(h, "")
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

type link struct {
	line   int
	target string
}

type doc struct {
	anchors map[string]bool
	links   []link
}

// parse reads one Markdown file: its anchors (headings, with GitHub's -1, -2 for repeats, and HTML id or name
// attributes) and its links. Fenced code and inline code are skipped.
func parse(t *testing.T, path string) *doc {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d := &doc{anchors: map[string]bool{}}
	seen := map[string]int{}
	inFence := false
	for i, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			s := slug(m[2])
			if n := seen[s]; n > 0 {
				d.anchors[s+"-"+strconv.Itoa(n)] = true
			} else {
				d.anchors[s] = true
			}
			seen[s]++
		}
		for _, m := range htmlIDRe.FindAllStringSubmatch(line, -1) {
			d.anchors[m[1]] = true
		}
		text := codeRe.ReplaceAllString(line, "")
		for _, m := range inlineRe.FindAllStringSubmatch(text, -1) {
			d.links = append(d.links, link{i + 1, m[1]})
		}
		if m := refDefRe.FindStringSubmatch(text); m != nil {
			d.links = append(d.links, link{i + 1, m[1]})
		}
		for _, m := range htmlURLRe.FindAllStringSubmatch(text, -1) {
			d.links = append(d.links, link{i + 1, m[1]})
		}
	}
	return d
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Where things are":                      "where-things-are",
		"4. Reach the people it is for, calmly": "4-reach-the-people-it-is-for-calmly",
		"🚀 Quick start":                         "-quick-start",
		"`go test` & the [CI](ci.md)":           "go-test--the-ci",
		"Open decisions (the maintainers)":      "open-decisions-the-maintainers",
		"snake_case stays":                      "snake_case-stays",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLinksResolve checks every relative link and #anchor in every Markdown file of the repository. External links
// (a scheme: https:, mailto:) are not followed.
func TestLinksResolve(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	err = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && (e.Name() == ".git" || e.Name() == "dist" || e.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !e.IsDir() && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 5 {
		t.Fatalf("only %d Markdown files found under %s", len(files), root)
	}
	docs := map[string]*doc{}
	get := func(p string) *doc {
		if d, ok := docs[p]; ok {
			return d
		}
		docs[p] = parse(t, p)
		return docs[p]
	}
	checked := 0
	for _, file := range files {
		rel, _ := filepath.Rel(root, file)
		for _, l := range get(file).links {
			target := strings.Trim(l.target, "<>")
			if target == "" || schemeRe.MatchString(target) || strings.HasPrefix(target, "//") {
				continue
			}
			checked++
			pathPart, frag, _ := strings.Cut(target, "#")
			if i := strings.IndexByte(pathPart, '?'); i >= 0 {
				pathPart = pathPart[:i]
			}
			if u, err := url.PathUnescape(pathPart); err == nil {
				pathPart = u
			}
			if u, err := url.PathUnescape(frag); err == nil {
				frag = u
			}
			dest := file
			if pathPart != "" {
				if strings.HasPrefix(pathPart, "/") {
					dest = filepath.Join(root, filepath.FromSlash(pathPart))
				} else {
					dest = filepath.Join(filepath.Dir(file), filepath.FromSlash(pathPart))
				}
				info, err := os.Stat(dest)
				if err != nil {
					t.Errorf("%s:%d: %q does not exist", filepath.ToSlash(rel), l.line, l.target)
					continue
				}
				if info.IsDir() {
					continue
				}
			}
			if frag == "" || !strings.HasSuffix(dest, ".md") {
				continue
			}
			if !get(dest).anchors[strings.ToLower(frag)] {
				t.Errorf("%s:%d: %q has no heading or anchor %q", filepath.ToSlash(rel), l.line, l.target, frag)
			}
		}
	}
	t.Logf("%d Markdown files, %d relative links checked", len(files), checked)
}

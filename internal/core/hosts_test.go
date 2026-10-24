package core

import (
	"go/scanner"
	"go/token"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var urlRE = regexp.MustCompile(`https?://[A-Za-z0-9.\-\[\]:]+`)

// notContacted is what a URL in the code may name without being an address the program reaches: examples in help
// texts, this computer and the local network, and document identifiers (XML namespaces, the plist DTD, the exe manifest).
func notContacted(host string) bool {
	h := strings.ToLower(host)
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.HasSuffix(h, "]") {
		h = h[:i]
	}
	switch {
	case !strings.Contains(h, ".") || h == "127.0.0.1" || strings.HasPrefix(h, "["):
		return true // "localhost", a placeholder ("https://user"), an IPv6 literal
	case strings.HasPrefix(h, "192.168.") || strings.HasPrefix(h, "10.") || strings.HasSuffix(h, ".local"):
		return true
	case h == "www.w3.org" || h == "www.apple.com" || h == "schemas.microsoft.com" || strings.HasSuffix(h, ".example") || strings.Contains(h, "example."):
		return true
	}
	return false
}

// urlsIn returns every http(s) address in the string literals of a Go file.
func urlsIn(t *testing.T, path string) []string {
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s scanner.Scanner
	fset := token.NewFileSet()
	s.Init(fset.AddFile(path, -1, len(src)), src, nil, 0)
	var out []string
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			return out
		}
		if tok != token.STRING {
			continue
		}
		if v, err := strconv.Unquote(lit); err == nil {
			lit = v
		}
		out = append(out, urlRE.FindAllString(lit, -1)...)
	}
}

// Every outside address the code names is in Hosts or Sources, so the list SECURITY.md shows is complete.
func TestEveryHostIsListed(t *testing.T) {
	root := filepath.Join("..", "..")
	found := map[string][]string{}
	add := func(u, where string) {
		p, err := url.Parse(u)
		if err != nil || p.Host == "" || notContacted(p.Host) {
			return
		}
		found[strings.ToLower(p.Host)] = append(found[strings.ToLower(p.Host)], where)
	}
	for _, dir := range []string{"cmd", "internal", "tools"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if dir == "tools" && !strings.Contains(filepath.ToSlash(path), "tools/mk") {
				return nil // the CI helpers never ship; mkrsrc and mkico write the program's resources
			}
			for _, u := range urlsIn(t, path) {
				add(u, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"ui.html", "ui.css", "ui.js"} {
		page, err := os.ReadFile(filepath.Join(root, "internal", "assets", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range urlRE.FindAllString(string(page), -1) {
			add(u, name)
		}
	}
	if len(found) < 10 {
		t.Fatalf("only %d hosts found; the scan is broken", len(found))
	}
	for h, where := range found {
		if _, ok := KnownHost(h); !ok {
			t.Errorf("%s (in %s) is in neither core.Hosts nor core.Sources: add it there and to SECURITY.md", h, where[0])
		}
	}
}

// SECURITY.md names every listed host, and the list has no stray entries.
func TestSecurityListsEveryHost(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "SECURITY.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	seen := map[string]bool{}
	for _, h := range Hosts {
		if seen[h.Host] {
			t.Errorf("%s is listed twice", h.Host)
		}
		seen[h.Host] = true
		if h.How != HostAsked && h.How != HostLoaded && h.How != HostOpened {
			t.Errorf("%s: unknown How %q", h.Host, h.How)
		}
		if h.Service == "" || h.When == "" {
			t.Errorf("%s: no service or no description", h.Host)
		}
		if !strings.Contains(doc, "`"+h.Host+"`") {
			t.Errorf("SECURITY.md does not name `%s`", h.Host)
		}
	}
	for _, s := range Sources {
		if !strings.Contains(doc, "`"+s.Host+"`") {
			t.Errorf("SECURITY.md does not name `%s` (%s)", s.Host, s.Label)
		}
	}
}

func TestKnownHost(t *testing.T) {
	for h, want := range map[string]string{
		"api.tvmaze.com": HostAsked, "en.wikipedia.org": HostAsked, "API.GitHub.com": HostAsked,
		"raw.githubusercontent.com": HostLoaded, "www.stremio.com": HostOpened, "kitsu.io": HostAsked,
	} {
		if got, ok := KnownHost(h); !ok || got != want {
			t.Errorf("KnownHost(%q) = %q, %v; want %q", h, got, ok, want)
		}
	}
	for _, h := range []string{"tvmaze.com.evil.test", "notgithub.com", "media-imdb.com.test"} {
		if _, ok := KnownHost(h); ok {
			t.Errorf("KnownHost(%q) = true", h)
		}
	}
}

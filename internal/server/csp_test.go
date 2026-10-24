package server

import (
	"strings"
	"testing"
)

// The preview card shows the cover from the catalog's image host: the page policy must let https images through
// (and nothing else beyond the page itself).
func TestPolicyAllowsCoverImages(t *testing.T) {
	var img string
	for _, d := range strings.Split(csp, ";") {
		if f := strings.Fields(d); len(f) > 0 && f[0] == "img-src" {
			img = strings.Join(f[1:], " ")
		}
	}
	if !strings.Contains(" "+img+" ", " https: ") {
		t.Fatalf("img-src %q blocks cover images", img)
	}
	for _, d := range []string{"frame-ancestors 'none'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, d) {
			t.Errorf("policy lacks %q", d)
		}
	}
	// no inline script or style: the page is a file, a style sheet and a script from this server
	for _, d := range []string{"style-src 'self'", "script-src 'self'"} {
		if !strings.Contains(csp, d) {
			t.Errorf("policy lacks %q", d)
		}
	}
	if strings.Contains(csp, "unsafe-") {
		t.Fatalf("policy allows inline or eval code: %q", csp)
	}
	if strings.Contains(csp, "connect-src") || strings.Contains(img, "http:") || strings.Contains(img, "*") {
		t.Fatalf("policy is wider than needed: %q", csp)
	}
}

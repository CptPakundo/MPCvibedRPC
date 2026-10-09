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
	if strings.Contains(csp, "connect-src") || strings.Contains(img, "http:") || strings.Contains(img, "*") {
		t.Fatalf("policy is wider than needed: %q", csp)
	}
}

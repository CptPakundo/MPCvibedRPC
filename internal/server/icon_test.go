package server

import (
	"strconv"
	"strings"
	"testing"
)

// The page names its icons with the version in their addresses (a browser profile keeps a page's icon and would go
// on showing an older logo), and /icon.png serves the large logo.
func TestPageIconAddresses(t *testing.T) {
	s := New(Options{Token: "tok", Page: `<link href="/icon.png?v=__VERSION__"><link href="/icon.ico?v=__VERSION__">`,
		Icon: []byte("ICO"), IconPNG: []byte("PNG"), Version: "9.9.9"})
	if _, err := s.Listen(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	base := "http://127.0.0.1:" + strconv.Itoa(s.Port())
	if code, body, _ := do(t, "GET", base+"/?t=tok", "", nil, ""); code != 200 || strings.Count(body, "?v=9.9.9") != 2 || strings.Contains(body, "__VERSION__") {
		t.Errorf("page: %d %s", code, body)
	}
	if code, body, hdr := do(t, "GET", base+"/icon.png?v=9.9.9", "", nil, ""); code != 200 || body != "PNG" || hdr.Get("Content-Type") != "image/png" {
		t.Errorf("icon.png: %d %q %s", code, body, hdr.Get("Content-Type"))
	}
	if code, body, _ := do(t, "GET", base+"/icon.ico?v=9.9.9", "", nil, ""); code != 200 || body != "ICO" {
		t.Errorf("icon.ico: %d %q", code, body)
	}
}

package assets

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// The page names both icons with the version in their addresses (a browser profile keeps a page's icon and would go
// on showing an older logo), and every size it claims for the .ico is in the file.
func TestPageIconLinks(t *testing.T) {
	for _, link := range []string{
		`<link rel="icon" type="image/png" sizes="256x256" href="/icon.png?v=__VERSION__">`,
		`href="/icon.ico?v=__VERSION__">`,
	} {
		if !strings.Contains(UI, link) {
			t.Errorf("ui.html has no %s", link)
		}
	}
	m := regexp.MustCompile(`<link rel="icon" sizes="([^"]+)" href="/icon\.ico`).FindStringSubmatch(UI)
	if m == nil {
		t.Fatal("ui.html names no sizes for icon.ico")
	}
	have := map[string]bool{}
	if len(Icon) < 6 || binary.LittleEndian.Uint16(Icon[2:]) != 1 {
		t.Fatal("icon.ico is not an icon file")
	}
	n := int(binary.LittleEndian.Uint16(Icon[4:]))
	for i := 0; i < n && 6+16*i+2 <= len(Icon); i++ {
		w, h := int(Icon[6+16*i]), int(Icon[6+16*i+1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		have[fmt.Sprintf("%dx%d", w, h)] = true
	}
	for _, size := range strings.Fields(m[1]) {
		if !have[size] {
			t.Errorf("ui.html says icon.ico holds %s; it holds %v", size, have)
		}
	}
}

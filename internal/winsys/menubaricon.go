package winsys

import (
	"bytes"
	"image/png"

	"github.com/CptPakundo/MPRP/internal/logo"
)

// menuBarIcon draws the icon for the macOS menu bar at n x n pixels (18 points on a Retina screen): the program's logo
// in one colour (logo.Template), black on transparent, which macOS recolours for light and dark menu bars. With dot,
// a filled circle in the top right corner, set apart by a transparent gap, says an update waits. It lives outside
// macapp_darwin.go so it is built and tested on every system.
func menuBarIcon(n int, dot bool) []byte {
	var out bytes.Buffer
	_ = png.Encode(&out, logo.Template(n, n/18, dot))
	return out.Bytes()
}

package main

import (
	"bytes"
	"image/png"

	"github.com/CptPakundo/MPRP/internal/logo"
)

// logoPNG is the logo as a 256 px PNG for the settings page's icon (/icon.png). The browser shows the page's icon on
// the window's taskbar button, and the .ico it would otherwise take holds only small sizes, which look blurry there.
func logoPNG() []byte {
	var b bytes.Buffer
	if png.Encode(&b, logo.Image(256, false)) != nil {
		return nil
	}
	return b.Bytes()
}

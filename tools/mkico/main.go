// mkico writes internal/assets/icon.ico: the program's logo (internal/logo, a play triangle over a progress bar on an
// indigo tile), drawn at every size Windows asks for from 100% to 250% scaling (16, 24, 32, 40, 48, 64 pixels, 32-bit
// BMP frames).
//
//	go run ./tools/mkico -out internal/assets/icon.ico
//	go run ./tools/mkico -icns MPRP.icns     (the macOS app icon, made when the app is packaged)
//	go run ./tools/mkico -png docs/images/icon.png    (a 256-pixel logo for the README)
//	go run ./tools/mkico -dot -out internal/assets/icon-update.ico   (the tray icon while an update is waiting)
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"

	"github.com/CptPakundo/MPRP/internal/logo"
)

var sizes = []int{16, 24, 32, 40, 48, 64}

func main() {
	out := flag.String("out", "internal/assets/icon.ico", "output .ico file")
	icns := flag.String("icns", "", "write a macOS .icns file here instead")
	pngFile := flag.String("png", "", "write a 256-pixel PNG here instead")
	withDot := flag.Bool("dot", false, "the .ico with the update dot in its top right corner")
	flag.Parse()
	path, data := *out, func() []byte { return build(sizes, *withDot) }
	if *icns != "" {
		path, data = *icns, BuildICNS
	}
	if *pngFile != "" {
		path, data = *pngFile, func() []byte { return pngOf(256) }
	}
	if err := os.WriteFile(path, data(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "mkico:", err)
		os.Exit(1)
	}
}

// frame returns the BMP-in-ICO payload (header, bottom-up BGRA pixels, empty AND mask) for one size.
func frame(n int, dot bool) []byte {
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, struct {
		Size          uint32
		W, H          int32
		Planes, Bits  uint16
		Comp, ImgSize uint32
		X, Y          int32
		Used, Imp     uint32
	}{40, int32(n), int32(2 * n), 1, 32, 0, uint32(4 * n * n), 0, 0, 0, 0})
	img := logo.Image(n, dot)
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			c := img.NRGBAAt(x, y)
			b.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	b.Write(make([]byte, ((n+31)/32*4)*n)) // AND mask, unused with an alpha channel
	return b.Bytes()
}

// Build makes the .ico file for the given square sizes.
func Build(sizes []int) []byte { return build(sizes, false) }

// build makes the .ico file, with the update dot when dot is set.
func build(sizes []int, dot bool) []byte {
	var frames [][]byte
	for _, n := range sizes {
		frames = append(frames, frame(n, dot))
	}
	out := new(bytes.Buffer)
	binary.Write(out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	off := 6 + 16*len(sizes)
	for i, n := range sizes {
		out.Write([]byte{byte(n), byte(n), 0, 0})
		binary.Write(out, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(out, binary.LittleEndian, [2]uint32{uint32(len(frames[i])), uint32(off)})
		off += len(frames[i])
	}
	for _, f := range frames {
		out.Write(f)
	}
	return out.Bytes()
}

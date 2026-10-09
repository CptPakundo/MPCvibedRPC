package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

// icnsTypes are the PNG entries of a macOS .icns file, by pixel size.
var icnsTypes = []struct {
	kind string
	size int
}{
	{"icp4", 16}, {"icp5", 32}, {"ic11", 32}, {"icp6", 64}, {"ic12", 64}, {"ic07", 128},
	{"ic08", 256}, {"ic13", 256}, {"ic09", 512}, {"ic14", 512}, {"ic10", 1024},
}

// pngOf draws the icon at one size.
func pngOf(n int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			r, g, b, a := pixel(n, x, y)
			img.SetNRGBA(x, y, color.NRGBA{r, g, b, a})
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

// BuildICNS makes the macOS app icon (the same drawing as the .ico).
func BuildICNS() []byte {
	cache := map[int][]byte{}
	var body bytes.Buffer
	for _, t := range icnsTypes {
		p, ok := cache[t.size]
		if !ok {
			p = pngOf(t.size)
			cache[t.size] = p
		}
		body.WriteString(t.kind)
		binary.Write(&body, binary.BigEndian, uint32(8+len(p)))
		body.Write(p)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

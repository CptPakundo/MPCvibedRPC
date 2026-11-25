package main

import (
	"bytes"
	"encoding/binary"
	"image/png"

	"github.com/CptPakundo/MPRP/internal/logo"
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
	var out bytes.Buffer
	_ = png.Encode(&out, logo.Image(n, false))
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

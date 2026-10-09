// mkico writes internal/assets/icon.ico: a blurple rounded square with a white play triangle, drawn at every
// size Windows asks for from 100% to 250% scaling (16, 24, 32, 40, 48, 64 pixels, 32-bit BMP frames).
//
//	go run ./tools/mkico -out internal/assets/icon.ico
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
)

var sizes = []int{16, 24, 32, 40, 48, 64}

func main() {
	out := flag.String("out", "internal/assets/icon.ico", "output .ico file")
	flag.Parse()
	if err := os.WriteFile(*out, Build(sizes), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "mkico:", err)
		os.Exit(1)
	}
}

// Shape in unit coordinates (0..1).
const radius = 0.15

var tri = [3][2]float64{{0.375, 0.2708}, {0.375, 0.7292}, {0.75, 0.5}}

func inRoundRect(x, y float64) bool {
	dx := math.Max(math.Max(radius-x, x-(1-radius)), 0)
	dy := math.Max(math.Max(radius-y, y-(1-radius)), 0)
	return dx*dx+dy*dy <= radius*radius
}

func inTriangle(x, y float64) bool {
	sign := func(a, b [2]float64) float64 { return (x-b[0])*(a[1]-b[1]) - (a[0]-b[0])*(y-b[1]) }
	d1, d2, d3 := sign(tri[0], tri[1]), sign(tri[1], tri[2]), sign(tri[2], tri[0])
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}

// frame returns the BMP-in-ICO payload (header, bottom-up BGRA pixels, empty AND mask) for one size.
func frame(n int) []byte {
	const ss = 8
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, struct {
		Size          uint32
		W, H          int32
		Planes, Bits  uint16
		Comp, ImgSize uint32
		X, Y          int32
		Used, Imp     uint32
	}{40, int32(n), int32(2 * n), 1, 32, 0, uint32(4 * n * n), 0, 0, 0, 0})
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			var cover, white float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					u, v := (float64(x)+(float64(sx)+0.5)/ss)/float64(n), (float64(y)+(float64(sy)+0.5)/ss)/float64(n)
					if inRoundRect(u, v) {
						cover++
						if inTriangle(u, v) {
							white++
						}
					}
				}
			}
			a := cover / (ss * ss)
			if cover == 0 {
				b.Write([]byte{0, 0, 0, 0})
				continue
			}
			w := white / cover
			ch := func(blue, full float64) byte { return byte(math.Round(blue + (full-blue)*w)) }
			b.Write([]byte{ch(242, 255), ch(101, 255), ch(88, 255), byte(math.Round(255 * a))}) // B, G, R, A
		}
	}
	b.Write(make([]byte, ((n+31)/32*4)*n)) // AND mask, unused with an alpha channel
	return b.Bytes()
}

// Build makes the .ico file for the given square sizes.
func Build(sizes []int) []byte {
	var frames [][]byte
	for _, n := range sizes {
		frames = append(frames, frame(n))
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

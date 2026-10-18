package winsys

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// menuBarIcon draws the icon for the macOS menu bar at n x n pixels (18 points on a Retina screen): the program's
// rounded square with the play triangle cut out, in black on transparent, which macOS recolours for light and dark
// menu bars. With dot, a filled circle in the top right corner, set apart by a transparent gap, says an update waits.
// It lives outside macapp_darwin.go so it is built and tested on every system.
func menuBarIcon(n int, dot bool) []byte {
	const ss, radius, inset = 8, 0.2, 0.06
	const dotX, dotY, dotR, gap = 0.8, 0.2, 0.2, 0.08
	tri := [3][2]float64{{0.39, 0.29}, {0.39, 0.71}, {0.73, 0.5}}
	inRect := func(x, y float64) bool {
		x, y = (x-inset)/(1-2*inset), (y-inset)/(1-2*inset)
		if x < 0 || y < 0 || x > 1 || y > 1 {
			return false
		}
		dx := math.Max(math.Max(radius-x, x-(1-radius)), 0)
		dy := math.Max(math.Max(radius-y, y-(1-radius)), 0)
		return dx*dx+dy*dy <= radius*radius
	}
	inTri := func(x, y float64) bool {
		s := func(a, b [2]float64) float64 { return (x-b[0])*(a[1]-b[1]) - (a[0]-b[0])*(y-b[1]) }
		d1, d2, d3 := s(tri[0], tri[1]), s(tri[1], tri[2]), s(tri[2], tri[0])
		return !((d1 < 0 || d2 < 0 || d3 < 0) && (d1 > 0 || d2 > 0 || d3 > 0))
	}
	inCircle := func(x, y, r float64) bool { return (x-dotX)*(x-dotX)+(y-dotY)*(y-dotY) <= r*r }
	filled := func(u, v float64) bool {
		switch {
		case dot && inCircle(u, v, dotR):
			return true
		case dot && inCircle(u, v, dotR+gap):
			return false
		}
		return inRect(u, v) && !inTri(u, v)
	}
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			cover := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					u, v := (float64(x)+(float64(sx)+0.5)/ss)/float64(n), (float64(y)+(float64(sy)+0.5)/ss)/float64(n)
					if filled(u, v) {
						cover++
					}
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, uint8(math.Round(255 * float64(cover) / (ss * ss)))})
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

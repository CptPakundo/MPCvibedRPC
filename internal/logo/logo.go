// Package logo draws the program's icon, "the status card": a play triangle over a two-tone progress bar on an indigo
// tile. tools/mkico makes the Windows icons, the macOS app icon and the README's picture from Image; the macOS menu
// bar shows Template.
//
// The shapes are on a 256-unit grid whose 16-unit steps are the pixels of the 16-pixel icon, and every test is done in
// whole numbers, so the same pixels come out on every system (the release build compares the icons byte for byte).
package logo

import "image"

// The colours, from the settings window's tokens in ui.css.
var (
	tileColour    = rgb{0x47, 0x52, 0xc4} // --accent (light theme)
	whiteColour   = rgb{0xff, 0xff, 0xff} // --on-accent: the triangle and the elapsed part of the bar
	barRestColour = rgb{0xa5, 0xac, 0xf8} // --accent-text (dark theme): the rest of the bar
	dotColour     = rgb{0xd1, 0x3a, 0x3a} // --bad-fill-hover (dark theme): the update dot
)

type rgb struct{ r, g, b int }

// shape is one part of the drawing. A sample takes the colour (and the template's opacity, 0-255) of the last shape
// that covers it.
type shape struct {
	in     func(p point) bool
	colour rgb
	mono   int
}

// The drawing, in order. The template keeps the tile and cuts the triangle and the elapsed bar out of it; the rest of
// the bar is cut to about 45 %.
var shapes = []shape{
	{roundRect(0, 0, 256, 256, 40), tileColour, 255},
	{triangle(96, 48, 96, 160, 176, 104), whiteColour, 0},
	{roundRect(48, 176, 208, 208, 16), barRestColour, 140},
	{roundRect(48, 176, 136, 208, 16), whiteColour, 0},
}

// The update dot: a red circle in the top right corner, set apart from the drawing by a transparent ring.
const dotX, dotY, dotR, dotRing = 208, 48, 44, 14

// point is a sample's place on the grid, as x/d and y/d units.
type point struct{ x, y, d int64 }

func roundRect(x0, y0, x1, y1, r int64) func(point) bool {
	return func(p point) bool {
		dx := max((x0+r)*p.d-p.x, p.x-(x1-r)*p.d, 0)
		dy := max((y0+r)*p.d-p.y, p.y-(y1-r)*p.d, 0)
		return dx*dx+dy*dy <= r*r*p.d*p.d
	}
}

func triangle(ax, ay, bx, by, cx, cy int64) func(point) bool {
	return func(p point) bool {
		side := func(ax, ay, bx, by int64) int64 {
			return (p.x-bx*p.d)*(ay-by)*p.d - (ax-bx)*p.d*(p.y-by*p.d)
		}
		d1, d2, d3 := side(ax, ay, bx, by), side(bx, by, cx, cy), side(cx, cy, ax, ay)
		return !((d1 < 0 || d2 < 0 || d3 < 0) && (d1 > 0 || d2 > 0 || d3 > 0))
	}
}

func inCircle(p point, cx, cy, r int64) bool {
	dx, dy := p.x-cx*p.d, p.y-cy*p.d
	return dx*dx+dy*dy <= r*r*p.d*p.d
}

// sample is what covers one sample: its colour and template opacity, and whether anything does.
func sample(p point, dot bool) (c rgb, mono int, covered bool) {
	if dot {
		if inCircle(p, dotX, dotY, dotR) {
			return dotColour, 255, true
		}
		if inCircle(p, dotX, dotY, dotR+dotRing) {
			return rgb{}, 0, false
		}
	}
	for _, s := range shapes {
		if s.in(p) {
			c, mono, covered = s.colour, s.mono, true
		}
	}
	return c, mono, covered
}

// ss is the samples per pixel along each side.
const ss = 8

// render draws the logo at n x n pixels into img at (ox, oy); each pixel is the average of its ss x ss samples.
func render(img *image.NRGBA, ox, oy, n int, dot, template bool) {
	d := int64(2 * ss * n) // a sample's middle is at (2*(ss*x+sx)+1)/(2*ss*n) of the side, times 256
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, cover, alpha int
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					p := point{int64(256 * (2*(ss*x+sx) + 1)), int64(256 * (2*(ss*y+sy) + 1)), d}
					c, mono, ok := sample(p, dot)
					if !ok {
						continue
					}
					cover++
					alpha += mono
					r, g, b = r+c.r, g+c.g, b+c.b
				}
			}
			i := img.PixOffset(ox+x, oy+y)
			px := img.Pix[i : i+4 : i+4]
			if template {
				px[0], px[1], px[2], px[3] = 0, 0, 0, uint8(div(alpha, ss*ss))
				continue
			}
			if cover == 0 {
				continue
			}
			px[0], px[1], px[2], px[3] = uint8(div(r, cover)), uint8(div(g, cover)), uint8(div(b, cover)), uint8(div(255*cover, ss*ss))
		}
	}
}

// div is a/b rounded to the nearest whole number (a, b >= 0).
func div(a, b int) int { return (2*a + b) / (2 * b) }

// Image is the icon at n x n pixels; with dot, the update dot in the top right corner (the tray while an update waits).
func Image(n int, dot bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	render(img, 0, 0, n, dot, false)
	return img
}

// Template is the one-colour icon for the macOS menu bar: black, with the shape in the alpha channel, which macOS
// recolours for light and dark menu bars. It is n x n pixels with the drawing pad pixels in from each side.
func Template(n, pad int, dot bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	render(img, pad, pad, n-2*pad, dot, true)
	return img
}

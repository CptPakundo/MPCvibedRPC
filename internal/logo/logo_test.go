package logo

import (
	"image/color"
	"testing"
)

// The drawing at 256 pixels, one pixel per grid unit, against the shapes of the design (concept-1.svg).
func TestImageShapes(t *testing.T) {
	img := Image(256, false)
	for _, c := range []struct {
		x, y int
		want color.NRGBA
		what string
	}{
		{128, 20, color.NRGBA{0x47, 0x52, 0xc4, 255}, "the tile"},
		{120, 104, color.NRGBA{255, 255, 255, 255}, "the triangle"},
		{80, 192, color.NRGBA{255, 255, 255, 255}, "the elapsed part of the bar"},
		{180, 192, color.NRGBA{0xa5, 0xac, 0xf8, 255}, "the rest of the bar"},
		{1, 1, color.NRGBA{}, "outside the rounded corner"},
	} {
		if got := img.NRGBAAt(c.x, c.y); got != c.want {
			t.Errorf("%s at (%d,%d): %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
	dot := Image(256, true)
	if got := dot.NRGBAAt(208, 48); got != (color.NRGBA{0xd1, 0x3a, 0x3a, 255}) {
		t.Errorf("the update dot: %v", got)
	}
	if got := dot.NRGBAAt(208, 100); got.A != 0 { // 52 units below its middle: the ring
		t.Errorf("the ring around the dot: %v", got)
	}
	if dot.NRGBAAt(60, 220) != img.NRGBAAt(60, 220) {
		t.Error("the dot changes the rest of the icon")
	}
}

// At 16 pixels the shapes land on whole pixels: the triangle's flat side and the bar are sharp.
func TestImageIsSharpAt16(t *testing.T) {
	img := Image(16, false)
	if a := img.NRGBAAt(6, 4); a != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("the triangle's first column: %v", a)
	}
	if a := img.NRGBAAt(5, 11); a != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("the bar's elapsed part: %v", a)
	}
	if a := img.NRGBAAt(11, 12); a != (color.NRGBA{0xa5, 0xac, 0xf8, 255}) {
		t.Errorf("the bar's rest: %v", a)
	}
	if a := img.NRGBAAt(5, 10); a != (color.NRGBA{0x47, 0x52, 0xc4, 255}) {
		t.Errorf("the row between the triangle and the bar: %v", a)
	}
}

// The menu bar's template: the tile is solid, the triangle and the elapsed bar are cut out, the rest of the bar is
// partly cut, and the drawing sits pad pixels in.
func TestTemplate(t *testing.T) {
	img := Template(36, 2, false)
	alpha := func(x, y int) uint8 { return img.NRGBAAt(x, y).A }
	if a := alpha(0, 18); a != 0 {
		t.Errorf("the margin: %d", a)
	}
	if a := alpha(18, 4); a != 255 {
		t.Errorf("the tile: %d", a)
	}
	if a := alpha(16, 15); a != 0 {
		t.Errorf("the triangle: %d", a)
	}
	if a := alpha(10, 26); a != 0 {
		t.Errorf("the elapsed bar: %d", a)
	}
	if a := alpha(24, 26); a != 140 {
		t.Errorf("the rest of the bar: %d", a)
	}
	for y := 0; y < 36; y++ {
		for x := 0; x < 36; x++ {
			if c := img.NRGBAAt(x, y); c.R != 0 || c.G != 0 || c.B != 0 {
				t.Fatalf("(%d,%d) is not black: %v", x, y, c)
			}
		}
	}
}

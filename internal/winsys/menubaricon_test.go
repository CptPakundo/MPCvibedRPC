package winsys

import (
	"bytes"
	"image/png"
	"testing"
)

// The macOS menu bar icon with the update dot: the dot is solid, a gap sets it apart from the square, and the rest
// of the icon is the plain one.
func TestMenuBarIconDot(t *testing.T) {
	plain, dotted := menuBarIcon(36, false), menuBarIcon(36, true)
	if bytes.Equal(plain, dotted) {
		t.Fatal("the dot is missing")
	}
	alpha := func(b []byte, x, y int) uint32 {
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if w := img.Bounds().Dx(); w != 36 {
			t.Fatalf("%d px wide", w)
		}
		_, _, _, a := img.At(x, y).RGBA()
		return a >> 8
	}
	if a := alpha(dotted, 29, 7); a != 255 { // the dot's middle
		t.Errorf("dot alpha %d", a)
	}
	if a := alpha(dotted, 22, 13); a != 0 { // in the gap (part of the square in the plain icon)
		t.Errorf("gap alpha %d", a)
	}
	if a := alpha(plain, 22, 13); a != 255 {
		t.Errorf("plain icon at the gap: alpha %d", a)
	}
	if alpha(plain, 6, 30) != alpha(dotted, 6, 30) { // far from the dot
		t.Error("the rest of the icon changed")
	}
}

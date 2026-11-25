package main

import (
	"bytes"
	"image/png"
	"testing"
)

// The page's 256 px icon (/icon.png) is a PNG of that size.
func TestLogoPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(logoPNG()))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Errorf("icon.png is %dx%d, want 256x256", b.Dx(), b.Dy())
	}
}

package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"testing"
)

// The committed icon must be what the generator produces, so it can be rebuilt at any time.
func TestCommittedIconMatchesGenerator(t *testing.T) {
	have, err := os.ReadFile("../../internal/assets/icon.ico")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(have, Build(sizes)) {
		t.Fatal("internal/assets/icon.ico is out of date: run go run ./tools/mkico")
	}
}

func TestICNS(t *testing.T) {
	b := BuildICNS()
	if string(b[:4]) != "icns" || int(binary.BigEndian.Uint32(b[4:])) != len(b) {
		t.Fatalf("bad header %q", b[:8])
	}
	n := 0
	for off := 8; off < len(b); n++ {
		size := int(binary.BigEndian.Uint32(b[off+4:]))
		img, err := png.Decode(bytes.NewReader(b[off+8 : off+size]))
		if err != nil {
			t.Fatalf("entry %q: %v", b[off:off+4], err)
		}
		if w := img.Bounds().Dx(); w != icnsTypes[n].size {
			t.Errorf("entry %q is %d px, want %d", b[off:off+4], w, icnsTypes[n].size)
		}
		off += size
	}
	if n != len(icnsTypes) {
		t.Errorf("%d entries, want %d", n, len(icnsTypes))
	}
}

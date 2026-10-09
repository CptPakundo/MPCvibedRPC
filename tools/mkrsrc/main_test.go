package main

import (
	"bytes"
	"debug/pe"
	"os"
	"testing"
)

func TestBuildProducesValidObject(t *testing.T) {
	ico, err := os.ReadFile("../../internal/assets/icon.ico")
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Build(ico, "3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	f, err := pe.NewFile(bytes.NewReader(obj))
	if err != nil {
		t.Fatal(err)
	}
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 || len(f.Sections) != 1 || f.Sections[0].Name != ".rsrc" {
		t.Fatalf("unexpected object: machine %x, %d sections", f.Machine, len(f.Sections))
	}
	// 6 icon images + the icon group + the version block + the manifest, each with one relocation
	if f.Sections[0].NumberOfRelocations != 9 {
		t.Fatalf("relocations: %d", f.Sections[0].NumberOfRelocations)
	}
	if _, err := Build(ico, "x"); err == nil {
		t.Fatal("bad version should be rejected")
	}
	if _, err := Build([]byte("nope"), "1.0.0"); err == nil {
		t.Fatal("bad icon should be rejected")
	}
}

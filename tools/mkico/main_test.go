package main

import (
	"bytes"
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

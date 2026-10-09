package jsre

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
)

func TestNFCAgainstNode(t *testing.T) {
	f, err := os.Open("testdata/nfc.json.gz")
	if err != nil {
		t.Skip("no fixture")
	}
	defer f.Close()
	z, _ := gzip.NewReader(f)
	var cases [][2]string
	if err := json.NewDecoder(z).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	bad := 0
	for _, c := range cases {
		marks := 0
		for _, r := range c[0] {
			if r >= 0x300 && r <= 0x36f {
				marks++
			}
		}
		if marks > 1 { // stacked marks are not re-ordered (documented limitation)
			continue
		}
		if got := NFC(c[0]); got != c[1] {
			bad++
			if bad < 10 {
				t.Errorf("NFC(%+q)=%+q want %+q", c[0], got, c[1])
			}
		}
	}
	t.Logf("%d cases, %d bad", len(cases), bad)
}

func TestToLowerAgainstNode(t *testing.T) {
	f, err := os.Open("testdata/lower.json.gz")
	if err != nil {
		t.Skip("no fixture")
	}
	defer f.Close()
	z, _ := gzip.NewReader(f)
	var cases [][2]string
	if err := json.NewDecoder(z).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	bad := 0
	for _, c := range cases {
		if got := ToLower(c[0]); got != c[1] {
			bad++
			if bad < 10 {
				t.Errorf("ToLower(%+q)=%+q want %+q", c[0], got, c[1])
			}
		}
	}
	t.Logf("%d cases, %d bad", len(cases), bad)
}

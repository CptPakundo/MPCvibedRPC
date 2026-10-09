package jsonx

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
)

func TestRoundTripAgainstNode(t *testing.T) {
	f, err := os.Open("testdata.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, _ := gzip.NewReader(f)
	var cases [][2]string
	if err := json.NewDecoder(z).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	bad := 0
	for _, c := range cases {
		v, err := Parse(c[0])
		if err != nil {
			t.Fatalf("parse %s: %v", c[0], err)
		}
		if got := Stringify(v, 2); got != c[1] {
			bad++
			if bad < 5 {
				t.Errorf("stringify mismatch\n got  %q\n want %q", got, c[1])
			}
		}
		if got := Stringify(v, 0); got != c[0] {
			bad++
			if bad < 5 {
				t.Errorf("compact mismatch\n got  %q\n want %q", got, c[0])
			}
		}
	}
	t.Logf("%d cases, %d bad", len(cases), bad)
}

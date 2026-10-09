package winsys

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetIniValueAgainstJS(t *testing.T) {
	f, err := os.Open("testdata/ini.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := gzip.NewReader(f)
	var cases []struct{ Text, Sec, Key, Value, Out string }
	if err := json.NewDecoder(zr).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	bad := 0
	for i, c := range cases {
		if got := SetIniValue(c.Text, c.Sec, c.Key, c.Value); got != c.Out {
			bad++
			if bad < 5 {
				t.Errorf("case %d: %q [%s] %s=%s\n got %q\nwant %q", i, c.Text, c.Sec, c.Key, c.Value, got, c.Out)
			}
		}
	}
	if len(cases) < 500 {
		t.Fatal("too few cases")
	}
}

func TestIniFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	for _, u16 := range []bool{false, true} {
		p := filepath.Join(dir, "x.ini")
		if err := WriteIni(p, "[Settings]\r\nLanguage=ä\r\n", u16); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(p)
		if u16 != bytes.HasPrefix(raw, []byte{0xff, 0xfe}) {
			t.Fatal("BOM wrong")
		}
		txt, gotU16, err := ReadIni(p)
		if err != nil || gotU16 != u16 || txt != "[Settings]\r\nLanguage=ä\r\n" {
			t.Fatalf("%q %v %v", txt, gotU16, err)
		}
	}
}

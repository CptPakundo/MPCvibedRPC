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
func TestGetIniValue(t *testing.T) {
	text := "\ufeff[Settings]\r\nEnableWebServer=1\r\n[WebServer]\r\n EnableWebServer = 0 \r\nPort=13581\r\n"
	for _, c := range []struct {
		sec, key, want string
		ok             bool
	}{
		{"Settings", "EnableWebServer", "1", true},
		{"webserver", "enablewebserver", "0", true},
		{"WebServer", "Port", "13581", true},
		{"WebServer", "LocalhostOnly", "", false},
		{"Missing", "Port", "", false},
	} {
		if v, ok := GetIniValue(text, c.sec, c.key); v != c.want || ok != c.ok {
			t.Errorf("[%s] %s = %q %v, want %q %v", c.sec, c.key, v, ok, c.want, c.ok)
		}
	}
	// what SetIniValue writes reads back
	out := SetIniValue(SetIniValue("", "WebServer", "LocalhostOnly", "1"), "WebServer", "Port", "1")
	if v, _ := GetIniValue(out, "WebServer", "LocalhostOnly"); v != "1" {
		t.Errorf("round trip: %q", out)
	}
}

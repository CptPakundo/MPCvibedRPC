package winsys

import (
	"os"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

var (
	reEOL     = regexp.MustCompile(`\r?\n`)
	reSection = regexp.MustCompile(`^\s*\[.*\]\s*$`)
)

// SetIniValue sets key=value inside [section] of INI text, adding the section/key when missing.
func SetIniValue(text, section, key, value string) string {
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	var lines []string
	if len(text) > 0 {
		lines = reEOL.Split(text, -1)
	}
	head := core.LowerJS("[" + section + "]")
	s := -1
	for i, l := range lines {
		if core.LowerJS(core.Trim(l)) == head {
			s = i
			break
		}
	}
	if s < 0 {
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, "["+section+"]", key+"="+value, "")
		return strings.Join(lines, eol)
	}
	e := len(lines)
	for i := s + 1; i < len(lines); i++ {
		if reSection.MatchString(lines[i]) {
			e = i
			break
		}
	}
	re := regexp.MustCompile(`(?i)^\s*` + regexp.QuoteMeta(key) + `\s*=`)
	for i := s + 1; i < e; i++ {
		if re.MatchString(lines[i]) {
			lines[i] = key + "=" + value
			return strings.Join(lines, eol)
		}
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:s+1]...)
	out = append(out, key+"="+value)
	out = append(out, lines[s+1:]...)
	return strings.Join(out, eol)
}

// GetIniValue reads key inside [section] of INI text (both matched without regard to case).
func GetIniValue(text, section, key string) (string, bool) {
	head := core.LowerJS("[" + section + "]")
	in := false
	for _, l := range reEOL.Split(strings.TrimPrefix(text, "\ufeff"), -1) {
		t := core.Trim(l)
		if reSection.MatchString(l) {
			in = core.LowerJS(t) == head
			continue
		}
		if !in {
			continue
		}
		if k, v, ok := strings.Cut(t, "="); ok && core.LowerJS(core.Trim(k)) == core.LowerJS(key) {
			return core.Trim(v), true
		}
	}
	return "", false
}

// ReadIni reads an INI file that is either UTF-8 or UTF-16 with a BOM (MPC-HC writes the latter).
func ReadIni(file string) (text string, utf16le bool, err error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return "", false, err
	}
	if len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe {
		b = b[2:]
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
		}
		return string(utf16.Decode(u)), true, nil
	}
	return string(b), false, nil
}

// WriteIni is the reverse of ReadIni.
func WriteIni(file, text string, asUTF16 bool) error {
	if !asUTF16 {
		return os.WriteFile(file, []byte(text), 0o644)
	}
	u := utf16.Encode([]rune(text))
	b := make([]byte, 0, 2+2*len(u))
	b = append(b, 0xff, 0xfe)
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return os.WriteFile(file, b, 0o644)
}

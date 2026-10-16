package winsys

import (
	"os"
	"path/filepath"
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
	head := core.Lower("[" + section + "]")
	s := -1
	for i, l := range lines {
		if core.Lower(core.Trim(l)) == head {
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
	head := core.Lower("[" + section + "]")
	in := false
	for _, l := range reEOL.Split(strings.TrimPrefix(text, "\ufeff"), -1) {
		t := core.Trim(l)
		if reSection.MatchString(l) {
			in = core.Lower(t) == head
			continue
		}
		if !in {
			continue
		}
		if k, v, ok := strings.Cut(t, "="); ok && core.Lower(core.Trim(k)) == core.Lower(key) {
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

// writeFileAtomic writes a player's config file through a temporary file in the same folder and a rename, so a crash
// or a full disk never leaves half a file. The file keeps its permissions; a read-only file, or a folder that does not
// allow the temporary file, is written in place as before.
func writeFileAtomic(file string, data []byte) error {
	perm := os.FileMode(0o644)
	if r, err := filepath.EvalSymlinks(file); err == nil {
		file = r
	}
	if st, err := os.Stat(file); err == nil {
		perm = st.Mode().Perm()
		if perm&0o200 == 0 {
			return os.WriteFile(file, data, perm)
		}
	}
	tmp := file + ".mpcvibedrpc.tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		_ = os.Remove(tmp) // a part may have been written
		return os.WriteFile(file, data, perm)
	}
	if err := os.Rename(tmp, file); err != nil {
		_ = os.Remove(tmp)
		return os.WriteFile(file, data, perm)
	}
	return nil
}

// WriteIni is the reverse of ReadIni.
func WriteIni(file, text string, asUTF16 bool) error {
	if !asUTF16 {
		return writeFileAtomic(file, []byte(text))
	}
	u := utf16.Encode([]rune(text))
	b := make([]byte, 0, 2+2*len(u))
	b = append(b, 0xff, 0xfe)
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return writeFileAtomic(file, b)
}

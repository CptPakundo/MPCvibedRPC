package core

import (
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsre"
)

// NA marks "no value" for the optional numbers (season, episode, year, ...), like null/undefined in JavaScript.
const NA = math.MinInt32

func has(n int) bool { return n != NA }

func re(pattern, flags string) *jsre.Regexp { return jsre.MustCompile(pattern, flags) }

func pad2n(n int) string { return pad2s(strconv.Itoa(n)) }
func pad2s(s string) string {
	if len(s) < 2 {
		return strings.Repeat("0", 2-len(s)) + s
	}
	return s
}

// atoi is +"12" for strings that are all digits (or have leading digits).
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

var firstDigits = re(`\d+`, "")

// digitsOf is +t.match(/\d+/)[0].
func digitsOf(t string) int {
	m := firstDigits.Exec(t)
	if m == nil {
		return 0
	}
	return atoi(m.Str(0))
}

// u16len / u16slice give JavaScript's string length and slice (UTF-16 code units).
func u16len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func u16slice(s string, from, to int) string {
	u := utf16.Encode([]rune(s))
	if to > len(u) {
		to = len(u)
	}
	if from >= to {
		return ""
	}
	return string(utf16.Decode(u[from:to]))
}

// Lower / Upper follow JavaScript's toLowerCase / toUpperCase for the cases that differ from Go's simple mapping.
func Lower(s string) string { return jsre.ToLower(s) }

func upperRune(r rune) string {
	if r == 'ß' {
		return "SS"
	}
	return string(unicode.ToUpper(r))
}

func firstRuneUpper(w string) string {
	if w == "" {
		return w
	}
	rs := []rune(w)
	return upperRune(rs[0]) + string(rs[1:])
}

// splitFilter is s.split(sep).filter(Boolean).
func splitFilter(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// fields is s.split(/\s+/).filter(Boolean) using JavaScript's whitespace set.
func fields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
			return true
		}
		return r >= 0x2000 && r <= 0x200a
	})
}

// trim is String.prototype.trim (JavaScript whitespace).
func trim(s string) string {
	isWS := func(r rune) bool {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
			return true
		}
		return r >= 0x2000 && r <= 0x200a
	}
	return strings.TrimFunc(s, isWS)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// normKey is the comparison key used all over: accents stripped, lower-cased, letters and digits only.
var nonAlnum = re(`[^\p{L}\p{N}]+`, "gu")

func normKey(s string) string { return nonAlnum.ReplaceStr(Lower(jsre.StripAccents(s)), "") }

// normText is the same without accent stripping.
func normText(s string) string { return nonAlnum.ReplaceStr(Lower(s), "") }

func itoa(n int) string { return strconv.Itoa(n) }

// parseIntJS mimics parseInt(s, 10): leading whitespace, optional sign, digits; ok=false when NaN.
func parseIntJS(s string) (int, bool) {
	s = strings.TrimLeft(s, " \t\r\n")
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	n, any := 0, false
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		any = true
		if n < 1<<40 {
			n = n*10 + int(c-'0')
		}
	}
	if !any {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}

// parseFloat mimics parseFloat(s); NaN becomes 0 (callers test > 0).
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	end, i := 0, 0
	seenDot, seenDigit := false, false
loop:
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
			end = i + 1
		case c == '.' && !seenDot:
			seenDot = true
		case (c == '-' || c == '+') && i == 0:
		default:
			break loop
		}
	}
	if !seenDigit {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSuffix(s[:end], "."), 64)
	if err != nil {
		return 0
	}
	return f
}

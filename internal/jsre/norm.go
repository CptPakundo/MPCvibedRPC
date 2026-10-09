package jsre

import (
	"strings"
	"unicode"
)

// NFD is s.normalize("NFD"): canonical decomposition (so "é" becomes "e" + U+0301). Combining marks are not
// re-ordered, which only matters for strings stacking several marks of different classes.
func NFD(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	var b strings.Builder
	var emit func(r rune)
	emit = func(r rune) {
		if r >= 0xAC00 && r <= 0xD7A3 { // Hangul syllable
			const sBase, lBase, vBase, tBase = 0xAC00, 0x1100, 0x1161, 0x11A7
			const vCount, tCount = 21, 28
			si := int(r - sBase)
			b.WriteRune(rune(lBase + si/(vCount*tCount)))
			b.WriteRune(rune(vBase + (si%(vCount*tCount))/tCount))
			if t := si % tCount; t != 0 {
				b.WriteRune(rune(tBase + t))
			}
			return
		}
		if d, ok := nfdTable[r]; ok {
			b.WriteString(d) // the table already holds the full (recursive) decomposition
			return
		}
		b.WriteRune(r)
	}
	for _, r := range s {
		emit(r)
	}
	return b.String()
}

// StripAccents removes combining diacritical marks (U+0300-U+036F) after decomposition: "Café" -> "Cafe".
func StripAccents(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0x300 && r <= 0x36f {
			return -1
		}
		return r
	}, NFD(s))
}

// ToLower is String.prototype.toLowerCase: Go's simple mapping plus the two context rules that differ
// (a final capital sigma becomes ς; İ becomes i + U+0307).
func ToLower(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(s)
	}
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		switch {
		case r == 'Σ':
			if finalSigma(rs, i) {
				b.WriteRune('ς')
			} else {
				b.WriteRune('σ')
			}
		case r == 0x130:
			b.WriteString("i̇")
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// NFC is s.normalize("NFC") for the text this program handles: it decomposes, then recomposes each base letter with
// the mark that directly follows it (marks stacked in unusual orders are not re-ordered first).
func NFC(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	in := []rune(NFD(s))
	out := make([]rune, 0, len(in))
	for _, r := range in {
		if n := len(out); n > 0 {
			p := out[n-1]
			if c, ok := nfcPairs[[2]rune{p, r}]; ok {
				out[n-1] = c
				continue
			}
			// Hangul: L+V -> LV, LV+T -> LVT
			if p >= 0x1100 && p < 0x1113 && r >= 0x1161 && r < 0x1176 {
				out[n-1] = 0xAC00 + ((p-0x1100)*21+(r-0x1161))*28
				continue
			}
			if p >= 0xAC00 && p <= 0xD7A3 && (p-0xAC00)%28 == 0 && r > 0x11A7 && r < 0x11C3 {
				out[n-1] = p + (r - 0x11A7)
				continue
			}
		}
		out = append(out, r)
	}
	return string(out)
}

func isCased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) || unicode.Is(unicode.Other_Lowercase, r) || unicode.Is(unicode.Other_Uppercase, r)
}

func isCaseIgnorable(r rune) bool {
	switch r {
	case 0x27, 0x2E, 0x3A, 0xB7, 0x387, 0x55F, 0x5F4, 0x2018, 0x2019, 0x2024, 0x2027, 0xFE13, 0xFE52, 0xFE55, 0xFF07, 0xFF0E, 0xFF1A:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

// finalSigma: Σ at i is word-final (a cased letter before it, no cased letter after it, ignoring case-ignorable marks).
func finalSigma(rs []rune, i int) bool {
	j := i - 1
	for j >= 0 && isCaseIgnorable(rs[j]) {
		j--
	}
	if j < 0 || !isCased(rs[j]) {
		return false
	}
	k := i + 1
	for k < len(rs) && isCaseIgnorable(rs[k]) {
		k++
	}
	return !(k < len(rs) && isCased(rs[k]))
}

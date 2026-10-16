package artwork

import (
	"bytes"
	"encoding/json"
	"strings"
)

const hexU = "0123456789ABCDEF"

func escapeWith(s string, keep func(b byte) bool, space bool) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case keep(c):
			sb.WriteByte(c)
		case space && c == ' ':
			sb.WriteByte('+')
		default:
			sb.WriteByte('%')
			sb.WriteByte(hexU[c>>4])
			sb.WriteByte(hexU[c&15])
		}
	}
	return sb.String()
}

func alnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// encodeURIComponent
func encURI(s string) string {
	return escapeWith(toWellFormed(s), func(c byte) bool { return alnum(c) || strings.IndexByte("-_.!~*'()", c) >= 0 }, false)
}

// formEnc is what URLSearchParams.toString does to each name and value.
func formEnc(s string) string {
	return escapeWith(toWellFormed(s), func(c byte) bool { return alnum(c) || strings.IndexByte("*-._", c) >= 0 }, true)
}

func toWellFormed(s string) string { return strings.ToValidUTF8(s, "�") }

// params mimics URLSearchParams (ordered; set replaces).
type params [][2]string

func (p *params) set(k, v string) {
	for i := range *p {
		if (*p)[i][0] == k {
			(*p)[i][1] = v
			return
		}
	}
	*p = append(*p, [2]string{k, v})
}

func (p params) String() string {
	parts := make([]string, len(p))
	for i, kv := range p {
		parts[i] = formEnc(kv[0]) + "=" + formEnc(kv[1])
	}
	return strings.Join(parts, "&")
}

// jsonString is JSON.stringify for plain values (no HTML escaping; Go writes U+2028/2029 as \u escapes, which is valid JSON).
func jsonString(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(buf.String(), "\n")
}

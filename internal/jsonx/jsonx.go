// Package jsonx is an order-preserving JSON reader/writer that behaves like JavaScript's JSON.parse / stringify:
// config.json keeps the user's key order, and catalog responses can be walked like plain JS objects.
package jsonx

import (
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Obj is a JSON object that remembers its key order.
type Obj struct {
	Keys []string
	M    map[string]any
}

// Values is Object.values: integer-like keys ascending first, then the others in insertion order.
func (o *Obj) Values() []any {
	var ints, rest []string
	for _, k := range o.Keys {
		if isArrayIndex(k) {
			ints = append(ints, k)
		} else {
			rest = append(rest, k)
		}
	}
	sort.SliceStable(ints, func(i, j int) bool {
		a, _ := strconv.ParseUint(ints[i], 10, 64)
		b, _ := strconv.ParseUint(ints[j], 10, 64)
		return a < b
	})
	var out []any
	for _, k := range append(ints, rest...) {
		out = append(out, o.M[k])
	}
	return out
}

func isArrayIndex(k string) bool {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return false
	}
	for _, c := range k {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, _ := strconv.ParseUint(k, 10, 64)
	return n < 4294967295
}

type jparser struct {
	s   string
	pos int
}

var ErrSyntax = errors.New("Unexpected token in JSON")

// Parse decodes into nil, bool, float64, string, []any and *Obj.
func Parse(s string) (any, error) {
	p := &jparser{s: s}
	p.ws()
	v, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.ws()
	if p.pos != len(p.s) {
		return nil, ErrSyntax
	}
	return v, nil
}

func (p *jparser) ws() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *jparser) value(depth int) (any, error) {
	if depth > 200 || p.pos >= len(p.s) {
		return nil, ErrSyntax
	}
	switch c := p.s[p.pos]; {
	case c == '{':
		p.pos++
		o := &Obj{M: map[string]any{}}
		p.ws()
		if p.pos < len(p.s) && p.s[p.pos] == '}' {
			p.pos++
			return o, nil
		}
		for {
			p.ws()
			if p.pos >= len(p.s) || p.s[p.pos] != '"' {
				return nil, ErrSyntax
			}
			k, err := p.str()
			if err != nil {
				return nil, err
			}
			p.ws()
			if p.pos >= len(p.s) || p.s[p.pos] != ':' {
				return nil, ErrSyntax
			}
			p.pos++
			p.ws()
			v, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			if _, dup := o.M[k]; !dup {
				o.Keys = append(o.Keys, k)
			}
			o.M[k] = v
			p.ws()
			if p.pos >= len(p.s) {
				return nil, ErrSyntax
			}
			if p.s[p.pos] == ',' {
				p.pos++
				continue
			}
			if p.s[p.pos] == '}' {
				p.pos++
				return o, nil
			}
			return nil, ErrSyntax
		}
	case c == '[':
		p.pos++
		arr := []any{}
		p.ws()
		if p.pos < len(p.s) && p.s[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		for {
			p.ws()
			v, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
			p.ws()
			if p.pos >= len(p.s) {
				return nil, ErrSyntax
			}
			if p.s[p.pos] == ',' {
				p.pos++
				continue
			}
			if p.s[p.pos] == ']' {
				p.pos++
				return arr, nil
			}
			return nil, ErrSyntax
		}
	case c == '"':
		return p.str()
	case c == 't' && strings.HasPrefix(p.s[p.pos:], "true"):
		p.pos += 4
		return true, nil
	case c == 'f' && strings.HasPrefix(p.s[p.pos:], "false"):
		p.pos += 5
		return false, nil
	case c == 'n' && strings.HasPrefix(p.s[p.pos:], "null"):
		p.pos += 4
		return nil, nil
	case c == '-' || (c >= '0' && c <= '9'):
		st := p.pos
		p.pos++
		for p.pos < len(p.s) {
			d := p.s[p.pos]
			if (d >= '0' && d <= '9') || d == '.' || d == 'e' || d == 'E' || d == '+' || d == '-' {
				p.pos++
			} else {
				break
			}
		}
		f, err := strconv.ParseFloat(p.s[st:p.pos], 64)
		if err != nil {
			return nil, ErrSyntax
		}
		return f, nil
	}
	return nil, ErrSyntax
}

func (p *jparser) str() (string, error) {
	p.pos++ // opening quote
	var b strings.Builder
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch {
		case c == '"':
			p.pos++
			return b.String(), nil
		case c == '\\':
			p.pos++
			if p.pos >= len(p.s) {
				return "", ErrSyntax
			}
			e := p.s[p.pos]
			p.pos++
			switch e {
			case '"', '\\', '/':
				b.WriteByte(e)
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				r, ok := p.hex4()
				if !ok {
					return "", ErrSyntax
				}
				if utf16.IsSurrogate(r) {
					if r < 0xDC00 && strings.HasPrefix(p.s[p.pos:], "\\u") {
						save := p.pos
						p.pos += 2
						if r2, ok := p.hex4(); ok && r2 >= 0xDC00 && r2 < 0xE000 {
							b.WriteRune(utf16.DecodeRune(r, r2))
							continue
						}
						p.pos = save
					}
					b.WriteRune(utf8.RuneError)
				} else {
					b.WriteRune(r)
				}
			default:
				return "", ErrSyntax
			}
		case c < 0x20:
			return "", ErrSyntax
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
	return "", ErrSyntax
}

func (p *jparser) hex4() (rune, bool) {
	if p.pos+4 > len(p.s) {
		return 0, false
	}
	n, err := strconv.ParseUint(p.s[p.pos:p.pos+4], 16, 32)
	if err != nil {
		return 0, false
	}
	p.pos += 4
	return rune(n), true
}

// ---- writing ------------------------------------------------------------------------

// NumStr is how JavaScript prints a Number.
func NumStr(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	if f == 0 {
		return "0"
	}
	neg := f < 0
	if neg {
		f = -f
	}
	// shortest round-trip digits and decimal exponent, then the ECMAScript Number::toString layout
	e := strconv.FormatFloat(f, 'e', -1, 64) // d.ddddde+XX
	mant, expS, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, _ := strconv.Atoi(expS)
	k, n := len(digits), exp+1
	var out string
	switch {
	case k <= n && n <= 21:
		out = digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		out = digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		out = "0." + strings.Repeat("0", -n) + digits
	default:
		sign := "+"
		if n-1 < 0 {
			sign = "-"
		}
		ex := n - 1
		if ex < 0 {
			ex = -ex
		}
		if k == 1 {
			out = digits + "e" + sign + strconv.Itoa(ex)
		} else {
			out = digits[:1] + "." + digits[1:] + "e" + sign + strconv.Itoa(ex)
		}
	}
	if neg {
		return "-" + out
	}
	return out
}

// Quote is JSON.stringify for a string.
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteByte("0123456789abcdef"[r>>4])
				b.WriteByte("0123456789abcdef"[r&15])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Set assigns a key: an existing key keeps its position, a new one is appended (like a JS object).
func (o *Obj) Set(k string, v any) {
	if _, ok := o.M[k]; !ok {
		o.Keys = append(o.Keys, k)
	}
	o.M[k] = v
}

// NewObj returns an empty object.
func NewObj() *Obj { return &Obj{M: map[string]any{}} }

// Delete removes a key.
func (o *Obj) Delete(k string) {
	if _, ok := o.M[k]; !ok {
		return
	}
	delete(o.M, k)
	for i, x := range o.Keys {
		if x == k {
			o.Keys = append(o.Keys[:i], o.Keys[i+1:]...)
			break
		}
	}
}

// Stringify is JSON.stringify(v, null, indent) for nil, bool, float64, int, string, []any, []string, *Obj and
// map[string]any (keys sorted). indent 0 gives the compact form.
func Stringify(v any, indent int) string {
	var b strings.Builder
	write(&b, v, indent, 0)
	return b.String()
}

func write(b *strings.Builder, v any, indent, depth int) {
	nl := func(d int) {
		if indent > 0 {
			b.WriteByte('\n')
			b.WriteString(strings.Repeat(" ", indent*d))
		}
	}
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(NumStr(x))
		}
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case string:
		b.WriteString(Quote(x))
	case []string:
		a := make([]any, len(x))
		for i, e := range x {
			a[i] = e
		}
		write(b, a, indent, depth)
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(depth + 1)
			write(b, e, indent, depth+1)
		}
		nl(depth)
		b.WriteByte(']')
	case *Obj:
		if len(x.Keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, k := range x.Keys {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(depth + 1)
			b.WriteString(Quote(k))
			b.WriteByte(':')
			if indent > 0 {
				b.WriteByte(' ')
			}
			write(b, x.M[k], indent, depth+1)
		}
		nl(depth)
		b.WriteByte('}')
	case map[string]any:
		o := NewObj()
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			o.Set(k, x[k])
		}
		write(b, o, indent, depth)
	case map[string]string:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[k] = e
		}
		write(b, m, indent, depth)
	default:
		b.WriteString("null")
	}
}

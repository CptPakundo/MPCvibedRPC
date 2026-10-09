// Package jsre is a small regular-expression engine with JavaScript semantics.
//
// Go's regexp (RE2) has no lookahead, lookbehind or backreferences, which the filename parser relies on, so this
// package re-implements the JavaScript flavour: backtracking, greedy/lazy quantifiers, lookaround, \p{L}-style
// properties (L, Lu, Ll, N, M), flags g i m s u y. Text is handled as []rune, so indexes are code points.
package jsre

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ---- AST -----------------------------------------------------------------------------------------

type nodeKind int

const (
	nChar nodeKind = iota
	nAny
	nClass
	nStart
	nEnd
	nWordB
	nNotWordB
	nGroup  // capturing
	nNonCap // (?:...)
	nSeq
	nAlt
	nLook // lookahead/lookbehind
	nQuant
	nBackref
	nEmpty
)

type classItem struct {
	lo, hi rune
	pred   func(rune) bool // when set, the item is a predicate (\d, \w, \p{L}, negated forms)
}

type node struct {
	kind     nodeKind
	ch       rune
	items    []classItem
	neg      bool // class negation / negative lookaround
	subs     []*node
	idx      int // capture index / backref number
	behind   bool
	min, max int // max < 0: unbounded
	greedy   bool
	capLo    int // captures [capLo, capHi) live inside a quantified node (reset per iteration)
	capHi    int
}

// ---- parser --------------------------------------------------------------------------------------

type parser struct {
	src   []rune
	pos   int
	ncap  int
	unic  bool
	icase bool
	names map[string]int
}

func (p *parser) more() bool { return p.pos < len(p.src) }
func (p *parser) peek() rune { return p.src[p.pos] }
func (p *parser) eat(r rune) bool {
	if p.more() && p.src[p.pos] == r {
		p.pos++
		return true
	}
	return false
}
func (p *parser) lookingAt(s string) bool {
	rs := []rune(s)
	if p.pos+len(rs) > len(p.src) {
		return false
	}
	for i, r := range rs {
		if p.src[p.pos+i] != r {
			return false
		}
	}
	return true
}

func (p *parser) parseDisjunction() (*node, error) {
	var alts []*node
	for {
		alt, err := p.parseAlternative()
		if err != nil {
			return nil, err
		}
		alts = append(alts, alt)
		if !p.eat('|') {
			break
		}
	}
	if len(alts) == 1 {
		return alts[0], nil
	}
	return &node{kind: nAlt, subs: alts}, nil
}

func (p *parser) parseAlternative() (*node, error) {
	var seq []*node
	for p.more() && p.peek() != '|' && p.peek() != ')' {
		t, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		seq = append(seq, t)
	}
	if len(seq) == 0 {
		return &node{kind: nEmpty}, nil
	}
	if len(seq) == 1 {
		return seq[0], nil
	}
	return &node{kind: nSeq, subs: seq}, nil
}

func (p *parser) parseTerm() (*node, error) {
	capBefore := p.ncap
	c := p.peek()
	var atom *node
	switch c {
	case '^':
		p.pos++
		return &node{kind: nStart}, nil
	case '$':
		p.pos++
		return &node{kind: nEnd}, nil
	case '\\':
		if p.pos+1 < len(p.src) && (p.src[p.pos+1] == 'b' || p.src[p.pos+1] == 'B') {
			k := nWordB
			if p.src[p.pos+1] == 'B' {
				k = nNotWordB
			}
			p.pos += 2
			return &node{kind: k}, nil
		}
	case '(':
		if p.lookingAt("(?=") || p.lookingAt("(?!") || p.lookingAt("(?<=") || p.lookingAt("(?<!") {
			behind := p.lookingAt("(?<")
			neg := false
			if behind {
				neg = p.src[p.pos+3] == '!'
				p.pos += 4
			} else {
				neg = p.src[p.pos+2] == '!'
				p.pos += 3
			}
			sub, err := p.parseDisjunction()
			if err != nil {
				return nil, err
			}
			if !p.eat(')') {
				return nil, fmt.Errorf("unterminated group")
			}
			n := &node{kind: nLook, subs: []*node{sub}, neg: neg, behind: behind}
			if !behind { // Annex B: a lookahead may carry a quantifier
				return p.parseQuantifier(n, capBefore)
			}
			return n, nil
		}
		if p.lookingAt("(?:") {
			p.pos += 3
			sub, err := p.parseDisjunction()
			if err != nil {
				return nil, err
			}
			if !p.eat(')') {
				return nil, fmt.Errorf("unterminated group")
			}
			atom = &node{kind: nNonCap, subs: []*node{sub}}
		} else if p.lookingAt("(?<") {
			// named group (?<name>...)
			p.pos += 3
			start := p.pos
			for p.more() && p.peek() != '>' {
				p.pos++
			}
			name := string(p.src[start:p.pos])
			p.pos++
			p.ncap++
			idx := p.ncap
			if p.names == nil {
				p.names = map[string]int{}
			}
			p.names[name] = idx
			sub, err := p.parseDisjunction()
			if err != nil {
				return nil, err
			}
			if !p.eat(')') {
				return nil, fmt.Errorf("unterminated group")
			}
			atom = &node{kind: nGroup, idx: idx, subs: []*node{sub}}
		} else {
			p.pos++
			p.ncap++
			idx := p.ncap
			sub, err := p.parseDisjunction()
			if err != nil {
				return nil, err
			}
			if !p.eat(')') {
				return nil, fmt.Errorf("unterminated group")
			}
			atom = &node{kind: nGroup, idx: idx, subs: []*node{sub}}
		}
	case '.':
		p.pos++
		atom = &node{kind: nAny}
	case '[':
		cl, err := p.parseClass()
		if err != nil {
			return nil, err
		}
		atom = cl
	case '*', '+', '?':
		return nil, fmt.Errorf("nothing to repeat")
	}
	if atom == nil {
		if c == '\\' {
			a, err := p.parseAtomEscape()
			if err != nil {
				return nil, err
			}
			atom = a
		} else {
			p.pos++
			atom = &node{kind: nChar, ch: c}
		}
	}
	return p.parseQuantifier(atom, capBefore)
}

func (p *parser) parseQuantifier(atom *node, capBefore int) (*node, error) {
	if !p.more() {
		return atom, nil
	}
	min, max := -1, -1
	switch p.peek() {
	case '*':
		p.pos++
		min, max = 0, -1
	case '+':
		p.pos++
		min, max = 1, -1
	case '?':
		p.pos++
		min, max = 0, 1
	case '{':
		save := p.pos
		p.pos++
		a, okA := p.parseInt()
		if !okA {
			p.pos = save
			return atom, nil // literal "{"
		}
		b := a
		if p.eat(',') {
			if v, ok := p.parseInt(); ok {
				b = v
			} else {
				b = -1
			}
		}
		if !p.eat('}') {
			p.pos = save
			return atom, nil
		}
		min, max = a, b
	default:
		return atom, nil
	}
	greedy := true
	if p.eat('?') {
		greedy = false
	}
	return &node{kind: nQuant, subs: []*node{atom}, min: min, max: max, greedy: greedy, capLo: capBefore + 1, capHi: p.ncap + 1}, nil
}

func (p *parser) parseInt() (int, bool) {
	start := p.pos
	for p.more() && p.peek() >= '0' && p.peek() <= '9' {
		p.pos++
	}
	if start == p.pos {
		return 0, false
	}
	v, _ := strconv.Atoi(string(p.src[start:p.pos]))
	return v, true
}

func hexVal(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10
	}
	return -1
}

func (p *parser) hexDigits(n int) (rune, bool) {
	if p.pos+n > len(p.src) {
		return 0, false
	}
	v := 0
	for i := 0; i < n; i++ {
		h := hexVal(p.src[p.pos+i])
		if h < 0 {
			return 0, false
		}
		v = v*16 + h
	}
	p.pos += n
	return rune(v), true
}

// parseEscapeChar handles the escapes that denote a single character; ok=false when it is a class escape.
func (p *parser) parseCharEscape(inClass bool) (rune, bool, error) {
	// p.pos is at the char after the backslash
	if !p.more() {
		return 0, false, fmt.Errorf("\\ at end of pattern")
	}
	c := p.src[p.pos]
	p.pos++
	switch c {
	case 't':
		return '\t', true, nil
	case 'n':
		return '\n', true, nil
	case 'r':
		return '\r', true, nil
	case 'f':
		return '\f', true, nil
	case 'v':
		return '\v', true, nil
	case '0':
		if !p.more() || p.peek() < '0' || p.peek() > '9' {
			return 0, true, nil
		}
		return '0', true, nil
	case 'b':
		if inClass {
			return '\b', true, nil
		}
	case 'c':
		if p.more() && ((p.peek() >= 'a' && p.peek() <= 'z') || (p.peek() >= 'A' && p.peek() <= 'Z')) {
			v := p.peek() % 32
			p.pos++
			return rune(v), true, nil
		}
		return '\\', true, nil
	case 'x':
		if v, ok := p.hexDigits(2); ok {
			return v, true, nil
		}
		return 'x', true, nil
	case 'u':
		if p.unic && p.more() && p.peek() == '{' {
			save := p.pos
			p.pos++
			v := 0
			n := 0
			for p.more() && hexVal(p.peek()) >= 0 {
				v = v*16 + hexVal(p.peek())
				p.pos++
				n++
			}
			if n > 0 && p.eat('}') {
				return rune(v), true, nil
			}
			p.pos = save
			return 'u', true, nil
		}
		if v, ok := p.hexDigits(4); ok {
			// surrogate pair 😀 in unicode mode
			if v >= 0xD800 && v <= 0xDBFF && p.lookingAt("\\u") {
				save := p.pos
				p.pos += 2
				if lo, ok2 := p.hexDigits(4); ok2 && lo >= 0xDC00 && lo <= 0xDFFF {
					return 0x10000 + (v-0xD800)<<10 + (lo - 0xDC00), true, nil
				}
				p.pos = save
			}
			return v, true, nil
		}
		return 'u', true, nil
	case 'd', 'D', 'w', 'W', 's', 'S', 'p', 'P':
		p.pos--
		return 0, false, nil
	}
	return c, true, nil
}

func isSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}
func isDigit(r rune) bool { return r >= '0' && r <= '9' }
func isWord(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}
func isLineTerm(r rune) bool { return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029 }

func propPred(name string) (func(rune) bool, error) {
	name = strings.TrimPrefix(name, "General_Category=")
	name = strings.TrimPrefix(name, "gc=")
	var tab *unicode.RangeTable
	switch name {
	case "L", "Letter":
		tab = unicode.L
	case "Lu", "Uppercase_Letter":
		tab = unicode.Lu
	case "Ll", "Lowercase_Letter":
		tab = unicode.Ll
	case "Lt":
		tab = unicode.Lt
	case "Lm":
		tab = unicode.Lm
	case "Lo":
		tab = unicode.Lo
	case "N", "Number":
		tab = unicode.N
	case "Nd":
		tab = unicode.Nd
	case "M", "Mark":
		tab = unicode.M
	case "Mn":
		tab = unicode.Mn
	case "P", "Punctuation":
		tab = unicode.P
	case "S", "Symbol":
		tab = unicode.S
	case "Z", "Separator":
		tab = unicode.Z
	default:
		if strings.HasPrefix(name, "Script=") || strings.HasPrefix(name, "sc=") || strings.HasPrefix(name, "Script_Extensions=") {
			s := name[strings.Index(name, "=")+1:]
			if t, ok := unicode.Scripts[s]; ok {
				return func(r rune) bool { return unicode.Is(t, r) }, nil
			}
		}
		return nil, fmt.Errorf("unsupported property \\p{%s}", name)
	}
	return func(r rune) bool { return unicode.Is(tab, r) }, nil
}

func (p *parser) classEscapeItem() (classItem, error) {
	c := p.src[p.pos]
	p.pos++
	switch c {
	case 'd':
		return classItem{pred: isDigit}, nil
	case 'D':
		return classItem{pred: func(r rune) bool { return !isDigit(r) }}, nil
	case 'w':
		return classItem{pred: isWord}, nil
	case 'W':
		return classItem{pred: func(r rune) bool { return !isWord(r) }}, nil
	case 's':
		return classItem{pred: isSpace}, nil
	case 'S':
		return classItem{pred: func(r rune) bool { return !isSpace(r) }}, nil
	case 'p', 'P':
		if !p.unic || !p.eat('{') {
			// without the u flag, \p is just "p"
			return classItem{lo: c, hi: c}, nil
		}
		start := p.pos
		for p.more() && p.peek() != '}' {
			p.pos++
		}
		name := string(p.src[start:p.pos])
		p.pos++
		pred, err := propPred(name)
		if err != nil {
			return classItem{}, err
		}
		if c == 'P' {
			inner := pred
			pred = func(r rune) bool { return !inner(r) }
		}
		return classItem{pred: pred}, nil
	}
	return classItem{}, fmt.Errorf("bad class escape")
}

func (p *parser) parseAtomEscape() (*node, error) {
	// at the backslash
	p.pos++
	if !p.more() {
		return nil, fmt.Errorf("\\ at end of pattern")
	}
	c := p.peek()
	if c >= '1' && c <= '9' {
		save := p.pos
		n, _ := p.parseInt()
		if n <= 99 { // resolved against the final group count in compile; Annex B octal fallback ignored
			return &node{kind: nBackref, idx: n}, nil
		}
		p.pos = save
	}
	if c == 'k' && p.names != nil {
		p.pos++
		if p.eat('<') {
			start := p.pos
			for p.more() && p.peek() != '>' {
				p.pos++
			}
			name := string(p.src[start:p.pos])
			p.pos++
			if idx, ok := p.names[name]; ok {
				return &node{kind: nBackref, idx: idx}, nil
			}
		}
		return nil, fmt.Errorf("bad named reference")
	}
	r, isChar, err := p.parseCharEscape(false)
	if err != nil {
		return nil, err
	}
	if isChar {
		return &node{kind: nChar, ch: r}, nil
	}
	it, err := p.classEscapeItem()
	if err != nil {
		return nil, err
	}
	return &node{kind: nClass, items: []classItem{it}}, nil
}

func (p *parser) parseClass() (*node, error) {
	p.pos++ // [
	n := &node{kind: nClass}
	if p.eat('^') {
		n.neg = true
	}
	for {
		if !p.more() {
			return nil, fmt.Errorf("unterminated character class")
		}
		if p.peek() == ']' {
			p.pos++
			break
		}
		lo, loItem, err := p.classAtom()
		if err != nil {
			return nil, err
		}
		if loItem != nil {
			n.items = append(n.items, *loItem)
			continue
		}
		// range?
		if p.pos+1 < len(p.src) && p.peek() == '-' && p.src[p.pos+1] != ']' {
			save := p.pos
			p.pos++
			hi, hiItem, err := p.classAtom()
			if err != nil {
				return nil, err
			}
			if hiItem != nil { // "a-\d": the dash is literal
				n.items = append(n.items, classItem{lo: lo, hi: lo}, classItem{lo: '-', hi: '-'}, *hiItem)
				continue
			}
			if hi < lo {
				p.pos = save
				return nil, fmt.Errorf("range out of order in character class")
			}
			n.items = append(n.items, classItem{lo: lo, hi: hi})
			continue
		}
		n.items = append(n.items, classItem{lo: lo, hi: lo})
	}
	return n, nil
}

func (p *parser) classAtom() (rune, *classItem, error) {
	c := p.src[p.pos]
	if c != '\\' {
		p.pos++
		return c, nil, nil
	}
	p.pos++
	if !p.more() {
		return 0, nil, fmt.Errorf("\\ at end of pattern")
	}
	r, isChar, err := p.parseCharEscape(true)
	if err != nil {
		return 0, nil, err
	}
	if isChar {
		return r, nil, nil
	}
	it, err := p.classEscapeItem()
	if err != nil {
		return 0, nil, err
	}
	return 0, &it, nil
}

// ---- compiled form -----------------------------------------------------------------------------------

// Regexp is a compiled JavaScript-style regular expression.
type Regexp struct {
	Source string
	Flags  string
	Global bool
	icase  bool
	multi  bool
	dotAll bool
	sticky bool
	unic   bool
	ncap   int
	root   matcher
	names  map[string]int
}

type state struct {
	in    []rune
	caps  []int
	steps int
}

type matcher func(s *state, i int, k func(int) bool) bool

// ErrTooComplex is returned (via panic recovery in Exec) when a pattern backtracks without end.
const stepLimit = 5_000_000

// Compile parses a pattern with JavaScript flags ("g", "i", "m", "s", "u", "y").
func Compile(pattern, flags string) (*Regexp, error) {
	re := &Regexp{Source: pattern, Flags: flags}
	for _, f := range flags {
		switch f {
		case 'g':
			re.Global = true
		case 'i':
			re.icase = true
		case 'm':
			re.multi = true
		case 's':
			re.dotAll = true
		case 'u':
			re.unic = true
		case 'y':
			re.sticky = true
		default:
			return nil, fmt.Errorf("invalid flag %q", f)
		}
	}
	p := &parser{src: []rune(pattern), unic: re.unic, icase: re.icase}
	// first pass names so \k<name> can resolve forward references is not needed for the patterns in use
	root, err := p.parseDisjunction()
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression /%s/: %v", pattern, err)
	}
	if p.more() {
		return nil, fmt.Errorf("invalid regular expression /%s/: unmatched )", pattern)
	}
	re.ncap = p.ncap
	re.names = p.names
	re.root = re.compile(root, false)
	return re, nil
}

// MustCompile is Compile that panics on error (for package-level patterns).
func MustCompile(pattern, flags string) *Regexp {
	re, err := Compile(pattern, flags)
	if err != nil {
		panic(err)
	}
	return re
}

func canon(r rune) rune {
	m := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < m {
			m = f
		}
	}
	return m
}

func (re *Regexp) classMatch(n *node, r rune) bool {
	hit := false
	for _, it := range n.items {
		if it.pred != nil {
			if it.pred(r) {
				hit = true
				break
			}
		} else if r >= it.lo && r <= it.hi {
			hit = true
			break
		}
	}
	if !hit && re.icase {
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			for _, it := range n.items {
				if it.pred != nil {
					if it.pred(f) {
						hit = true
					}
				} else if f >= it.lo && f <= it.hi {
					hit = true
				}
				if hit {
					break
				}
			}
			if hit {
				break
			}
		}
	}
	return hit != n.neg
}

// single returns a one-rune predicate when the node matches exactly one rune.
func (re *Regexp) single(n *node) func(rune) bool {
	switch n.kind {
	case nChar:
		c := n.ch
		if re.icase {
			cc := canon(c)
			return func(r rune) bool { return r == c || canon(r) == cc }
		}
		return func(r rune) bool { return r == c }
	case nAny:
		if re.dotAll {
			return func(r rune) bool { return true }
		}
		return func(r rune) bool { return !isLineTerm(r) }
	case nClass:
		return func(r rune) bool { return re.classMatch(n, r) }
	case nNonCap:
		return re.single(n.subs[0])
	}
	return nil
}

func (re *Regexp) compile(n *node, backward bool) matcher {
	if pred := re.single(n); pred != nil && n.kind != nNonCap {
		return func(s *state, i int, k func(int) bool) bool {
			if i < len(s.in) && pred(s.in[i]) {
				return k(i + 1)
			}
			return false
		}
	}
	switch n.kind {
	case nEmpty:
		return func(s *state, i int, k func(int) bool) bool { return k(i) }
	case nStart:
		return func(s *state, i int, k func(int) bool) bool {
			if i == 0 || (re.multi && isLineTerm(s.in[i-1])) {
				return k(i)
			}
			return false
		}
	case nEnd:
		return func(s *state, i int, k func(int) bool) bool {
			if i == len(s.in) || (re.multi && isLineTerm(s.in[i])) {
				return k(i)
			}
			return false
		}
	case nWordB, nNotWordB:
		want := n.kind == nWordB
		return func(s *state, i int, k func(int) bool) bool {
			a := i > 0 && isWord(s.in[i-1])
			b := i < len(s.in) && isWord(s.in[i])
			if (a != b) == want {
				return k(i)
			}
			return false
		}
	case nNonCap:
		return re.compile(n.subs[0], backward)
	case nGroup:
		inner := re.compile(n.subs[0], backward)
		idx := n.idx
		return func(s *state, i int, k func(int) bool) bool {
			o0, o1 := s.caps[2*idx], s.caps[2*idx+1]
			ok := inner(s, i, func(j int) bool {
				p0, p1 := s.caps[2*idx], s.caps[2*idx+1]
				s.caps[2*idx], s.caps[2*idx+1] = i, j
				if k(j) {
					return true
				}
				s.caps[2*idx], s.caps[2*idx+1] = p0, p1
				return false
			})
			if !ok {
				s.caps[2*idx], s.caps[2*idx+1] = o0, o1
			}
			return ok
		}
	case nSeq:
		ms := make([]matcher, len(n.subs))
		for i, sub := range n.subs {
			ms[i] = re.compile(sub, backward)
		}
		var build func(idx int) matcher
		build = func(idx int) matcher {
			if idx == len(ms) {
				return func(s *state, i int, k func(int) bool) bool { return k(i) }
			}
			head := ms[idx]
			rest := build(idx + 1)
			return func(s *state, i int, k func(int) bool) bool {
				return head(s, i, func(j int) bool { return rest(s, j, k) })
			}
		}
		return build(0)
	case nAlt:
		ms := make([]matcher, len(n.subs))
		for i, sub := range n.subs {
			ms[i] = re.compile(sub, backward)
		}
		return func(s *state, i int, k func(int) bool) bool {
			for _, m := range ms {
				if m(s, i, k) {
					return true
				}
			}
			return false
		}
	case nLook:
		inner := re.compile(n.subs[0], false)
		neg, behind := n.neg, n.behind
		return func(s *state, i int, k func(int) bool) bool {
			saved := append([]int(nil), s.caps...)
			matched := false
			if !behind {
				matched = inner(s, i, func(int) bool { return true })
			} else {
				for j := i; j >= 0 && !matched; j-- {
					matched = inner(s, j, func(e int) bool { return e == i })
				}
			}
			if neg {
				copy(s.caps, saved)
				if matched {
					return false
				}
				return k(i)
			}
			if !matched {
				copy(s.caps, saved)
				return false
			}
			if k(i) {
				return true
			}
			copy(s.caps, saved)
			return false
		}
	case nBackref:
		idx := n.idx
		return func(s *state, i int, k func(int) bool) bool {
			if idx > re.ncap || s.caps[2*idx] < 0 {
				return k(i)
			}
			sub := s.in[s.caps[2*idx]:s.caps[2*idx+1]]
			if i+len(sub) > len(s.in) {
				return false
			}
			for x, r := range sub {
				o := s.in[i+x]
				if o != r && !(re.icase && canon(o) == canon(r)) {
					return false
				}
			}
			return k(i + len(sub))
		}
	case nQuant:
		return re.compileQuant(n, backward)
	}
	panic("jsre: unknown node")
}

func (re *Regexp) compileQuant(n *node, backward bool) matcher {
	min, max, greedy := n.min, n.max, n.greedy
	sub := n.subs[0]
	if pred := re.single(sub); pred != nil {
		// fast path: one rune per iteration
		return func(s *state, i int, k func(int) bool) bool {
			limit := len(s.in) - i
			if max >= 0 && max < limit {
				limit = max
			}
			if greedy {
				c := 0
				for c < limit && pred(s.in[i+c]) {
					c++
				}
				for ; c >= min; c-- {
					if k(i + c) {
						return true
					}
				}
				return false
			}
			c := 0
			for ; c < min; c++ {
				if c >= limit || !pred(s.in[i+c]) {
					return false
				}
			}
			for {
				if k(i + c) {
					return true
				}
				if c >= limit || !pred(s.in[i+c]) {
					return false
				}
				c++
			}
		}
	}
	inner := re.compile(sub, backward)
	capLo, capHi := n.capLo, n.capHi
	var rep func(s *state, i int, k func(int) bool, count int) bool
	rep = func(s *state, i int, k func(int) bool, count int) bool {
		s.steps++
		if s.steps > stepLimit {
			panic(errComplex)
		}
		tryMore := func() bool {
			if max >= 0 && count >= max {
				return false
			}
			var saved []int
			if capHi > capLo {
				saved = append(saved, s.caps[2*capLo:2*capHi]...)
				for x := 2 * capLo; x < 2*capHi; x++ {
					s.caps[x] = -1
				}
			}
			if inner(s, i, func(j int) bool {
				if j == i && count >= min {
					return false
				}
				return rep(s, j, k, count+1)
			}) {
				return true
			}
			if capHi > capLo {
				copy(s.caps[2*capLo:2*capHi], saved)
			}
			return false
		}
		if greedy {
			if tryMore() {
				return true
			}
			return count >= min && k(i)
		}
		if count >= min && k(i) {
			return true
		}
		return tryMore()
	}
	return func(s *state, i int, k func(int) bool) bool { return rep(s, i, k, 0) }
}

type complexErr struct{}

var errComplex = complexErr{}

// ---- matching API ---------------------------------------------------------------------------------------

// Match is one successful match. Indexes count runes of the input.
type Match struct {
	In   []rune
	Caps []int // 2 per group (group 0 = whole match); -1 when the group did not take part
}

// Index and End of the whole match.
func (m *Match) Index() int { return m.Caps[0] }
func (m *Match) End() int   { return m.Caps[1] }

// Group returns group n's text ("" and false when it did not take part).
func (m *Match) Group(n int) (string, bool) {
	if n*2+1 >= len(m.Caps) || m.Caps[2*n] < 0 {
		return "", false
	}
	return string(m.In[m.Caps[2*n]:m.Caps[2*n+1]]), true
}

// Str is Group without the flag ("" for an absent group, like String(undefined)-less code that checks truthiness).
func (m *Match) Str(n int) string { s, _ := m.Group(n); return s }

// Has reports whether group n took part in the match.
func (m *Match) Has(n int) bool { _, ok := m.Group(n); return ok }

// NumGroups is the number of capture groups (excluding group 0).
func (re *Regexp) NumGroups() int { return re.ncap }

// GroupIndex looks up a named group (0 when missing).
func (re *Regexp) GroupIndex(name string) int { return re.names[name] }

func (re *Regexp) execAt(in []rune, from int) *Match {
	s := &state{in: in, caps: make([]int, 2*(re.ncap+1))}
	last := len(in)
	if re.sticky {
		last = from
	}
	for st := from; st <= last; st++ {
		for x := range s.caps {
			s.caps[x] = -1
		}
		end := -1
		if re.root(s, st, func(e int) bool { end = e; return true }) {
			s.caps[0], s.caps[1] = st, end
			return &Match{In: in, Caps: s.caps}
		}
	}
	return nil
}

// ExecAt runs the search from a rune index (like setting lastIndex). A pattern that backtracks without bound
// reports no match instead of hanging.
func (re *Regexp) ExecAt(in []rune, from int) (m *Match) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(complexErr); ok {
				m = nil
				return
			}
			panic(r)
		}
	}()
	return re.execAt(in, from)
}

// Exec searches s from the start.
func (re *Regexp) Exec(s string) *Match { return re.ExecAt([]rune(s), 0) }

// Test reports whether s contains a match.
func (re *Regexp) Test(s string) bool { return re.Exec(s) != nil }

// FindAll returns every match for a global pattern, or just the first one otherwise.
func (re *Regexp) FindAll(s string) []*Match {
	in := []rune(s)
	var out []*Match
	pos := 0
	for pos <= len(in) {
		m := re.ExecAt(in, pos)
		if m == nil {
			break
		}
		out = append(out, m)
		if !re.Global {
			break
		}
		if m.End() == m.Index() {
			pos = m.End() + 1
		} else {
			pos = m.End()
		}
	}
	return out
}

// expandTemplate implements the $-patterns of String.prototype.replace.
func expandTemplate(tpl string, m *Match, nGroups int) string {
	if !strings.Contains(tpl, "$") {
		return tpl
	}
	var b strings.Builder
	rs := []rune(tpl)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '$' || i+1 >= len(rs) {
			b.WriteRune(rs[i])
			continue
		}
		c := rs[i+1]
		switch {
		case c == '$':
			b.WriteRune('$')
			i++
		case c == '&':
			b.WriteString(string(m.In[m.Index():m.End()]))
			i++
		case c == '`':
			b.WriteString(string(m.In[:m.Index()]))
			i++
		case c == '\'':
			b.WriteString(string(m.In[m.End():]))
			i++
		case c >= '0' && c <= '9':
			n := int(c - '0')
			consumed := 1
			if i+2 < len(rs) && rs[i+2] >= '0' && rs[i+2] <= '9' {
				if two := n*10 + int(rs[i+2]-'0'); two >= 1 && two <= nGroups {
					n, consumed = two, 2
				}
			}
			if n >= 1 && n <= nGroups {
				b.WriteString(m.Str(n))
				i += consumed
			} else {
				b.WriteRune('$')
			}
		default:
			b.WriteRune('$')
		}
	}
	return b.String()
}

// Replace is s.replace(re, fn): every match for a global pattern, else the first.
func (re *Regexp) Replace(s string, fn func(m *Match) string) string {
	in := []rune(s)
	var b strings.Builder
	pos, last := 0, 0
	for pos <= len(in) {
		m := re.ExecAt(in, pos)
		if m == nil {
			break
		}
		b.WriteString(string(in[last:m.Index()]))
		b.WriteString(fn(m))
		last = m.End()
		if !re.Global {
			break
		}
		if m.End() == m.Index() {
			pos = m.End() + 1
		} else {
			pos = m.End()
		}
	}
	b.WriteString(string(in[last:]))
	return b.String()
}

// ReplaceStr is s.replace(re, "template") with $1, $&, $$ ... expansion.
func (re *Regexp) ReplaceStr(s, tpl string) string {
	return re.Replace(s, func(m *Match) string { return expandTemplate(tpl, m, re.ncap) })
}

// Split is s.split(re) with JavaScript semantics (captured groups are included in the result).
func (re *Regexp) Split(s string) []string {
	in := []rune(s)
	if len(in) == 0 {
		if re.ExecAt(in, 0) != nil {
			return []string{}
		}
		return []string{""}
	}
	sticky := *re
	sticky.sticky = true
	var out []string
	p, q := 0, 0
	for q < len(in) {
		m := sticky.ExecAt(in, q)
		if m == nil || m.End() == p || m.End() > len(in) {
			q++
			continue
		}
		e := m.End()
		out = append(out, string(in[p:q]))
		for g := 1; g <= re.ncap; g++ {
			out = append(out, m.Str(g)) // undefined groups become "" (callers never rely on undefined)
		}
		p = e
		q = p
	}
	out = append(out, string(in[p:]))
	return out
}

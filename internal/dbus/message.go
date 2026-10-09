package dbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
)

// Message is one D-Bus message. Body holds the values described by Signature: strings for s/o/g, the Go integer of
// the matching size, float64 for d, bool for b, []any for arrays and structs, map[string]any for dictionaries (keys
// written with fmt) and, when decoding, the value inside a variant. To encode a variant, pass a Variant.
type Message struct {
	Type        byte
	Flags       byte
	Serial      uint32
	Path        string
	Interface   string
	Member      string
	ErrorName   string
	ReplySerial uint32
	Destination string
	Sender      string
	Signature   string
	Body        []any
}

// Variant is a value with its own signature (D-Bus type "v").
type Variant struct {
	Sig   string
	Value any
}

// maxMessage bounds what is read from the bus (the protocol allows 128 MiB; nothing we ask about comes close).
const maxMessage = 16 << 20

// header field codes
const (
	fieldPath        = 1
	fieldInterface   = 2
	fieldMember      = 3
	fieldErrorName   = 4
	fieldReplySerial = 5
	fieldDestination = 6
	fieldSender      = 7
	fieldSignature   = 8
)

// Marshal encodes the message (little-endian).
func (m *Message) Marshal() ([]byte, error) {
	var body encoder
	types, err := splitSig(m.Signature)
	if err != nil {
		return nil, err
	}
	if len(types) != len(m.Body) {
		return nil, fmt.Errorf("dbus: signature %q needs %d values, got %d", m.Signature, len(types), len(m.Body))
	}
	for i, t := range types {
		if err := body.value(t, m.Body[i]); err != nil {
			return nil, err
		}
	}
	var fields []any
	add := func(code byte, sig string, v any) { fields = append(fields, []any{code, Variant{sig, v}}) }
	if m.Path != "" {
		add(fieldPath, "o", m.Path)
	}
	if m.Interface != "" {
		add(fieldInterface, "s", m.Interface)
	}
	if m.Member != "" {
		add(fieldMember, "s", m.Member)
	}
	if m.ErrorName != "" {
		add(fieldErrorName, "s", m.ErrorName)
	}
	if m.ReplySerial != 0 {
		add(fieldReplySerial, "u", m.ReplySerial)
	}
	if m.Destination != "" {
		add(fieldDestination, "s", m.Destination)
	}
	if m.Sender != "" {
		add(fieldSender, "s", m.Sender)
	}
	if m.Signature != "" {
		add(fieldSignature, "g", m.Signature)
	}
	var h encoder
	h.b = append(h.b, 'l', m.Type, m.Flags, 1)
	h.u32(uint32(len(body.b)))
	h.u32(m.Serial)
	if err := h.value("a(yv)", fields); err != nil {
		return nil, err
	}
	h.align(8)
	return append(h.b, body.b...), nil
}

// ReadMessage reads one message.
func ReadMessage(r io.Reader) (*Message, error) {
	fixed := make([]byte, 16)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch fixed[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return nil, errors.New("dbus: bad byte order")
	}
	bodyLen, fieldsLen := order.Uint32(fixed[4:]), order.Uint32(fixed[12:])
	pad := (8 - (16+fieldsLen)%8) % 8
	total := uint64(16) + uint64(fieldsLen) + uint64(pad) + uint64(bodyLen)
	if total > maxMessage {
		return nil, errors.New("dbus: message too large")
	}
	buf := make([]byte, total)
	copy(buf, fixed)
	if _, err := io.ReadFull(r, buf[16:]); err != nil {
		return nil, err
	}
	m := &Message{Type: fixed[1], Flags: fixed[2], Serial: order.Uint32(fixed[8:])}
	d := &decoder{b: buf[:16+fieldsLen], off: 12, order: order}
	fv, err := d.top("a(yv)")
	if err != nil {
		return nil, err
	}
	for _, f := range fv.([]any) {
		s := f.([]any)
		code, _ := s[0].(byte)
		str, _ := s[1].(string)
		switch code {
		case fieldPath:
			m.Path = str
		case fieldInterface:
			m.Interface = str
		case fieldMember:
			m.Member = str
		case fieldErrorName:
			m.ErrorName = str
		case fieldReplySerial:
			m.ReplySerial, _ = s[1].(uint32)
		case fieldDestination:
			m.Destination = str
		case fieldSender:
			m.Sender = str
		case fieldSignature:
			m.Signature = str
		}
	}
	types, err := splitSig(m.Signature)
	if err != nil {
		return nil, err
	}
	// the body's alignment counts from its own start
	bd := &decoder{b: buf[16+fieldsLen+pad:], order: order}
	for _, t := range types {
		v, err := bd.top(t)
		if err != nil {
			return nil, err
		}
		m.Body = append(m.Body, v)
	}
	return m, nil
}

// ---- signatures ----

// nextType splits the first complete type off a signature.
func nextType(sig string) (string, string, error) {
	if sig == "" {
		return "", "", errors.New("dbus: empty signature")
	}
	switch sig[0] {
	case 'a':
		t, rest, err := nextType(sig[1:])
		return "a" + t, rest, err
	case '(', '{':
		closer := byte(')')
		if sig[0] == '{' {
			closer = '}'
		}
		depth := 0
		for i := 0; i < len(sig); i++ {
			switch sig[i] {
			case '(', '{':
				depth++
			case ')', '}':
				depth--
				if depth == 0 {
					if sig[i] != closer {
						return "", "", errors.New("dbus: bad signature")
					}
					return sig[:i+1], sig[i+1:], nil
				}
			}
		}
		return "", "", errors.New("dbus: unclosed signature")
	case 'y', 'b', 'n', 'q', 'i', 'u', 'x', 't', 'd', 's', 'o', 'g', 'v', 'h':
		return sig[:1], sig[1:], nil
	}
	return "", "", fmt.Errorf("dbus: unknown type %q", sig[0])
}

// splitSig splits a signature into its complete types.
func splitSig(sig string) ([]string, error) {
	var out []string
	for sig != "" {
		t, rest, err := nextType(sig)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		sig = rest
	}
	return out, nil
}

// inner returns the member types of a struct or dict entry type ("(sv)" -> [s v]).
func inner(t string) ([]string, error) { return splitSig(t[1 : len(t)-1]) }

func alignment(t byte) int {
	switch t {
	case 'n', 'q':
		return 2
	case 'b', 'i', 'u', 's', 'o', 'a', 'h':
		return 4
	case 'x', 't', 'd', '(', '{':
		return 8
	}
	return 1 // y g v
}

// ---- encoding ----

type encoder struct{ b []byte }

func (e *encoder) align(n int) {
	for len(e.b)%n != 0 {
		e.b = append(e.b, 0)
	}
}

func (e *encoder) u32(v uint32) {
	e.align(4)
	e.b = binary.LittleEndian.AppendUint32(e.b, v)
}

func (e *encoder) u64(v uint64) {
	e.align(8)
	e.b = binary.LittleEndian.AppendUint64(e.b, v)
}

func (e *encoder) str(s string) {
	e.u32(uint32(len(s)))
	e.b = append(append(e.b, s...), 0)
}

func wrongType(t string, v any) error { return fmt.Errorf("dbus: cannot encode %T as %q", v, t) }

func (e *encoder) value(t string, v any) error {
	switch t[0] {
	case 'y':
		x, ok := v.(byte)
		if !ok {
			return wrongType(t, v)
		}
		e.b = append(e.b, x)
	case 'b':
		x, ok := v.(bool)
		if !ok {
			return wrongType(t, v)
		}
		n := uint32(0)
		if x {
			n = 1
		}
		e.u32(n)
	case 'n', 'q':
		var n uint16
		switch x := v.(type) {
		case int16:
			n = uint16(x)
		case uint16:
			n = x
		default:
			return wrongType(t, v)
		}
		e.align(2)
		e.b = binary.LittleEndian.AppendUint16(e.b, n)
	case 'i', 'u', 'h':
		switch x := v.(type) {
		case int32:
			e.u32(uint32(x))
		case uint32:
			e.u32(x)
		default:
			return wrongType(t, v)
		}
	case 'x', 't':
		switch x := v.(type) {
		case int64:
			e.u64(uint64(x))
		case uint64:
			e.u64(x)
		default:
			return wrongType(t, v)
		}
	case 'd':
		x, ok := v.(float64)
		if !ok {
			return wrongType(t, v)
		}
		e.u64(math.Float64bits(x))
	case 's', 'o':
		x, ok := v.(string)
		if !ok {
			return wrongType(t, v)
		}
		e.str(x)
	case 'g':
		x, ok := v.(string)
		if !ok || len(x) > 255 {
			return wrongType(t, v)
		}
		e.b = append(append(append(e.b, byte(len(x))), x...), 0)
	case 'v':
		x, ok := v.(Variant)
		if !ok {
			return wrongType(t, v)
		}
		if err := e.value("g", x.Sig); err != nil {
			return err
		}
		return e.value(x.Sig, x.Value)
	case '(':
		x, ok := v.([]any)
		members, err := inner(t)
		if err != nil {
			return err
		}
		if !ok || len(x) != len(members) {
			return wrongType(t, v)
		}
		e.align(8)
		for i, m := range members {
			if err := e.value(m, x[i]); err != nil {
				return err
			}
		}
	case 'a':
		elem := t[1:]
		e.u32(0) // the length, filled in below
		at := len(e.b) - 4
		e.align(alignment(elem[0]))
		start := len(e.b)
		if elem[0] == '{' {
			kv, err := inner(elem)
			if err != nil || len(kv) != 2 {
				return errors.New("dbus: bad dictionary type")
			}
			m, ok := v.(map[string]any)
			if !ok {
				return wrongType(t, v)
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				e.align(8)
				if err := e.value(kv[0], k); err != nil {
					return err
				}
				if err := e.value(kv[1], m[k]); err != nil {
					return err
				}
			}
		} else {
			var items []any
			switch x := v.(type) {
			case []any:
				items = x
			case []string:
				for _, s := range x {
					items = append(items, s)
				}
			default:
				return wrongType(t, v)
			}
			for _, it := range items {
				if err := e.value(elem, it); err != nil {
					return err
				}
			}
		}
		binary.LittleEndian.PutUint32(e.b[at:], uint32(len(e.b)-start))
	default:
		return fmt.Errorf("dbus: cannot encode type %q", t)
	}
	return nil
}

// ---- decoding ----

type decoder struct {
	b     []byte
	off   int
	order binary.ByteOrder
	depth int
}

var errShort = errors.New("dbus: message ends early")

// top decodes one value, turning a malformed message into an error rather than a panic.
func (d *decoder) top(t string) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			v, err = nil, fmt.Errorf("dbus: malformed message (%v)", r)
		}
	}()
	return d.value(t)
}

func (d *decoder) align(n int) error {
	for d.off%n != 0 {
		d.off++
	}
	if d.off > len(d.b) {
		return errShort
	}
	return nil
}

func (d *decoder) take(n int) ([]byte, error) {
	if n < 0 || d.off+n > len(d.b) {
		return nil, errShort
	}
	s := d.b[d.off : d.off+n]
	d.off += n
	return s, nil
}

func (d *decoder) u32() (uint32, error) {
	if err := d.align(4); err != nil {
		return 0, err
	}
	s, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return d.order.Uint32(s), nil
}

func (d *decoder) str(lenBytes int) (string, error) {
	var n int
	if lenBytes == 1 {
		s, err := d.take(1)
		if err != nil {
			return "", err
		}
		n = int(s[0])
	} else {
		u, err := d.u32()
		if err != nil {
			return "", err
		}
		n = int(u)
	}
	s, err := d.take(n + 1) // the text and its NUL
	if err != nil {
		return "", err
	}
	return string(s[:n]), nil
}

func (d *decoder) value(t string) (any, error) {
	d.depth++
	defer func() { d.depth-- }()
	if d.depth > 64 {
		return nil, errors.New("dbus: nested too deeply")
	}
	if err := d.align(alignment(t[0])); err != nil {
		return nil, err
	}
	switch t[0] {
	case 'y':
		s, err := d.take(1)
		if err != nil {
			return nil, err
		}
		return s[0], nil
	case 'b':
		u, err := d.u32()
		return u != 0, err
	case 'n', 'q':
		s, err := d.take(2)
		if err != nil {
			return nil, err
		}
		if t[0] == 'n' {
			return int16(d.order.Uint16(s)), nil
		}
		return d.order.Uint16(s), nil
	case 'i':
		u, err := d.u32()
		return int32(u), err
	case 'u', 'h':
		return d.u32()
	case 'x', 't', 'd':
		s, err := d.take(8)
		if err != nil {
			return nil, err
		}
		u := d.order.Uint64(s)
		switch t[0] {
		case 'x':
			return int64(u), nil
		case 't':
			return u, nil
		}
		return math.Float64frombits(u), nil
	case 's', 'o':
		return d.str(4)
	case 'g':
		return d.str(1)
	case 'v':
		sig, err := d.str(1)
		if err != nil {
			return nil, err
		}
		vt, rest, err := nextType(sig)
		if err != nil || rest != "" {
			return nil, errors.New("dbus: bad variant signature")
		}
		return d.value(vt)
	case '(', '{':
		members, err := inner(t)
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, len(members))
		for _, m := range members {
			v, err := d.value(m)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 'a':
		n, err := d.u32()
		if err != nil {
			return nil, err
		}
		elem := t[1:]
		if err := d.align(alignment(elem[0])); err != nil {
			return nil, err
		}
		end := d.off + int(n)
		if int(n) < 0 || end > len(d.b) {
			return nil, errShort
		}
		if elem[0] == '{' {
			m := map[string]any{}
			for d.off < end {
				kv, err := d.value(elem)
				if err != nil {
					return nil, err
				}
				pair := kv.([]any)
				if len(pair) == 2 {
					m[fmt.Sprint(pair[0])] = pair[1]
				}
			}
			return m, nil
		}
		out := []any{}
		for d.off < end {
			v, err := d.value(elem)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	return nil, fmt.Errorf("dbus: cannot decode type %q", t)
}

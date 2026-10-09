package winsys

import (
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// parsePlist reads an XML property list (what macOS's "defaults export" writes) into Go values: map[string]any for a
// dict, []any for an array, string, bool, int64 and float64. Data and dates are kept as their text.
func parsePlist(r io.Reader) (any, error) {
	d := xml.NewDecoder(r)
	d.Strict = false
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local != "plist" {
			return plistValue(d, se, 0)
		}
	}
}

func plistValue(d *xml.Decoder, se xml.StartElement, depth int) (any, error) {
	if depth > 32 {
		return nil, errors.New("plist: nested too deeply")
	}
	switch se.Name.Local {
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					if key, err = plistText(d); err != nil {
						return nil, err
					}
					continue
				}
				v, err := plistValue(d, t, depth+1)
				if err != nil {
					return nil, err
				}
				m[key] = v
			case xml.EndElement:
				return m, nil
			}
		}
	case "array":
		out := []any{}
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := plistValue(d, t, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			case xml.EndElement:
				return out, nil
			}
		}
	case "true", "false":
		if err := d.Skip(); err != nil {
			return nil, err
		}
		return se.Name.Local == "true", nil
	case "integer":
		s, err := plistText(d)
		if err != nil {
			return nil, err
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n, nil
	case "real":
		s, err := plistText(d)
		if err != nil {
			return nil, err
		}
		f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return f, nil
	default: // string, data, date
		return plistText(d)
	}
}

// plistText reads the text of the element just opened, up to its end.
func plistText(d *xml.Decoder) (string, error) {
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), nil
		case xml.StartElement:
			if err := d.Skip(); err != nil {
				return "", err
			}
		}
	}
}

// oldStylePlist writes a list of string lists the way "defaults write" reads a value: (("a", "b"), ("c", "d")).
func oldStylePlist(rows [][]string) string {
	q := func(s string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
	}
	parts := make([]string, len(rows))
	for i, r := range rows {
		cells := make([]string, len(r))
		for j, c := range r {
			cells[j] = q(c)
		}
		parts[i] = "(" + strings.Join(cells, ", ") + ")"
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

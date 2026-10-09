package artwork

import (
	"math"
	"strconv"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
)

// OMap is an order-preserving JSON object (see jsonx).
type OMap = jsonx.Obj

func parseJSON(s string) (any, error) { return jsonx.Parse(s) }

// ---- JavaScript-style accessors -------------------------------------------------------

func asObj(v any) *OMap { o, _ := v.(*OMap); return o }
func asArr(v any) []any { a, _ := v.([]any); return a }

// get walks properties; anything missing or of the wrong shape is nil (undefined).
func get(v any, path ...string) any {
	for _, k := range path {
		o, ok := v.(*OMap)
		if !ok {
			return nil
		}
		v = o.M[k]
	}
	return v
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	}
	return true
}

// str is String(v) for the primitives that turn up in catalog data (nil gives "").
func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return numStr(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = str(e)
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

// tmpl is how a template literal prints a value: missing properties read "undefined".
func tmpl(v any) string {
	if v == nil {
		return "undefined"
	}
	return str(v)
}

func numStr(f float64) string { return jsonx.NumStr(f) }

// num is Number(v) (NaN when it isn't a number).
func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case bool:
		if x {
			return 1
		}
		return 0
	case nil:
		return math.NaN()
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	}
	return math.NaN()
}

// numOK reports a real (non-NaN) number value.
func numOK(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok && !math.IsNaN(f)
}

func isStr(v any) bool { _, ok := v.(string); return ok }

// orStr is `v || fallback` for string-ish values.
func orStr(v any, fallback string) string {
	if truthy(v) {
		return str(v)
	}
	return fallback
}

func strs(v any) []string {
	var out []string
	for _, e := range asArr(v) {
		out = append(out, str(e))
	}
	return out
}

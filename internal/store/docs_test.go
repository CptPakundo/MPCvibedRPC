package store

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// Every setting, with its default, has a row in docs/config.md: the program's settings (core.Config) and its own
// (App). The row's default must be the code's. A setting that only tests read is documented as internal.
func TestEverySettingIsDocumented(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "config.md"))
	if err != nil {
		t.Fatal(err)
	}
	// key -> the default as written; a row that documents several keys at once (the parts of a sign-in) has no single
	// default to compare, so those get "?"
	rows := map[string]string{}
	for _, m := range regexp.MustCompile("(?m)^\\| ((?:`\\w+`,? ?)+) \\| (.*?) \\|(?: |\r?$)").FindAllStringSubmatch(string(b), -1) {
		keys := regexp.MustCompile("`(\\w+)`").FindAllStringSubmatch(m[1], -1)
		for _, k := range keys {
			if len(keys) > 1 {
				rows[k[1]] = "?"
			} else {
				rows[k[1]] = m[2]
			}
		}
	}
	check := func(v reflect.Value) {
		rt := v.Type()
		for i := 0; i < rt.NumField(); i++ {
			key := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
			cell, ok := rows[key]
			if !ok {
				t.Errorf("%s has no row in docs/config.md", key)
				continue
			}
			if want, got := docDefault(cell), codeDefault(v.Field(i)); want != got && got != "?" && cell != "?" {
				t.Errorf("%s: docs/config.md says the default is %q, the code says %q", key, want, got)
			}
		}
	}
	check(reflect.ValueOf(core.DefaultConfig()))
	check(reflect.ValueOf(defaultApp))
}

// docDefault reads a default as config.md writes it: in backticks (a remark may follow them), strings in single
// quotes, lists in brackets, an empty list or map as [] or {}, no value as (none).
func docDefault(cell string) string {
	s := strings.TrimSpace(cell)
	if m := regexp.MustCompile("`([^`]*)`").FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if s == "(none)" || s == "{}" || s == "[]" {
		return ""
	}
	if strings.HasPrefix(s, "[") {
		s = strings.NewReplacer("[", "", "]", "", ",", " ").Replace(s)
	}
	s = strings.ReplaceAll(s, "'", "")
	return strings.Join(strings.Fields(s), " ")
}

// codeDefault is a default the way docDefault writes it ("?" for a value the test does not compare).
func codeDefault(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Bool, reflect.Int, reflect.Float64, reflect.String:
		return fmt.Sprint(v.Interface())
	case reflect.Slice:
		parts := make([]string, v.Len())
		for i := range parts {
			parts[i] = fmt.Sprint(v.Index(i).Interface())
		}
		return strings.Join(parts, " ")
	case reflect.Map:
		if v.Len() == 0 {
			return ""
		}
	}
	return "?"
}

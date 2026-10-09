package core

// Small helpers the lookup packages share (JavaScript string semantics).

func U16Len(s string) int                { return u16len(s) }
func U16Slice(s string, a, b int) string { return u16slice(s, a, b) }
func Trim(s string) string               { return trim(s) }
func LowerJS(s string) string            { return Lower(s) }
func Itoa(n int) string                  { return itoa(n) }

func Contains(list []string, s string) bool { return contains(list, s) }

// ParseIntPrefix is parseInt(s, 10): ok=false for NaN.
func ParseIntPrefix(s string) (int, bool) { return parseIntJS(s) }

// ParseFloatPrefix is parseFloat(s) with NaN as 0.
func ParseFloatPrefix(s string) float64 { return parseFloat(s) }
func NewMedia() *Media                  { return newMedia() }

// ConfigAsMap returns every setting keyed by its config.json name (as JSON values).
func ConfigAsMap(c *Config) map[string]any {
	b, _ := jsonMarshal(c)
	var m map[string]any
	_ = jsonUnmarshal(b, &m)
	return m
}

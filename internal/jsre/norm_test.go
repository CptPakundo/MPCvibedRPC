package jsre

import "testing"

func TestNFD(t *testing.T) {
	cases := map[string]string{
		"Pokémon":  "Pokémon",
		"Amélie":   "Amélie",
		"ǖ":        "ǖ",
		"가":        "가",
		"각":        "각",
		"plain":    "plain",
		"ａｂ":       "ａｂ", // full-width letters have only compatibility decompositions: untouched by NFD
		"Nausicaä": "Nausicaä",
	}
	for in, want := range cases {
		if got := NFD(in); got != want {
			t.Errorf("NFD(%q) = %q, want %q", in, got, want)
		}
	}
	if StripAccents("Éternité") != "Eternite" {
		t.Error("StripAccents")
	}
}

func TestEngineBasics(t *testing.T) {
	re := MustCompile(`(?<=\p{L})-\s+(?=\p{Lu})`, "gu")
	if got := re.ReplaceStr("Title- Subtitle - x", "$&"); got != "Title- Subtitle - x" {
		t.Error(got)
	}
	if got := MustCompile(`(a+?)(b*)`, "").Exec("aaab"); got == nil || got.Str(1) != "a" {
		t.Errorf("lazy: %v", got)
	}
	if !MustCompile(`^(?:ab|a)(?!c)`, "i").Test("ABd") || MustCompile(`^ab(?!c)`, "").Test("abc") {
		t.Error("lookahead")
	}
	// features the package does not implement fail at compile time
	for _, c := range [][2]string{{`(a)\1`, ""}, {`(?<n>a)`, ""}, {`a`, "m"}, {`a`, "s"}, {`\p{Ll}`, "u"}, {`\u{61}`, "u"}, {`(?=a)*`, ""}} {
		if _, err := Compile(c[0], c[1]); err == nil {
			t.Errorf("/%s/%s compiled", c[0], c[1])
		}
	}
	// a pathological pattern gives up instead of hanging
	if MustCompile(`(a*)*b`, "").Test("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Error("should not match")
	}
}

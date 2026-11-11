package jsre

import (
	"strings"
	"testing"
)

// A lazy "anything" that never finds its end is run again from every start: the square of the input. The search
// gives up (no match) instead of keeping a processor busy for minutes. The test lowers the limit, so that it need
// not spend the seconds the real one allows; no test of this package runs in parallel.
func TestHopelessLazyScanGivesUp(t *testing.T) {
	defer func(n int) { scanLimit = n }(scanLimit)
	scanLimit = 2_000_000
	re := MustCompile(`<p\s+id="([A-Za-z]+)"\s*>([\s\S]*?)<\/p>`, "g")
	// 5,000 starts, each running to the end of 50,000 characters: 125 million steps without the limit
	if m := re.Exec(strings.Repeat(`<p id="a">`, 5000)); m != nil {
		t.Fatal("there is nothing to match")
	}
	// a page that does have its ends still matches, however long its values are
	ok := `<p id="file">` + strings.Repeat("x", 200000) + `</p>`
	if m := re.Exec(ok); m == nil || m.Str(1) != "file" || len(m.Str(2)) != 200000 {
		t.Error("a long value between its tags must match")
	}
	// and a search that finds nothing on a long text costs one step per start, far below the limit
	if MustCompile(`zz+top`, "").Test(strings.Repeat("a", 1_000_000)) {
		t.Error("nothing to find")
	}
}

// A general repetition nests calls for every round. A run of hundreds of thousands gives up (no match) long before
// the stack could grow to the size that ends the program.
func TestEndlessRepetitionGivesUp(t *testing.T) {
	re := MustCompile(`(?:&#\d+;)+`, "")
	if m := re.Exec(strings.Repeat("&#1;", 300000)); m != nil {
		t.Error("a run beyond the depth limit reports no match")
	}
	if m := re.Exec(strings.Repeat("&#1;", 5000)); m == nil || len(m.Str(0)) != 20000 {
		t.Error("an ordinary run must match whole")
	}
}

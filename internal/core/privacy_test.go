package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMatchesHideList(t *testing.T) {
	const file = "Some.Show.S01E02.720p.mkv"
	const dir = `D:\Videos\Private\Season 1`
	yes := []string{
		"private",                // a folder name, any case
		`D:\Videos\Private`,      // a folder path, with backslashes
		"d:/videos/private/",     // the same with slashes and a trailing slash
		"some.show",              // part of the file name
		"SEASON 1",               // case does not matter
		"*.mkv",                  // wildcard on the file name
		"some.show.s??e02.*",     // ? and * together
		"d:/videos/*/season 1/*", // wildcard on the whole path
		"*private*",              // wildcard on the path
		"  private  ",            // spaces around an entry are ignored
	}
	for _, e := range yes {
		if !MatchesHideList(file, dir, []string{"unrelated", e}) {
			t.Errorf("entry %q should match %s\\%s", e, dir, file)
		}
	}
	// none of these match (a wildcard must match the whole file name or the whole path)
	for _, e := range []string{"", "   ", "public", `D:\Videos\Public`, "*.mp4", "some.other*", "s01e03", "*season 2*", "e:/videos"} {
		if MatchesHideList(file, dir, []string{e}) {
			t.Errorf("entry %q must not match", e)
		}
	}
	if MatchesHideList(file, dir, nil) || MatchesHideList("", dir, []string{"private"}) {
		t.Error("an empty list, or no file, never matches")
	}
	// a file with no known folder is still matched on its name
	if !MatchesHideList(file, "", []string{"s01e02"}) || MatchesHideList(file, "", []string{"private"}) {
		t.Error("matching without a folder")
	}
}

func TestWildMatch(t *testing.T) {
	cases := []struct {
		p, s string
		want bool
	}{
		{"*", "", true}, {"*", "abc", true}, {"a*c", "abc", true}, {"a*c", "ac", true}, {"a*c", "abd", false},
		{"a?c", "abc", true}, {"a?c", "ac", false}, {"*b*", "abc", true}, {"*b*", "ac", false}, {"a**c", "abbbc", true},
		{"abc", "abc", true}, {"abc", "abcd", false}, {"", "", true}, {"", "a", false}, {"?", "é", true},
	}
	for _, c := range cases {
		if got := wildMatch(c.p, c.s); got != c.want {
			t.Errorf("wildMatch(%q, %q) = %v, want %v", c.p, c.s, got, c.want)
		}
	}
}

func TestHiddenActivityRevealsNothing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowArtwork, cfg.RichInfo, cfg.LinkButton, cfg.TitleAsName = true, true, true, true
	const secret = "Very.Private.Show.S03E04.1080p"
	for _, state := range []int{2, 1} {
		info := &Info{File: secret + ".mkv", Dir: []string{"SecretFolder", "Videos"}, FullDir: `D:\Videos\SecretFolder`, State: state, Position: 600000, Duration: 2700000, Rate: 1}
		a := HiddenActivity(info, &cfg, 1_700_000_000_000)
		if a == nil {
			t.Fatalf("state %d: nil activity", state)
		}
		b, _ := json.Marshal(a)
		js := strings.ToLower(string(b))
		for _, leak := range []string{"private", "secret", "show", "s03e04", "1080p", ".mkv", "videos", "http"} {
			if strings.Contains(js, leak) {
				t.Errorf("state %d: the hidden status mentions %q: %s", state, leak, b)
			}
		}
		if a.Details != "Watching a video" || len(a.Buttons) != 0 {
			t.Errorf("state %d: details=%q buttons=%v", state, a.Details, a.Buttons)
		}
		if a.Assets == nil || a.Assets.LargeImage != cfg.LargeImageKey {
			t.Errorf("state %d: only the app's own asset key may be used: %+v", state, a.Assets)
		}
		if state == 2 && (a.Timestamps == nil || a.Timestamps.End == 0) {
			t.Errorf("a playing hidden status should still show progress: %+v", a.Timestamps)
		}
	}
	if HiddenActivity(nil, &cfg, 0) != nil {
		t.Error("nil info, nil activity")
	}
	// nothing is built for a stopped player, just like the normal status
	if HiddenActivity(&Info{File: "x.mkv", State: 0}, &cfg, 0) != nil {
		t.Error("a stopped player has no status")
	}
}

package capture

import (
	"testing"
	"time"
)

func TestSharing(t *testing.T) {
	now := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	here := func(string) bool { return true }
	gone := func(string) bool { return false }
	const discord = `C:\Users\someone\AppData\Local\Discord\app-1.0.9300\Discord.exe`
	for _, c := range []struct {
		name    string
		entries []entry
		exists  func(string) bool
		want    bool
	}{
		{"no record at all", nil, here, false},
		{"a stream that ended", []entry{{discord, now.Add(-time.Hour), now.Add(-time.Minute)}}, here, false},
		{"a stream that is on", []entry{{discord, now.Add(-time.Minute), time.Time{}}}, here, true},
		{"the picker's previews, a second old", []entry{{discord, now.Add(-time.Second), time.Time{}}}, here, false},
		{"just past the wait", []entry{{discord, now.Add(-minLive), time.Time{}}}, here, true},
		{"a leftover of a version that is gone", []entry{{discord, now.Add(-90 * 24 * time.Hour), time.Time{}}}, gone, false},
		{"never captured", []entry{{discord, time.Time{}, time.Time{}}}, here, false},
		{"another program captures", []entry{{`C:\Program Files\Sample Recorder\recorder.exe`, now.Add(-time.Minute), time.Time{}}}, here, false},
		{"the test edition of Discord", []entry{{`C:\Users\someone\AppData\Local\DiscordPTB\app-1.0.1100\DiscordPTB.exe`, now.Add(-time.Minute), time.Time{}}}, here, true},
		{"an old version ended, the current one is on", []entry{
			{`C:\Users\someone\AppData\Local\Discord\app-1.0.9299\Discord.exe`, now.Add(-48 * time.Hour), now.Add(-47 * time.Hour)},
			{discord, now.Add(-10 * time.Second), time.Time{}},
		}, here, true},
	} {
		if got := sharing(c.entries, now, c.exists); got != c.want {
			t.Errorf("%s: sharing = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsDiscord(t *testing.T) {
	for path, want := range map[string]bool{
		`C:\Users\someone\AppData\Local\Discord\app-1.0.9300\Discord.exe`: true,
		`C:\x\DISCORDCANARY.EXE`:      true,
		`C:\x\NotDiscord.exe`:         false,
		`C:\Discord.exe\recorder.exe`: false,
		`Discord.exe`:                 true,
		``:                            false,
	} {
		if got := isDiscord(path); got != want {
			t.Errorf("isDiscord(%q) = %v, want %v", path, got, want)
		}
	}
}

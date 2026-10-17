package potplayer

import "testing"

func TestInfo(t *testing.T) {
	for _, c := range []struct {
		name                  string
		status, pos, dur, spd int
		path, title           string
		file, dir             string
		state                 int
		rate                  float64
	}{
		{"playing a file", 2, 61000, 1440000, 1000, `D:\Videos\Sample Show\Sample Show - S01E02.mkv`, "Sample Show - S01E02.mkv - PotPlayer",
			"Sample Show - S01E02.mkv", `D:\Videos\Sample Show`, 2, 1},
		{"paused, faster", 1, 5000, 60000, 1500, `D:\Videos\Sample Movie (2020).mp4`, "", "Sample Movie (2020).mp4", `D:\Videos`, 1, 1.5},
		{"a stream has no folder", 2, 0, 0, 0, "https://media.example/live/stream.m3u8", "", "stream.m3u8", "", 2, 1},
		{"no path: the title", 2, 1000, 2000, 0, "", "Sample Movie (2020).mkv - PotPlayer", "Sample Movie (2020).mkv", "", 2, 1},
		{"stopped, empty window", 0, 0, 0, 1000, "", "PotPlayer", "", "", 0, 1},
		{"nonsense answers", 7, -5, -1, 99999, "", "", "", "", -1, 1},
	} {
		in := info(c.status, c.pos, c.dur, c.spd, c.path, c.title)
		if in.File != c.file || in.FullDir != c.dir || in.State != c.state || in.Rate != c.rate || in.Position < 0 || in.Duration < 0 {
			t.Errorf("%s: %+v", c.name, in)
		}
	}
}

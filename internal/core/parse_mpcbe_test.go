package core

import "testing"

// MPC-BE's /variables.html lists the same variables as MPC-HC's (state 0/1/2, times in milliseconds).
func TestParseVariablesMpcBe(t *testing.T) {
	page := `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">
<html><head><title>MPC-BE WebServer - Variables</title></head><body>
<p id="file">Example.Show.S01E02.1080p.mkv</p>
<p id="filepatharg">D%3A%5CVideos%5CExample.Show.S01E02.1080p.mkv</p>
<p id="filepath">D:\Videos\Example Show\Example.Show.S01E02.1080p.mkv</p>
<p id="filedirarg">D%3A%5CVideos%5CExample%20Show</p>
<p id="filedir">D:\Videos\Example Show</p>
<p id="state">2</p>
<p id="statestring">Playing</p>
<p id="position">65000</p>
<p id="positionstring">00:01:05</p>
<p id="duration">1500000</p>
<p id="durationstring">00:25:00</p>
<p id="volumelevel">100</p>
<p id="muted">0</p>
<p id="playbackrate">1</p>
<p id="size">1 MB</p>
<p id="reloadtime">0</p>
<p id="hdr">n/a</p>
<p id="version">1.7.0</p>
</body></html>`
	in := ParseVariables(page)
	if in == nil {
		t.Fatal("not recognised")
	}
	if in.File != "Example.Show.S01E02.1080p.mkv" || in.State != 2 || in.Position != 65000 || in.Duration != 1500000 || in.Rate != 1 {
		t.Errorf("%+v", in)
	}
	if in.FullDir != `D:\Videos\Example Show` {
		t.Errorf("dir %q", in.FullDir)
	}
}

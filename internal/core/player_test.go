package core

import "testing"

// The pages below copy the markup of the real players (MPC-HC 2.8.3, MPC-BE 1.9.1, MPC-QT 26.07) as seen on
// their web interfaces; only the file names are made up.
func TestPlayerOf(t *testing.T) {
	const hc = `<title>MPC-HC WebServer - Variables</title></head><body class="page-variables">
<p id="file">%s</p><p id="state">2</p><p id="playbackrate">1.000000</p><p id="version">2.8.3.0</p></body>`
	const be = `<title>MPC-BE WebServer - Variables</title></head><body>
<p id="file">%s</p><p id="state">2</p><p id="playbackrate">1</p><p id="version">1.9.1</p></body>`
	const qt = `<title>MPC-HC WebServer - Variables</title></head><body class="page-variables">
       <p id="file">%s</p>
       <p id="playbackRate">1.24611e-306</p>
       <p id="version">26.07</p></body>`
	page := func(tmpl, file string) string { return replaceOnce(tmpl, "%s", file) }
	for _, c := range []struct{ html, want string }{
		{page(hc, "Sample.Show.S01E02.mkv"), "MPC-HC"},
		{page(be, "Sample.Show.S01E02.mkv"), "MPC-BE"},
		{page(qt, "Sample.Show.S01E02.mkv"), "MPC-QT"},
		// a file name never decides the player
		{page(hc, "Review of MPC-BE and the playbackRate setting.mkv"), "MPC-HC"},
		{`<p id="state">2</p>`, "MPC-HC"},
	} {
		if got := PlayerOf(c.html); got != c.want {
			t.Errorf("PlayerOf(%q) = %q, want %q", c.html, got, c.want)
		}
	}
}

func replaceOnce(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

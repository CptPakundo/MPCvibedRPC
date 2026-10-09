package core

import "testing"

func TestPlayerOf(t *testing.T) {
	for html, want := range map[string]string{
		`<title>MPC-HC WebServer - Variables</title><p id="playbackrate">1</p>`:                         "MPC-HC",
		`<title>MPC-BE WebServer - Variables</title><p id="playbackrate">1</p>`:                         "MPC-BE",
		`<title>MPC-HC WebServer - Variables</title><body class="page-variables"><p id="playbackRate">`: "MPC-QT",
		`<p id="state">2</p>`: "MPC-HC",
	} {
		if got := PlayerOf(html); got != want {
			t.Errorf("PlayerOf(%q) = %q, want %q", html, got, want)
		}
	}
}

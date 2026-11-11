package winsys

import (
	"strings"
	"testing"
	"time"
)

func TestStartupDisabled(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
		want bool
	}{
		{"nil", nil, false},
		{"empty", []byte{}, false},
		{"enabled 02", []byte{0x02, 0, 0, 0}, false},
		{"disabled 03", []byte{0x03, 0, 0, 0}, true},
		{"enabled 06", []byte{0x06, 0, 0, 0}, false},
		{"disabled 07", []byte{0x07, 0, 0, 0}, true},
	} {
		if got := startupDisabled(c.data); got != c.want {
			t.Errorf("%s: startupDisabled = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWebAddress(t *testing.T) {
	for u, want := range map[string]bool{
		"https://example.com/login?a=1":  true,
		"http://127.0.0.1:8080/?t=x":     true,
		"HTTPS://example.com":            true,
		"":                               false,
		"example.com":                    false,
		"file:///C:/Windows/notepad.exe": false,
		`C:\Windows\notepad.exe`:         false,
		"javascript:alert(1)":            false,
		"mailto:someone@example.com":     false,
		"http://":                        false,
		"https:///path":                  false,
		"ms-settings:":                   false,
	} {
		if got := webAddress(u); got != want {
			t.Errorf("webAddress(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestSetupRunsOnceAtATime(t *testing.T) {
	setupLock.Lock()
	defer setupLock.Unlock()
	r := EnableMpcWebInterface(13579, true, "", 0, "", "")
	if r.OK || !strings.Contains(r.Message, "already running") {
		t.Errorf("a second setup gave %+v, want the already-running answer", r)
	}
}

func TestWaitUntilGone(t *testing.T) {
	if !waitUntilGone(func() bool { return false }, time.Millisecond, time.Second) {
		t.Error("players that are gone must end the wait at once")
	}
	calls := 0
	if !waitUntilGone(func() bool { calls++; return calls < 4 }, time.Millisecond, 5*time.Second) || calls != 4 {
		t.Errorf("a player that leaves after 3 checks: calls = %d", calls)
	}
	start := time.Now()
	if waitUntilGone(func() bool { return true }, 5*time.Millisecond, 40*time.Millisecond) {
		t.Error("a player that stays must make the wait give up")
	}
	if d := time.Since(start); d < 40*time.Millisecond || d > 2*time.Second {
		t.Errorf("gave up after %v, want about 40ms", d)
	}
}

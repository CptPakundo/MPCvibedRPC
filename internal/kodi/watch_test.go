package kodi

import (
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventAddress(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:8080":            "127.0.0.1:9090",
		"http://192.168.1.30:8080/": "192.168.1.30:9090",
		"kodi.local:8080/jsonrpc":   "kodi.local:9090",
		"http://[fd00::5]:8080":     "[fd00::5]:9090",
		"":                          "",
	} {
		if got := EventAddress(in); got != want {
			t.Errorf("EventAddress(%q) = %q; want %q", in, got, want)
		}
	}
}

// Watch reports play, pause, seek and stop notifications, which Kodi sends back to back, and returns at the end.
func TestWatch(t *testing.T) {
	kodi, ours := net.Pipe()
	var changes atomic.Int32
	done := make(chan error, 1)
	go func() { done <- Watch(ours, func() { changes.Add(1) }) }()
	go func() {
		kodi.Write([]byte(`{"jsonrpc":"2.0","method":"Player.OnPause","params":{"data":{"player":{"playerid":1}},"sender":"xbmc"}}` +
			`{"jsonrpc":"2.0","method":"Application.OnVolumeChanged","params":{"data":{"volume":50}}}` +
			`{"jsonrpc":"2.0","method":"Player.OnResume","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","method":"Player.OnSeek","params":{}}{"jsonrpc":"2.0","method":"Player.OnStop","params":{}}`))
		kodi.Close()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after Kodi closed")
	}
	if n := changes.Load(); n != 4 {
		t.Fatalf("%d changes reported; want 4", n)
	}
}

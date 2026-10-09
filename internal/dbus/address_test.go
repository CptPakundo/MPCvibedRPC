package dbus

import (
	"reflect"
	"testing"
)

func TestSocketPaths(t *testing.T) {
	cases := map[string][]string{
		"unix:path=/run/user/1000/bus":                       {"/run/user/1000/bus"},
		"unix:abstract=/tmp/dbus-AbC,guid=0123":              {"@/tmp/dbus-AbC"},
		"tcp:host=127.0.0.1,port=5;unix:path=/tmp/a%20b/bus": {"/tmp/a b/bus"},
		"unix:path=/x;unix:path=/y":                          {"/x", "/y"},
		"":                                                   nil,
		"launchd:env=DBUS_LAUNCHD_SESSION_BUS_SOCKET":        nil,
	}
	for in, want := range cases {
		if got := socketPaths(in); !reflect.DeepEqual(got, want) {
			t.Errorf("socketPaths(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSessionAddress(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/somewhere/bus")
	if got := SessionAddress(); got != "unix:path=/somewhere/bus" {
		t.Errorf("SessionAddress() = %q", got)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir()) // no bus socket there
	if got := SessionAddress(); got != "" {
		t.Errorf("SessionAddress() = %q, want empty", got)
	}
}

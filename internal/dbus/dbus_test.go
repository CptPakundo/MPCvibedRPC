package dbus_test

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus/dbustest"
)

func TestRoundTrip(t *testing.T) {
	meta := map[string]any{
		"xesam:title":   dbus.Variant{Sig: "s", Value: "Sample Movie (2020)"},
		"mpris:length":  dbus.Variant{Sig: "x", Value: int64(5400000000)},
		"xesam:artist":  dbus.Variant{Sig: "as", Value: []any{"Someone"}},
		"mpris:trackid": dbus.Variant{Sig: "o", Value: "/org/videolan/vlc/playlist/3"},
		"nested":        dbus.Variant{Sig: "a{sv}", Value: map[string]any{"n": dbus.Variant{Sig: "u", Value: uint32(7)}}},
	}
	m := &dbus.Message{Type: dbus.TypeMethodReturn, Serial: 9, ReplySerial: 4, Destination: ":1.5", Sender: ":1.2",
		Signature: "a{sv}ybdqnit(si)", Body: []any{meta, byte(3), true, 1.5, uint16(2), int16(-2), int32(-7), uint64(1 << 40), []any{"x", int32(1)}}}
	raw, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if raw[0] != 'l' {
		t.Fatalf("bad header %q", raw[:4])
	}
	got, err := dbus.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got.Serial != 9 || got.ReplySerial != 4 || got.Destination != ":1.5" || got.Sender != ":1.2" || got.Signature != m.Signature {
		t.Errorf("header: %+v", got)
	}
	wantMeta := map[string]any{
		"xesam:title": "Sample Movie (2020)", "mpris:length": int64(5400000000), "xesam:artist": []any{"Someone"},
		"mpris:trackid": "/org/videolan/vlc/playlist/3", "nested": map[string]any{"n": uint32(7)},
	}
	want := []any{wantMeta, byte(3), true, 1.5, uint16(2), int16(-2), int32(-7), uint64(1 << 40), []any{"x", int32(1)}}
	if !reflect.DeepEqual(got.Body, want) {
		t.Errorf("body:\n got %#v\nwant %#v", got.Body, want)
	}
}

// A message as a big-endian bus would send it: a method return carrying the string "ok" (written out by hand from the
// specification).
func TestBigEndian(t *testing.T) {
	raw := []byte{
		'B', 2, 0, 1, 0, 0, 0, 7, 0, 0, 0, 1, // body length 7, serial 1
		0, 0, 0, 15, // header fields: 15 bytes
		5, 1, 'u', 0, 0, 0, 0, 3, // REPLY_SERIAL (u) 3
		8, 1, 'g', 0, 1, 's', 0, // SIGNATURE (g) "s"
		0,                       // padding to 8
		0, 0, 0, 2, 'o', 'k', 0, // the body: "ok"
	}
	m, err := dbus.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != dbus.TypeMethodReturn || m.ReplySerial != 3 || m.Signature != "s" || !reflect.DeepEqual(m.Body, []any{"ok"}) {
		t.Errorf("got %+v", m)
	}
}

func TestMalformedNeverPanics(t *testing.T) {
	m := &dbus.Message{Type: dbus.TypeMethodReturn, Serial: 1, ReplySerial: 1, Signature: "a{sv}as",
		Body: []any{map[string]any{"k": dbus.Variant{Sig: "s", Value: "v"}}, []any{"a", "b"}}}
	raw, _ := m.Marshal()
	for n := 0; n < len(raw); n++ {
		_, _ = dbus.ReadMessage(bytes.NewReader(raw[:n])) // must return an error, not panic
	}
	// lengths that point past the end
	for i := 16; i < len(raw); i++ {
		bad := append([]byte{}, raw...)
		bad[i] = 0xff
		_, _ = dbus.ReadMessage(bytes.NewReader(bad))
	}
}

func TestMarshalRejectsWrongValues(t *testing.T) {
	for _, m := range []*dbus.Message{
		{Signature: "s", Body: []any{5}},
		{Signature: "ss", Body: []any{"one"}},
		{Signature: "a{sv}", Body: []any{map[string]any{"k": "not a variant"}}},
		{Signature: "z", Body: []any{1}},
	} {
		if _, err := m.Marshal(); err == nil {
			t.Errorf("%q %v should be refused", m.Signature, m.Body)
		}
	}
}

func TestCall(t *testing.T) {
	bus := dbustest.New(t)
	bus.Own("org.example.Thing", func(m *dbus.Message) dbustest.Reply {
		if m.Member == "Echo" {
			return dbustest.Reply{Sig: "s", Body: []any{"echo: " + m.Body[0].(string)}}
		}
		return dbustest.Reply{Error: "org.freedesktop.DBus.Error.UnknownMethod"}
	})
	c, err := dbus.Dial(bus.Address, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got, err := c.Call("org.example.Thing", "/", "org.example.Thing", "Echo", time.Second, "s", "hi")
	if err != nil || !reflect.DeepEqual(got, []any{"echo: hi"}) {
		t.Fatalf("Echo = %v, %v", got, err)
	}
	names, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "ListNames", time.Second, "")
	if err != nil || len(names) != 1 {
		t.Fatalf("ListNames = %v, %v", names, err)
	}
	if !reflect.DeepEqual(names[0], []any{"org.freedesktop.DBus", ":1.1", "org.example.Thing"}) {
		t.Errorf("names = %v", names[0])
	}
	_, err = c.Call("org.example.Thing", "/", "org.example.Thing", "Nope", time.Second, "")
	var de *dbus.Error
	if !errors.As(err, &de) || de.Name != "org.freedesktop.DBus.Error.UnknownMethod" || de.Message != "fake error" {
		t.Errorf("error reply = %v", err)
	}
	if _, err := c.Call("org.example.Missing", "/", "x.y", "Z", time.Second, ""); err == nil {
		t.Error("a missing name should fail")
	}
}

func TestDialFailures(t *testing.T) {
	for _, a := range []string{"", "tcp:host=localhost,port=1", "unix:path=/nonexistent/socket/here"} {
		if c, err := dbus.Dial(a, 200*time.Millisecond); err == nil {
			c.Close()
			t.Errorf("Dial(%q) should fail", a)
		}
	}
}

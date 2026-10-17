package mpris

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus/dbustest"
)

// Watch reports a player that pauses, seeks or changes file, and nothing else, and returns when the bus goes away.
func TestWatch(t *testing.T) {
	bus := dbustest.New(t)
	var changes atomic.Int32
	done := make(chan error, 1)
	c, err := dbus.Dial(bus.Address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	go func() { done <- Watch(c, time.Second, func() { changes.Add(1) }) }()
	deadline := time.Now().Add(3 * time.Second)
	for bus.Watchers() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("Watch did not ask for signals")
		}
		time.Sleep(10 * time.Millisecond)
	}
	props := func(iface string, changed map[string]any, invalid ...any) *dbus.Message {
		return &dbus.Message{Serial: 7, Path: objectPath, Interface: propsIface, Member: "PropertiesChanged", Sender: ":1.9",
			Signature: "sa{sv}as", Body: []any{iface, changed, invalid}}
	}
	for _, m := range []*dbus.Message{
		props(playerIface, map[string]any{"PlaybackStatus": v("s", "Paused")}),                                                               // 1
		props(playerIface, map[string]any{"Volume": v("d", 0.5)}),                                                                            // not news
		props("org.mpris.MediaPlayer2", map[string]any{"Identity": v("s", "Sample Player")}),                                                 // not the player
		props(playerIface, map[string]any{"Metadata": v("a{sv}", map[string]any{"xesam:title": v("s", "Sample Show")})}),                     // 2
		props(playerIface, map[string]any{}, "Rate"),                                                                                         // 3
		{Serial: 8, Path: objectPath, Interface: playerIface, Member: "Seeked", Sender: ":1.9", Signature: "x", Body: []any{int64(5000000)}}, // 4
	} {
		bus.Emit(m)
	}
	deadline = time.Now().Add(3 * time.Second)
	for changes.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if n := changes.Load(); n != 4 {
		t.Fatalf("%d changes reported; want 4", n)
	}
	bus.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Watch did not return when the bus went away")
	}
}

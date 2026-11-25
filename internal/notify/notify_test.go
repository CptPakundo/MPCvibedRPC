package notify

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPRP/internal/dbus"
	"github.com/CptPakundo/MPRP/internal/dbus/dbustest"
)

// fakeService puts a notification service on the fake bus and records what it is asked.
type fakeService struct {
	mu    sync.Mutex
	got   []*dbus.Message
	next  uint32
	error string // answer every Notify with this error
}

func (f *fakeService) handle(m *dbus.Message) dbustest.Reply {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, m)
	if f.error != "" {
		return dbustest.Reply{Error: f.error}
	}
	f.next++
	return dbustest.Reply{Sig: "u", Body: []any{f.next}}
}

func (f *fakeService) messages() []*dbus.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*dbus.Message(nil), f.got...)
}

func signal(member string, body ...any) *dbus.Message {
	sig := "uu"
	if member == "ActionInvoked" {
		sig = "us"
	}
	return &dbus.Message{Serial: 77, Path: object, Interface: service, Member: member, Sender: service, Signature: sig, Body: body}
}

func TestShowSendsNotify(t *testing.T) {
	bus := dbustest.New(t)
	f := &fakeService{}
	bus.Own(service, f.handle)
	n := &Notifier{Address: bus.Address, AppName: "MPRP"}
	defer n.Close()
	if !n.Show("MPRP 9.9.9 is available", "You have 9.9.8. Click here to see what's new and install it.") {
		t.Fatal("Show = false with a notification service on the bus")
	}
	got := f.messages()
	if len(got) != 1 {
		t.Fatalf("%d calls, want 1", len(got))
	}
	m := got[0]
	if m.Path != object || m.Interface != service || m.Member != "Notify" || m.Signature != "susssasa{sv}i" {
		t.Errorf("call %s %s.%s (%s)", m.Path, m.Interface, m.Member, m.Signature)
	}
	if m.Flags&dbus.FlagNoAutoStart != 0 {
		t.Error("Notify must let the bus start a notification service that starts on demand")
	}
	want := []any{"MPRP", uint32(0), "", "MPRP 9.9.9 is available", "You have 9.9.8. Click here to see what's new and install it.",
		[]any{"default", "Open settings"}, map[string]any{}, int32(-1)}
	if !reflect.DeepEqual(m.Body, want) {
		t.Errorf("body\n got %#v\nwant %#v", m.Body, want)
	}
	// the connection is kept for the next one (and for the clicks)
	if !n.Show("second", "text") || len(f.messages()) != 2 {
		t.Error("a second notification was not shown")
	}
	hello := 0
	for _, c := range bus.Calls() {
		if c == "org.freedesktop.DBus org.freedesktop.DBus.Hello" {
			hello++
		}
	}
	if hello != 1 {
		t.Errorf("%d connections, want 1 kept", hello)
	}
}

func TestClickCallsBack(t *testing.T) {
	bus := dbustest.New(t)
	f := &fakeService{}
	bus.Own(service, f.handle)
	clicked := make(chan struct{}, 4)
	n := &Notifier{Address: bus.Address, OnClick: func() { clicked <- struct{}{} }}
	defer n.Close()
	if !n.Show("title", "text") { // id 1
		t.Fatal("Show = false")
	}
	bus.Emit(signal("ActionInvoked", uint32(9), "default")) // someone else's notification
	bus.Emit(signal("ActionInvoked", uint32(1), "other"))   // another action of ours
	bus.Emit(signal("ActionInvoked", uint32(1), "default"))
	select {
	case <-clicked:
	case <-time.After(2 * time.Second):
		t.Fatal("a click on the notification did not call back")
	}
	select {
	case <-clicked:
		t.Fatal("a click on another notification or another action called back")
	case <-time.After(200 * time.Millisecond):
	}
	// once closed, its id may be reused by the service for another program's notification
	bus.Emit(signal("NotificationClosed", uint32(1), uint32(2)))
	time.Sleep(100 * time.Millisecond)
	bus.Emit(signal("ActionInvoked", uint32(1), "default"))
	select {
	case <-clicked:
		t.Fatal("a closed notification's id still called back")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestNoService(t *testing.T) {
	bus := dbustest.New(t) // a bus without a notification service
	n := &Notifier{Address: bus.Address}
	defer n.Close()
	if n.Show("title", "text") {
		t.Error("Show = true without a notification service")
	}
	f := &fakeService{error: "org.freedesktop.DBus.Error.AccessDenied"}
	bus.Own(service, f.handle)
	if n.Show("title", "text") {
		t.Error("Show = true although the service refused")
	}
	none := &Notifier{Address: "unix:path=/nonexistent/socket/here"}
	if none.Show("title", "text") {
		t.Error("Show = true without a bus")
	}
}

// A connection that ended (the bus restarted) is replaced at the next notification.
func TestReconnects(t *testing.T) {
	bus := dbustest.New(t)
	f := &fakeService{}
	bus.Own(service, f.handle)
	n := &Notifier{Address: bus.Address}
	defer n.Close()
	if !n.Show("one", "text") {
		t.Fatal("Show = false")
	}
	n.mu.Lock()
	n.conn.Close() // as if the bus had dropped it
	n.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	if !n.Show("two", "text") {
		t.Fatal("Show = false after the connection ended")
	}
	if len(f.messages()) != 2 {
		t.Errorf("%d notifications, want 2", len(f.messages()))
	}
}

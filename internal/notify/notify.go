// Package notify shows desktop notifications on Linux through the notification service of the session bus
// (org.freedesktop.Notifications, which GNOME, KDE Plasma, Xfce, Cinnamon, MATE and the usual stand-alone services
// such as dunst and mako offer). A click on a notification calls back, so the settings window can open. Each
// notification carries the program's logo as pixels (the image-data hint), so no icon file has to be installed.
package notify

import (
	"errors"
	"sync"
	"time"

	"github.com/CptPakundo/MPRP/internal/dbus"
	"github.com/CptPakundo/MPRP/internal/logo"
)

const (
	service = "org.freedesktop.Notifications"
	object  = "/org/freedesktop/Notifications"
	// defaultAction is the action of a click on the notification itself (not on a button).
	defaultAction = "default"
)

// iconSize is the logo's size in the notification, in pixels (64 KB of pixels; sharp on a high-DPI screen too).
const iconSize = 128

// icon is the image-data hint (iiibiiay): width, height, row stride, has alpha, bits per sample, channels and the
// pixels, RGBA without premultiplied alpha, as image.NRGBA holds them. Version 1.2 of the notification specification
// named it image-data (image_data in 1.1, icon_data before); every current service takes that name.
var icon = sync.OnceValue(func() dbus.Variant {
	img := logo.Image(iconSize, false)
	return dbus.Variant{Sig: "(iiibiiay)", Value: []any{int32(iconSize), int32(iconSize), int32(img.Stride), true, int32(8), int32(4), img.Pix}}
})

// timeout bounds connecting to the bus and each call.
var timeout = 2 * time.Second

// Notifier shows notifications. It connects to the bus at the first one and keeps that connection, so it hears
// the click (ActionInvoked comes to the connection that sent the notification). The zero value is not usable:
// set Address (empty: the user's session bus, dbus.SessionAddress) and OnClick.
type Notifier struct {
	Address string
	AppName string // the name the notification service shows
	OnClick func() // a notification of this Notifier was clicked; called on a goroutine of its own
	Log     func(level, msg string)

	mu   sync.Mutex // one Show at a time; guards conn
	conn *dbus.Conn
	idMu sync.Mutex      // guards ids (apart from mu: a signal can come while Show waits for its answer)
	ids  map[uint32]bool // notifications shown and not closed yet
}

// Show shows a notification with a title and a line of text, and reports whether the notification service took it.
// With no session bus or no notification service it returns false.
func (n *Notifier) Show(title, text string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	for try := 0; try < 2; try++ { // a connection kept from earlier may have ended meanwhile: one new try
		if n.conn == nil {
			if err := n.connect(); err != nil {
				n.log("INFO", "No desktop notification service ("+err.Error()+").")
				return false
			}
		}
		// actions: a click on the notification (the default action; its label is shown by services that show it as
		// a button); hints: the logo; expire_timeout -1: the service decides how long it stays
		got, err := n.conn.CallStart(service, object, service, "Notify", timeout, "susssasa{sv}i",
			n.AppName, uint32(0), "", title, text, []string{defaultAction, "Open settings"}, map[string]any{"image-data": icon()}, int32(-1))
		if err == nil {
			if len(got) > 0 {
				if id, ok := got[0].(uint32); ok && id != 0 {
					n.idMu.Lock()
					n.ids[id] = true
					n.idMu.Unlock()
				}
			}
			return true
		}
		var de *dbus.Error
		if errors.As(err, &de) { // the bus answered: there is no notification service, or it refused
			n.log("INFO", "The desktop notification service did not show the notification ("+err.Error()+").")
			return false
		}
		n.conn.Close()
		n.conn = nil
	}
	return false
}

// connect opens the connection and listens for the clicks and closings of notifications. n.mu is held.
func (n *Notifier) connect() error {
	addr := n.Address
	if addr == "" {
		addr = dbus.SessionAddress()
	}
	if addr == "" {
		return errors.New("no session bus")
	}
	c, err := dbus.Dial(addr, timeout)
	if err != nil {
		return err
	}
	n.idMu.Lock()
	n.ids = map[uint32]bool{}
	n.idMu.Unlock()
	c.Receive(n.signal)
	// the signals of the notification service (most services send them to the sender alone, some to everyone)
	rule := "type='signal',sender='" + service + "',interface='" + service + "',path='" + object + "'"
	if _, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "AddMatch", timeout, "s", rule); err != nil {
		c.Close()
		return err
	}
	n.conn = c
	return nil
}

// signal handles a signal of the notification service: a click on one of ours opens the window.
func (n *Notifier) signal(m *dbus.Message) {
	if m.Interface != service || len(m.Body) < 2 {
		return
	}
	id, _ := m.Body[0].(uint32)
	n.idMu.Lock()
	ours := n.ids[id]
	switch m.Member {
	case "ActionInvoked":
		if key, _ := m.Body[1].(string); key != defaultAction {
			ours = false
		}
	case "NotificationClosed":
		delete(n.ids, id)
		ours = false
	default:
		ours = false
	}
	n.idMu.Unlock()
	if ours && n.OnClick != nil {
		go n.OnClick()
	}
}

// Close ends the connection (when the program quits).
func (n *Notifier) Close() {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.conn != nil {
		n.conn.Close()
		n.conn = nil
	}
}

func (n *Notifier) log(level, msg string) {
	if n.Log != nil {
		n.Log(level, msg)
	}
}

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/CptPakundo/MPRP/internal/dbus"
	"github.com/CptPakundo/MPRP/internal/dbus/dbustest"
	"github.com/CptPakundo/MPRP/internal/notify"
	"github.com/CptPakundo/MPRP/internal/updater"
)

// With no tray icon (Linux), a version found in the background that will not install itself is announced once
// through the desktop's notification service: here a fake one on a fake session bus.
func TestUpdateNoticeOverTheBus(t *testing.T) {
	defer func(old bool) { notifyOverBus = old }(notifyOverBus)
	notifyOverBus = true
	defer func(old string) { updater.APIBase = old }(updater.APIBase)
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v99.0.0","body":"notes","html_url":"x","assets":[]}`)
	}))
	defer gh.Close()
	updater.APIBase = gh.URL

	bus := dbustest.New(t)
	var mu sync.Mutex
	var shown [][]any
	bus.Own("org.freedesktop.Notifications", func(m *dbus.Message) dbustest.Reply {
		mu.Lock()
		defer mu.Unlock()
		shown = append(shown, m.Body)
		return dbustest.Reply{Sig: "u", Body: []any{uint32(len(shown))}}
	})
	a, logs := autoApp(t)
	setApp(t, a.st, `{"checkUpdates":true,"autoUpdate":false}`)
	a.notifier = &notify.Notifier{Address: bus.Address, AppName: "MPRP", Log: a.log}
	defer a.notifier.Close()

	if _, err := a.checkUpdates(false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.checkUpdates(false); err != nil { // the same version again: no second notice
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(shown) != 1 {
		t.Fatalf("%d notices, want 1; log:\n%s", len(shown), logs())
	}
	if title, text := shown[0][3], shown[0][4]; title != "MPRP 99.0.0 is available" ||
		text != "You have "+version+". Click here to see what's new and install it." {
		t.Errorf("notice %q / %q", title, text)
	}
	if !strings.Contains(logs(), "Showed an update notice.") {
		t.Errorf("log:\n%s", logs())
	}
}

// Without a tray icon and away from Linux (macOS built without Cocoa), nothing is shown and nothing is asked.
func TestNoNoticeWithoutTrayOrBus(t *testing.T) {
	defer func(old bool) { notifyOverBus = old }(notifyOverBus)
	notifyOverBus = false
	a, _ := autoApp(t)
	if a.notify("title", "text") {
		t.Error("shown with neither a tray icon nor a notification service")
	}
	if a.getNotifier() != nil {
		t.Error("a notifier was made")
	}
}

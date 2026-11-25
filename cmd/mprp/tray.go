package main

import (
	"runtime"

	"github.com/CptPakundo/MPRP/internal/assets"
	"github.com/CptPakundo/MPRP/internal/engine"
	"github.com/CptPakundo/MPRP/internal/notify"
	"github.com/CptPakundo/MPRP/internal/winsys"
)

// startTray puts up the tray icon (Windows) or the menu bar icon (macOS); elsewhere there is none.
func (a *app) startTray() *winsys.Tray {
	return winsys.StartTray(winsys.TrayOptions{
		Icon: assets.Icon, Badged: assets.IconUpdate, Dir: a.st.Dir, Log: a.log,
		Badge:   func() bool { return a.upd.newer() != nil }, // the dot: an update waits
		Tooltip: func() string { return trayTooltip(a.eng.Status()) },
		Running: func() bool { return a.eng.Status().Running },
		OnOpen:  a.openWindow,
		OnToggle: func() {
			if a.eng.Status().Running {
				a.eng.Stop()
			} else {
				a.eng.Start()
			}
		},
		Hidden:   func() bool { return a.eng.Config().HideTitle },
		OnHide:   func() { setHideTitle(a.st, a.eng, a.log, !a.eng.Config().HideTitle) },
		Streams:  func() bool { return streamTitleOn(a.eng.Config().StreamTitle) },
		OnStream: func() { setStreamTitle(a.st, a.eng, a.log, !streamTitleOn(a.eng.Config().StreamTitle)) },
		OnQuit:   a.quit,
	})
}

// trayTooltip is the icon's tooltip and the menu's status line (Windows cuts tooltips at 63 characters).
func trayTooltip(s engine.Status) string {
	t := "MPRP - "
	switch {
	case !s.Running:
		t += "Not showing on Discord"
	case s.NowPlaying != nil:
		t += *s.NowPlaying
	case s.Discord == "connected" || s.Discord == "standby":
		t += "Ready"
	default:
		t += "Waiting for Discord"
	}
	if r := []rune(t); len(r) > 62 {
		t = string(r[:62])
	}
	return t
}

// notifyOverBus: with no tray icon, notifications go to the desktop's notification service over D-Bus (Linux).
var notifyOverBus = runtime.GOOS == "linux"

// notify shows a notification; a click on it opens the settings window. Windows shows it next to the tray icon, macOS
// through the menu bar app, Linux through the desktop's notification service. It reports whether it was shown.
func (a *app) notify(title, text string) bool {
	if t := a.getTray(); t != nil {
		return t.Balloon(title, text)
	}
	if !notifyOverBus {
		return false
	}
	a.mu.Lock()
	if a.notifier == nil {
		a.notifier = &notify.Notifier{AppName: "MPRP", OnClick: a.openWindow, Log: a.log}
	}
	n := a.notifier
	a.mu.Unlock()
	return n.Show(title, text)
}

func (a *app) getNotifier() *notify.Notifier {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.notifier
}

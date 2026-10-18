package main

import (
	"github.com/CptPakundo/MPCvibedRPC/internal/assets"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
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
		Hidden: func() bool { return a.eng.Config().HideTitle },
		OnHide: func() { setHideTitle(a.st, a.eng, a.log, !a.eng.Config().HideTitle) },
		OnQuit: a.quit,
	})
}

// trayTooltip is the icon's tooltip and the menu's status line (Windows cuts tooltips at 63 characters).
func trayTooltip(s engine.Status) string {
	t := "MPCvibedRPC - "
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

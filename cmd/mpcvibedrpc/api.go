package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/assets"
	"github.com/CptPakundo/MPCvibedRPC/internal/conns"
	"github.com/CptPakundo/MPCvibedRPC/internal/diag"
	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/server"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// fn adapts a handler that needs no request body and cannot fail.
func fn(f func() any) func(*jsonx.Obj) (any, error) {
	return func(*jsonx.Obj) (any, error) { return f(), nil }
}

// handlers are the local API the settings window calls (/api/<name>).
func (a *app) handlers() map[string]server.Handler {
	eng, log := a.eng, a.log
	h := map[string]server.Handler{
		"state":  {Get: true, Fn: fn(func() any { return a.fullState() })},
		"status": {Get: true, Fn: fn(func() any { return a.statusNow() })},
		"log":    {Get: true, Fn: fn(func() any { return map[string]any{"lines": a.lg.last(200)} })},
		"diagnostics": {Get: true, Fn: fn(func() any {
			return map[string]any{"text": diag.Report(diag.Input{
				Version: version, System: winsys.SystemInfo(), Players: winsys.RunningPlayerNames(),
				Config: eng.Config(), App: a.st.App(), Status: eng.Status(), Log: a.lg.last(100),
			})}
		})},
		"start":       {Fn: fn(func() any { eng.Start(); return eng.Status() })},
		"stop":        {Fn: fn(func() any { eng.Stop(); return eng.Status() })},
		"open":        {Fn: fn(func() any { a.openWindow(); return nil })},
		"quit":        {Fn: fn(func() any { time.AfterFunc(100*time.Millisecond, a.quit); return nil })},
		"open-folder": {Fn: fn(func() any { winsys.OpenFolder(a.st.Dir); return nil })},
		"clear-cache": {Fn: fn(func() any { eng.ClearArtworkCache(); log("INFO", "Cover cache cleared."); return nil })},
		"stats":       {Get: true, Fn: fn(func() any { return eng.Stats() })},
		"connections": {Get: true, Fn: fn(func() any { return map[string]any{"hosts": conns.List()} })},
		"stats-reset": {Fn: fn(func() any { eng.ResetStats(); log("INFO", "Stats reset."); return eng.Stats() })},
		"save":        {Fn: a.save},
		"reset":       {Fn: func(*jsonx.Obj) (any, error) { return a.reset() }},
		"mpc-web":     {Fn: a.setUpPlayers},

		"update-check": {Fn: func(*jsonx.Obj) (any, error) {
			r, err := a.checkUpdates(true)
			if err != nil {
				return nil, err
			}
			return r, nil
		}},
		"update-install": {Fn: func(*jsonx.Obj) (any, error) { return nil, a.installUpdate() }},

		"plex-signin": {Fn: func(*jsonx.Obj) (any, error) { return a.plex.signIn() }},
		"plex-servers": {Fn: func(*jsonx.Obj) (any, error) {
			list, err := a.plex.servers()
			if err != nil {
				return nil, err
			}
			return map[string]any{"servers": list}, nil
		}},
		"plex-server": {Fn: func(body *jsonx.Obj) (any, error) {
			id, _ := body.M["id"].(string)
			if err := a.plex.choose(id); err != nil {
				return nil, err
			}
			return a.plex.state(), nil
		}},
		"stremio-signin": {Fn: func(*jsonx.Obj) (any, error) { return a.stremio.signIn() }},
		"jellyfin-signin": {Fn: func(body *jsonx.Obj) (any, error) {
			address, _ := body.M["address"].(string)
			return a.jellyfin.signIn(address)
		}},
		"jellyfin-password": {Fn: func(body *jsonx.Obj) (any, error) {
			address, _ := body.M["address"].(string)
			user, _ := body.M["user"].(string)
			password, _ := body.M["password"].(string)
			return a.jellyfin.signInPassword(address, strings.TrimSpace(user), password)
		}},
	}
	addAccount(h, "plex", a.plex)
	addAccount(h, "stremio", a.stremio)
	addAccount(h, "jellyfin", a.jellyfin)
	return h
}

// fullState is everything the window draws itself from.
func (a *app) fullState() map[string]any {
	var upd any
	if u := a.upd.newer(); u != nil {
		upd = u
	}
	return map[string]any{
		"version": version, "schema": store.SectionsFor(runtime.GOOS), "os": runtime.GOOS, "values": a.st.Values(), "app": a.st.App(),
		"status": a.eng.Status(), "autoStart": winsys.GetAutoStart(a.exe), "dataDir": a.st.Dir, "update": upd,
		"changelog": json.RawMessage(assets.Changelog), "stats": a.eng.Stats(),
		"tray": a.getTray() != nil, // macOS: whether the menu bar icon is there (a build without it runs in the background)
	}
}

// status is what the window polls every second.
type status struct {
	engine.Status
	HideTitle bool          `json:"hideTitle"` // the window follows a switch made from the tray or the menu bar
	Update    *updater.Info `json:"update,omitempty"`
}

func (a *app) statusNow() status {
	return status{Status: a.eng.Status(), HideTitle: a.eng.Config().HideTitle, Update: a.upd.newer()}
}

// save stores what the window changed (one setting at a time) and hands it to the engine.
func (a *app) save(body *jsonx.Obj) (any, error) {
	before := a.st.App()
	if err := a.st.Update(body); err != nil {
		return nil, err
	}
	applySettings(a.st, a.eng)
	after := a.st.App()
	if pa, ok := body.M["app"].(*jsonx.Obj); ok {
		if _, has := pa.M["autoStart"]; has && !winsys.SetAutoStart(after.AutoStart, a.exe) {
			// the switch would only flip back at the next redraw, with "Saved." shown: say so, and keep the saved value
			// matching the system
			undo, _ := jsonx.Parse(`{"app":{"autoStart":` + strconv.FormatBool(!after.AutoStart) + `}}`)
			_ = a.st.Update(undo.(*jsonx.Obj))
			name := "Start at login"
			if winsys.IsWindows {
				name = "Start with Windows"
			}
			a.log("WARN", "Could not change "+name+".")
			return nil, errors.New("Could not change " + name + ": the system refused it.")
		}
	}
	if after.UpdateRepo != before.UpdateRepo {
		a.upd.set(nil)
	}
	a.log("INFO", "Settings saved.")
	return a.fullState(), nil
}

// reset is Restore defaults: every setting back to its default, signed out everywhere.
func (a *app) reset() (any, error) {
	a.plex.stopWaiting() // a sign-in waiting for approval must not save a token after the reset
	a.stremio.stopWaiting()
	a.stremio.revoke() // the reset forgets the Stremio key: end its session too
	a.jellyfin.stopWaiting()
	a.jellyfin.revoke() // and the Jellyfin or Emby sign-in
	if err := a.st.Reset(); err != nil {
		return nil, err
	}
	applySettings(a.st, a.eng)
	winsys.SetAutoStart(a.st.App().AutoStart, a.exe)
	a.upd.set(nil)
	a.log("INFO", "Settings restored to their defaults.")
	return a.fullState(), nil
}

// setUpPlayers is "Set up the player connection". A player that was already set up its own way (an mpv or IINA
// connection name, VLC's password and port) is adopted into the settings.
func (a *app) setUpPlayers(body *jsonx.Obj) (any, error) {
	closeMpc, _ := body.M["closeMpc"].(bool)
	cfg := a.eng.Config()
	r := winsys.EnableMpcWebInterface(cfg.Port, closeMpc, cfg.MpvPipe, cfg.VlcPort, cfg.VlcPassword, cfg.IinaPipe)
	take := map[string]any{}
	var adopted []string
	if r.MpvPipe != "" && r.MpvPipe != cfg.MpvPipe {
		take["mpvPipe"] = r.MpvPipe
		adopted = append(adopted, "Looking for mpv under the name it is set up with: "+r.MpvPipe+".")
	}
	if r.IinaPipe != "" && r.IinaPipe != cfg.IinaPipe {
		take["iinaPipe"] = r.IinaPipe
		adopted = append(adopted, "Looking for IINA under the name it is set up with: "+r.IinaPipe+".")
	}
	if r.VlcPassword != "" && (r.VlcPassword != cfg.VlcPassword || r.VlcPort != cfg.VlcPort) {
		take["vlcPassword"], take["vlcPort"] = r.VlcPassword, r.VlcPort
		adopted = append(adopted, fmt.Sprintf("Looking for VLC's web interface on port %d with its password.", r.VlcPort))
	}
	if len(take) > 0 && a.st.SetSettings(take) == nil {
		applySettings(a.st, a.eng)
		r.SettingsChanged = true
		for _, m := range adopted {
			a.log("INFO", m)
		}
	}
	return r, nil
}

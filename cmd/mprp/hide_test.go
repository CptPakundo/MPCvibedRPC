package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CptPakundo/MPRP/internal/capture"
	"github.com/CptPakundo/MPRP/internal/engine"
	"github.com/CptPakundo/MPRP/internal/store"
)

// "Hide what I'm watching" from the tray or the menu bar is saved like the switch in the window and reaches the engine.
func TestHideTitleFromTheMenu(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var logs []string
	lg := func(l, m string) { logs = append(logs, l+" "+m) }
	eng := engine.New(st.Config(), engine.Options{Log: lg})
	setHideTitle(st, eng, lg, true)
	if !st.Config().HideTitle || !eng.Config().HideTitle || st.Values()["hideTitle"] != true {
		t.Fatalf("on: saved %v, engine %v", st.Config().HideTitle, eng.Config().HideTitle)
	}
	setHideTitle(st, eng, lg, false)
	if st.Config().HideTitle || eng.Config().HideTitle {
		t.Fatal("off again")
	}
	if !strings.Contains(strings.Join(logs, "\n"), "Hide what I'm watching turned on from the menu.") {
		t.Fatal(logs)
	}
	if after := st.Config(); after.PauseClearMinutes != 30 || after.LinkButton != true {
		t.Fatalf("nothing else changes: %+v", after)
	}
}

// "Show the title on my streams" from the tray or the menu bar is saved and reaches the engine, and the window's
// status carries it (with the hide switch), so an open window follows the menu.
func TestStreamTitleFromTheMenuReachesTheWindow(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lg := func(string, string) {}
	eng := engine.New(st.Config(), engine.Options{Log: lg})
	a := &app{st: st, eng: eng, log: lg}
	if s := a.statusNow(); s.StreamTitle != "auto" || s.HideTitle {
		t.Fatalf("by default: %+v", s)
	}
	setStreamTitle(st, eng, lg, true)
	want := "auto" // where the program can tell when Discord shares the screen; otherwise all the time
	if !capture.Supported() {
		want = "always"
	}
	if s := a.statusNow(); s.StreamTitle != want || st.Values()["streamTitle"] != want {
		t.Errorf("on: status %q, saved %v, want %q", s.StreamTitle, st.Values()["streamTitle"], want)
	}
	setStreamTitle(st, eng, lg, false)
	want = "auto" // where the program cannot tell, never is "auto"
	if capture.Supported() {
		want = "off"
	}
	if s := a.statusNow(); s.StreamTitle != want {
		t.Errorf("off: status %q, want %q", s.StreamTitle, want)
	}
	b, err := json.Marshal(a.statusNow())
	if err != nil || !strings.Contains(string(b), `"streamTitle":"`+want+`"`) {
		t.Errorf("the window reads %s (%v)", b, err)
	}
}

package main

import (
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
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

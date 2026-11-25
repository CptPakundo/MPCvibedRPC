package core

import "testing"

func TestDefaultPauseClearIsThirtyMinutes(t *testing.T) {
	if got := DefaultConfig().PauseClearMinutes; got != 30 {
		t.Errorf("PauseClearMinutes default = %d, want 30", got)
	}
}

// The groups of Config (Settings, Accounts, Secrets, Hooks) are embedded: config.json stays one flat object, and every
// setting is read and written under its own name.
func TestConfigGroupsStayFlat(t *testing.T) {
	m := ConfigAsMap(&Config{})
	for _, group := range []string{"Settings", "Accounts", "Secrets", "Hooks"} {
		if _, ok := m[group]; ok {
			t.Errorf("config.json would hold a %q object", group)
		}
	}
	if len(m) != len(ConfigFields()) {
		t.Errorf("config.json has %d keys, Config %d settings", len(m), len(ConfigFields()))
	}
	cfg := LoadConfig([]byte(`{"plexToken":"t","clientId":"1","cinemetaBase":"http://127.0.0.1:1","plexUser":"u"}`))
	if cfg.PlexToken != "t" || cfg.ClientID != "1" || cfg.CinemetaBase != "http://127.0.0.1:1" || cfg.PlexUser != "u" {
		t.Errorf("a setting of a group was not loaded: %+v", cfg)
	}
}

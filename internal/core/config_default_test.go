package core

import "testing"

func TestDefaultPauseClearIsThirtyMinutes(t *testing.T) {
	if got := DefaultConfig().PauseClearMinutes; got != 30 {
		t.Errorf("PauseClearMinutes default = %d, want 30", got)
	}
}

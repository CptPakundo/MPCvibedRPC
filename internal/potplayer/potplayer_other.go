//go:build !windows

package potplayer

import "github.com/CptPakundo/MPCvibedRPC/internal/core"

// Query finds no PotPlayer outside Windows.
func Query() []*core.Info { return nil }

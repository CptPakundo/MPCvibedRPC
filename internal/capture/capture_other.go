//go:build !windows

package capture

import "context"

// Supported reports whether this system can tell: only Windows keeps a record that can be read.
func Supported() bool { return false }

// DiscordSharing reports whether a Discord desktop app is capturing the screen right now: unknown here, so no.
func DiscordSharing() bool { return false }

// Watch has nothing to watch here.
func Watch(context.Context, func()) {}

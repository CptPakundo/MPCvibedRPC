//go:build !windows

package store

// systemSealer is none on macOS and Linux: the secrets stay plain text there.
func systemSealer() sealer { return nil }

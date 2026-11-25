//go:build !darwin

package main

// prepareSystem has nothing to do outside macOS.
func prepareSystem() {}

// detach is only needed on macOS (see launch_darwin.go).
func detach() bool { return false }

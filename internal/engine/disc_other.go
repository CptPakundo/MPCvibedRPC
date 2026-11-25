//go:build !windows

package engine

// volumeLabel is not needed on macOS and Linux: a disc is mounted under a folder named after its label
// (/Volumes/SAMPLE_MOVIE, /media/<user>/SAMPLE_MOVIE), which core.DiscTitle reads like any folder.
func volumeLabel(string) string { return "" }

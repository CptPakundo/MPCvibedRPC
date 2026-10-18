// Package assets holds the files built into the program: the settings page and the icon.
package assets

import _ "embed"

//go:embed ui.html
var UI string

//go:embed icon.ico
var Icon []byte

// IconUpdate is the tray icon while an update waits: the same, with a red dot in its top right corner.
//
//go:embed icon-update.ico
var IconUpdate []byte

// Changelog is what changed in each version, newest first (shown in the window once after an update, and on demand).
//
//go:embed changelog.json
var Changelog string

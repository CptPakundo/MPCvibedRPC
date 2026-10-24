// Package assets holds the files built into the program: the settings page (markup, style sheet and script) and the icon.
package assets

import _ "embed"

//go:embed ui.html
var UI string

// CSS and JS are the page's style sheet and script, served as /ui.css and /ui.js (the page policy allows no inline ones).
//
//go:embed ui.css
var CSS string

//go:embed ui.js
var JS string

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

// Package assets holds the files built into the program: the settings page and the icon.
package assets

import _ "embed"

//go:embed ui.html
var UI string

//go:embed icon.ico
var Icon []byte

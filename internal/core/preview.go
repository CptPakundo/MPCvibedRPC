package core

import "strings"

// Preview is how the presence looks on Discord's card, for the settings window. It is built from the activity that
// was actually sent, so it shows what other people see.
type Preview struct {
	Watching   bool   `json:"watching"` // "Watching <name>" (otherwise "Playing <name>")
	Name       string `json:"name"`
	Details    string `json:"details"`
	State      string `json:"state"`
	LargeImage string `json:"largeImage,omitempty"` // an http(s) address; empty when only an uploaded Discord asset is used
	LargeText  string `json:"largeText,omitempty"`
	SmallText  string `json:"smallText,omitempty"`
	Start      int64  `json:"start,omitempty"` // epoch milliseconds
	End        int64  `json:"end,omitempty"`
	Button     string `json:"button,omitempty"` // label of the link button
}

// PreviewOf describes an activity for the window. The name falls back to the configured application name, which is
// what Discord shows when an activity carries none.
func PreviewOf(a *Activity, cfg *Config) *Preview {
	if a == nil {
		return nil
	}
	p := &Preview{Watching: a.Type == 3, Name: a.Name, Details: a.Details, State: a.State}
	if p.Name == "" && cfg != nil {
		p.Name = cfg.AppName
	}
	if a.Assets != nil {
		if strings.HasPrefix(a.Assets.LargeImage, "http://") || strings.HasPrefix(a.Assets.LargeImage, "https://") {
			p.LargeImage = a.Assets.LargeImage
		}
		p.LargeText, p.SmallText = a.Assets.LargeText, a.Assets.SmallText
	}
	if a.Timestamps != nil {
		p.Start, p.End = a.Timestamps.Start, a.Timestamps.End
	}
	if len(a.Buttons) > 0 {
		p.Button = a.Buttons[0].Label
	}
	return p
}

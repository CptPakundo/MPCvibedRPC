package engine

import (
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/artwork"
	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
)

// The stream status. Discord names a screen share after the streamer's Playing status and no other kind, and it keeps
// one status per connection to its app. So while Discord shares the screen ("auto"), the usual Watching status stays
// on the main connection, with its progress bar, and a second connection carries "Playing <title>", only for the
// stream's name. It has no button and no links: the Watching status has those. When the share ends, or nothing plays,
// the second connection is closed, and Discord drops its status with it. With the Playing style the main status is
// already a Playing one and is sent in the stream form instead, as before.

// showTwin puts the stream status built from info on the second connection, opening it first when needed. Called
// with e.mu released, from the loop only. A failure is logged once and tried again after a while (tick: twinDown).
func (e *Engine) showTwin(r *run, info *core.Info, cfg *core.Config, art *artwork.Artwork, at time.Time) {
	c := *cfg
	c.StreamTitle = "always"
	b := buildActivity(info, &c, art, &e.folders, at)
	if b.music || b.activity == nil {
		e.dropTwin()
		return
	}
	a := core.WithoutLinks(b.activity)
	a.Buttons = nil

	e.mu.Lock()
	client := e.twin
	e.mu.Unlock()
	if client == nil {
		if e.opts.NewDiscord != nil {
			client = e.opts.NewDiscord()
		} else {
			client = discord.New(e.opts.DiscordPaths)
		}
		if err := client.Login(c.ClientID, 0); err != nil {
			client.Close()
			e.twinFailed("Could not open the second connection to Discord for the stream's name (" + err.Error() + ").")
			return
		}
		e.mu.Lock()
		if e.cur != r || e.twin != nil {
			e.mu.Unlock()
			client.Close()
			return
		}
		e.twin = client
		e.mu.Unlock()
	}
	if err := client.SetActivity(a); err != nil {
		e.dropTwin() // a fresh connection at the next update
		e.twinFailed("Discord did not take the stream's name (" + err.Error() + ").")
		return
	}
	e.mu.Lock()
	e.twinDown = false
	e.mu.Unlock()
}

// twinFailed logs a stream status that did not go up, once until one does, and has it tried again in a while
// without sending the main status again.
func (e *Engine) twinFailed(msg string) {
	e.mu.Lock()
	warn := !e.twinDown
	e.twinDown = true
	e.twinAgain = time.Now().Add(e.retryAfter)
	e.mu.Unlock()
	if warn {
		e.log("WARN", msg+" Trying again shortly.")
	}
}

// dropTwin takes the stream status down by closing its connection. Safe with e.mu released, from anywhere.
func (e *Engine) dropTwin() {
	e.mu.Lock()
	c := e.twin
	e.twin = nil
	e.mu.Unlock()
	if c == nil {
		return
	}
	// Closing the connection drops its status; the clear before it is for a Discord that would keep it a moment
	// longer. Neither may hold the caller up (see stop).
	done := make(chan struct{})
	go func() { _ = c.ClearActivity(); c.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(1500 * time.Millisecond):
	}
}

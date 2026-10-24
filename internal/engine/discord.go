package engine

import (
	"errors"
	"fmt"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/discord"
)

func (e *Engine) connectDiscord(r *run) {
	e.mu.Lock()
	if e.ready || e.connecting || e.cur != r {
		e.mu.Unlock()
		return
	}
	e.connecting = true
	cfg := e.cfg
	e.mu.Unlock()

	Go(e.log, "The Discord connection", func() {
		var client *discord.Client
		if e.opts.NewDiscord != nil {
			client = e.opts.NewDiscord()
		} else {
			client = discord.New(e.opts.DiscordPaths)
		}
		err := client.Login(cfg.ClientID, 0)

		e.mu.Lock()
		// A run that ended has had its flag reset by stop; a newer run may be connecting now, so only the current run's
		// attempt clears it.
		if err != nil {
			if e.cur == r {
				e.connecting = false
				e.discRefused = errors.Is(err, discord.ErrRefused)
			}
			warn := !e.warnedDisc && e.cur == r
			if warn {
				e.warnedDisc = true
			}
			e.mu.Unlock()
			client.Close()
			if warn {
				e.log("WARN", fmt.Sprintf("Could not reach Discord (is the desktop app running?). Retrying in the background. [%s]", err.Error()))
			}
			return
		}
		if e.cur != r {
			e.mu.Unlock()
			client.Close()
			return
		}
		e.rpc = client
		e.ready = true
		e.connecting = false
		e.warnedDisc = false
		e.discRefused = false
		e.prev = nil
		e.mu.Unlock()
		e.log("INFO", "Connected to Discord.")
		e.Wake() // the status goes up now, not at the next check

		Go(e.log, "The Discord connection watch", func() {
			select {
			case <-client.Done():
			case <-r.ctx.Done():
				return
			}
			e.mu.Lock()
			if e.rpc != client {
				e.mu.Unlock()
				return
			}
			e.ready, e.rpc, e.shown, e.prev, e.preview = false, nil, false, nil, nil
			e.mu.Unlock()
			e.log("WARN", "Lost connection to Discord. Will reconnect when needed.")
		})
	})
}

// sendActivity shows a on Discord and returns what was sent. Only a refusal by Discord changes the mode: one that
// refuses the clickable title and cover gets them without links from then on; a refused button is left out of that
// update; one that refuses the Watching activity gets the basic one. Any other error (a timeout, a closed
// connection) changes nothing; the next tick retries.
func (e *Engine) sendActivity(rpc *discord.Client, cfg *core.Config, a *core.Activity) (*core.Activity, error) {
	e.mu.Lock()
	basic, noLinks := e.basicMode, e.noLinks
	e.mu.Unlock()
	if !basic {
		if noLinks && core.HasLinks(a) {
			a = core.WithoutLinks(a)
		}
		err := rpc.SetActivity(a)
		if err == nil {
			return a, nil
		}
		var refused *discord.Error
		if !errors.As(err, &refused) {
			return a, err
		}
		if core.HasLinks(a) {
			e.log("WARN", fmt.Sprintf("Discord rejected the clickable title and cover (%s). Sending them without links.", err.Error()))
			plain := core.WithoutLinks(a)
			err = rpc.SetActivity(plain)
			if err == nil {
				e.mu.Lock()
				e.noLinks = true
				e.mu.Unlock()
				return plain, nil
			}
			a = plain
			if !errors.As(err, &refused) {
				return a, err
			}
		}
		if len(a.Buttons) > 0 {
			// a button Discord does not like (an odd catalog address) must not cost the Watching activity for good
			bare := *a
			bare.Buttons = nil
			if err = rpc.SetActivity(&bare); err == nil {
				e.log("WARN", "Discord rejected the button; sent this title without it.")
				return &bare, nil
			}
			a = &bare
			if !errors.As(err, &refused) {
				return a, err
			}
		}
		e.log("WARN", fmt.Sprintf("Discord rejected the Watching activity (%s). Falling back to basic mode.", err.Error()))
		e.mu.Lock()
		e.basicMode = true
		e.mu.Unlock()
	}
	l := core.LegacyActivity(a, cfg)
	return l, rpc.SetActivity(l)
}

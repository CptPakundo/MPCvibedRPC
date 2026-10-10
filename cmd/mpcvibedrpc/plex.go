package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/plex"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// plexKeys are the settings "Sign in to Plex" writes (plexClient, this install's identifier, outlives a sign-out).
var plexKeys = []string{"plexToken", "plexAccount", "plexUser", "plexServer", "plexServerName", "plexClient"}

// plexAccount drives "Sign in to Plex": a code the user approves on plex.tv in their browser, after which the
// account's token, name and server are saved with the settings and the engine follows that server.
type plexAccount struct {
	st      *store.Store
	eng     *engine.Engine
	log     func(level, msg string)
	tv      string            // tests: replaces plex.tv
	openURL func(string) bool // tests: replaces the browser
	every   time.Duration     // how often the approval is checked (0 = every two seconds)

	mu      sync.Mutex
	waiting bool   // a sign-in waits for the user's approval
	url     string // where to approve it
	problem string // why the last sign-in did not work
	cancel  context.CancelFunc
}

func (p *plexAccount) client(clientID string) *plex.Client {
	return &plex.Client{ClientID: clientID, Version: version, TVBase: p.tv, ResourcesBase: p.tv}
}

// state is what the window shows. The token itself never leaves the program.
func (p *plexAccount) state() map[string]any {
	cfg := p.st.Config()
	p.mu.Lock()
	defer p.mu.Unlock()
	return map[string]any{"signedIn": cfg.PlexToken != "", "user": cfg.PlexUser, "server": cfg.PlexServer, "serverName": cfg.PlexServerName,
		"waiting": p.waiting, "url": p.url, "problem": p.problem}
}

func (p *plexAccount) apply() { p.eng.ApplySettings(p.st.Config()) }

// signIn asks plex.tv for a code, opens the page that approves it, and waits for the approval in the background.
func (p *plexAccount) signIn() (map[string]any, error) {
	cfg := p.st.Config()
	id := cfg.PlexClient
	if id == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		id = hex.EncodeToString(b)
		if err := p.st.SetHidden([]string{"plexClient"}, map[string]any{"plexClient": id}); err != nil {
			return nil, err
		}
	}
	c := p.client(id)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	pin, err := c.NewPin(ctx)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("Could not reach plex.tv to sign in (%v).", err)
	}
	u := c.AuthURL(pin)
	wctx, wcancel := context.WithTimeout(context.Background(), 30*time.Minute) // plex.tv's codes last 30 minutes
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.waiting, p.url, p.problem = wcancel, true, u, ""
	p.mu.Unlock()
	go p.wait(wctx, c, pin)
	open := p.openURL
	if open == nil {
		open = winsys.OpenURL
	}
	open(u)
	p.log("INFO", "Waiting for the Plex sign-in to be approved in the browser.")
	return p.state(), nil
}

func (p *plexAccount) finish(ctx context.Context, problem string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ctx.Err() == context.Canceled && problem == "" {
		return // cancelled, or replaced by a newer sign-in that now owns the state
	}
	p.waiting, p.url, p.problem = false, "", problem
	if problem != "" {
		p.log("WARN", problem)
	}
}

// wait checks the code every two seconds until it is approved, expires or the sign-in is cancelled.
func (p *plexAccount) wait(ctx context.Context, c *plex.Client, pin *plex.Pin) {
	every := p.every
	if every <= 0 {
		every = 2 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				p.finish(ctx, "The Plex sign-in was not approved in time. Press Sign in to Plex to try again.")
			}
			return
		case <-t.C:
		}
		token, err := c.CheckPin(ctx, pin)
		if errors.Is(err, plex.ErrPinExpired) {
			p.finish(ctx, "The Plex sign-in code expired. Press Sign in to Plex to try again.")
			return
		}
		if err != nil || token == "" {
			continue // not approved yet (or plex.tv did not answer this time)
		}
		acc, err := c.Account(ctx, token)
		if err != nil {
			p.finish(ctx, fmt.Sprintf("Signed in, but plex.tv did not say who you are (%v). Try again.", err))
			return
		}
		servers, err := c.Servers(ctx, token)
		if err != nil {
			p.finish(ctx, fmt.Sprintf("Signed in, but plex.tv did not list your Plex servers (%v). Try again.", err))
			return
		}
		cur := p.st.Config()
		pick := plex.Server{}
		for _, s := range servers {
			if s.ID == cur.PlexServer { // signing in again keeps the server chosen before
				pick = s
				break
			}
		}
		if pick.ID == "" && len(servers) > 0 {
			pick = servers[0] // the account's own servers come first
		}
		p.mu.Lock()
		stale := ctx.Err() != nil
		p.mu.Unlock()
		if stale {
			return
		}
		err = p.st.SetHidden(plexKeys, map[string]any{"plexToken": token, "plexAccount": acc.ID, "plexUser": acc.Name,
			"plexServer": pick.ID, "plexServerName": pick.Name, "plexClient": c.ClientID})
		if err != nil {
			p.finish(ctx, "Could not save the Plex sign-in: "+err.Error())
			return
		}
		p.apply()
		p.log("INFO", "Signed in to Plex.")
		problem := ""
		if pick.ID == "" {
			problem = "Signed in, but this Plex account has no server to follow."
		}
		p.finish(ctx, problem)
		p.mu.Lock()
		if p.cancel != nil {
			p.cancel() // ends the 30-minute limit of this sign-in
			p.cancel = nil
		}
		p.mu.Unlock()
		return
	}
}

// stopWaiting cancels a sign-in that waits for approval.
func (p *plexAccount) stopWaiting() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.waiting, p.url = false, ""
	p.mu.Unlock()
}

// servers lists the account's servers for the window's choice.
func (p *plexAccount) servers() ([]map[string]any, error) {
	cfg := p.st.Config()
	if cfg.PlexToken == "" {
		return []map[string]any{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	list, err := p.client(cfg.PlexClient).Servers(ctx, cfg.PlexToken)
	if errors.Is(err, plex.ErrToken) {
		return nil, errors.New("Plex refused the sign-in. Sign out and sign in again.")
	} else if err != nil {
		return nil, fmt.Errorf("Could not ask plex.tv for your Plex servers (%v).", err)
	}
	out := []map[string]any{}
	for _, s := range list {
		out = append(out, map[string]any{"id": s.ID, "name": s.Name, "owned": s.Owned})
	}
	return out, nil
}

// choose follows another of the account's servers.
func (p *plexAccount) choose(id string) error {
	list, err := p.servers()
	if err != nil {
		return err
	}
	for _, s := range list {
		if s["id"] == id {
			if err := p.st.SetHidden([]string{"plexServer", "plexServerName"}, map[string]any{"plexServer": id, "plexServerName": s["name"]}); err != nil {
				return err
			}
			p.apply()
			p.log("INFO", "Plex server changed.")
			return nil
		}
	}
	return errors.New("That server is not in your Plex account.")
}

// signOut forgets the sign-in (the token is removed from the settings file).
func (p *plexAccount) signOut() error {
	p.stopWaiting()
	p.mu.Lock()
	p.problem = ""
	p.mu.Unlock()
	keys := plexKeys[:len(plexKeys)-1] // plexClient stays: the next sign-in is the same device on plex.tv
	if err := p.st.SetHidden(keys, map[string]any{}); err != nil {
		return err
	}
	p.apply()
	p.log("INFO", "Signed out of Plex.")
	return nil
}

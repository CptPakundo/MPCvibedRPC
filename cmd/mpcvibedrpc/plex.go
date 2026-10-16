package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/plex"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
)

// plexSignIn are the settings a Plex sign-in writes and a sign-out forgets; plexClient, this install's identifier on
// plex.tv, is kept beside them.
var plexSignIn = []string{"plexToken", "plexAccount", "plexUser", "plexServer", "plexServerName"}

// plexAccount drives "Sign in to Plex": a code the user approves on plex.tv in their browser, after which the
// account's token, name and server are saved with the settings and the engine follows that server.
type plexAccount struct {
	approval
	st      *store.Store
	eng     *engine.Engine
	log     func(level, msg string)
	tv      string            // tests: replaces plex.tv
	openURL func(string) bool // tests: replaces the browser
	every   time.Duration     // how often the approval is checked (0 = every two seconds)
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
		"waiting": p.waiting, "url": p.shown, "problem": p.problem}
}

// signIn asks plex.tv for a code, opens the page that approves it, and waits for the approval in the background.
func (p *plexAccount) signIn() (map[string]any, error) {
	id, err := installID(p.st, "plexClient", p.st.Config().PlexClient)
	if err != nil {
		return nil, err
	}
	c := p.client(id)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	pin, err := c.NewPin(ctx)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("Could not reach plex.tv to sign in (%v).", err)
	}
	u := c.AuthURL(pin)
	wctx, wcancel := p.begin(30*time.Minute, u) // plex.tv's codes last 30 minutes
	go p.poll(wctx, wcancel, p.every, p.log, "The Plex sign-in was not approved in time. Press Sign in to Plex to try again.",
		func() (bool, string) { return p.check(wctx, c, pin) })
	openPage(p.openURL, u)
	p.log("INFO", "Waiting for the Plex sign-in to be approved in the browser.")
	return p.state(), nil
}

// check asks plex.tv whether the code was approved; once it is, the account and its server are saved.
func (p *plexAccount) check(ctx context.Context, c *plex.Client, pin *plex.Pin) (done bool, problem string) {
	token, err := c.CheckPin(ctx, pin)
	if errors.Is(err, plex.ErrPinExpired) {
		return true, "The Plex sign-in code expired. Press Sign in to Plex to try again."
	}
	if err != nil || token == "" {
		return false, "" // not approved yet (or plex.tv did not answer this time)
	}
	acc, err := c.Account(ctx, token)
	if err != nil {
		return true, fmt.Sprintf("Signed in, but plex.tv did not say who you are (%v). Try again.", err)
	}
	servers, err := c.Servers(ctx, token)
	if err != nil {
		return true, fmt.Sprintf("Signed in, but plex.tv did not list your Plex servers (%v). Try again.", err)
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
	saved, err := p.save(ctx, func() error {
		return p.st.SetHidden(append(plexSignIn, "plexClient"), map[string]any{"plexToken": token, "plexAccount": acc.ID,
			"plexUser": acc.Name, "plexServer": pick.ID, "plexServerName": pick.Name, "plexClient": c.ClientID})
	})
	if !saved {
		return true, ""
	}
	if err != nil {
		return true, "Could not save the Plex sign-in: " + err.Error()
	}
	applySettings(p.st, p.eng)
	p.log("INFO", "Signed in to Plex.")
	if pick.ID == "" {
		return true, "Signed in, but this Plex account has no server to follow."
	}
	return true, ""
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
			applySettings(p.st, p.eng)
			p.log("INFO", "Plex server changed.")
			return nil
		}
	}
	return errors.New("That server is not in your Plex account.")
}

// signOut forgets the sign-in (the token is removed from the settings file).
func (p *plexAccount) signOut() error {
	p.stopWaiting()
	p.clearProblem()
	if err := p.st.SetHidden(plexSignIn, map[string]any{}); err != nil {
		return err
	}
	applySettings(p.st, p.eng)
	p.log("INFO", "Signed out of Plex.")
	return nil
}

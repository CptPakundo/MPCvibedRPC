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
var plexSignIn = []string{"plexToken", "plexAccount", "plexUser", "plexServer", "plexServerName", "plexKey"}

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
	var key *plex.DeviceKey // with plexJwt: a key of this install's own, and 7-day tokens it renews (keepFresh)
	if p.st.Config().PlexJWT {
		if key, err = plex.NewDeviceKey(); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	var pin *plex.Pin
	if key != nil {
		pin, err = c.NewKeyPin(ctx, key)
	} else {
		pin, err = c.NewPin(ctx)
	}
	cancel()
	if err != nil {
		return nil, fmt.Errorf("Could not reach plex.tv to sign in (%v).", err)
	}
	u := c.AuthURL(pin)
	wctx, wcancel := p.begin(30*time.Minute, u) // plex.tv's codes last 30 minutes
	engine.Go(p.log, "The Plex sign-in", func() {
		p.poll(wctx, wcancel, p.every, p.log, "The Plex sign-in was not approved in time. Press Sign in to Plex to try again.",
			func() (bool, string) { return p.check(wctx, c, pin, key) })
	})
	openPage(p.openURL, u)
	p.log("INFO", "Waiting for the Plex sign-in to be approved in the browser.")
	return p.state(), nil
}

// check asks plex.tv whether the code was approved; once it is, the account and its server are saved.
func (p *plexAccount) check(ctx context.Context, c *plex.Client, pin *plex.Pin, key *plex.DeviceKey) (done bool, problem string) {
	var token string
	var err error
	if key != nil {
		token, err = c.CheckKeyPin(ctx, pin, key)
	} else {
		token, err = c.CheckPin(ctx, pin)
	}
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
	keyText := ""
	if key != nil {
		keyText = key.String()
	}
	saved, err := p.save(ctx, func() error {
		return p.st.SetHidden(append(plexSignIn, "plexClient"), map[string]any{"plexToken": token, "plexAccount": acc.ID,
			"plexUser": acc.Name, "plexServer": pick.ID, "plexServerName": pick.Name, "plexClient": c.ClientID, "plexKey": keyText})
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
			// under the saving lock, so a sign-out at the same moment cannot be followed by this write
			p.saving.Lock()
			err := errors.New("You are signed out of Plex.")
			if p.st.Config().PlexToken != "" {
				err = p.st.SetHidden([]string{"plexServer", "plexServerName"}, map[string]any{"plexServer": id, "plexServerName": s["name"]})
			}
			p.saving.Unlock()
			if err != nil {
				return err
			}
			applySettings(p.st, p.eng)
			p.log("INFO", "Plex server changed.")
			return nil
		}
	}
	return errors.New("That server is not in your Plex account.")
}

// renewBefore is how long before a 7-day token runs out keepFresh renews it.
const renewBefore = 2 * 24 * time.Hour

// keepFresh renews a 7-day token (a sign-in made with plexJwt) before it runs out, checking every `every` (an hour
// by default), until ctx ends. An older long-lived token, or none, is left alone.
func (p *plexAccount) keepFresh(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = time.Hour
	}
	for {
		p.renew(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// renew gets a new token when the saved one runs out within renewBefore (or already has).
func (p *plexAccount) renew(ctx context.Context) {
	cfg := p.st.Config()
	if cfg.PlexToken == "" || cfg.PlexKey == "" {
		return
	}
	exp := plex.TokenExpiry(cfg.PlexToken)
	if exp.IsZero() || time.Until(exp) > renewBefore {
		return
	}
	key, err := plex.ParseDeviceKey(cfg.PlexKey)
	if err != nil {
		p.log("WARN", "The saved Plex key cannot be read; sign in to Plex again.")
		return
	}
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	token, err := p.client(cfg.PlexClient).Refresh(rctx, key)
	if err != nil {
		p.log("WARN", "Could not renew the Plex sign-in ("+err.Error()+"); trying again later.")
		return
	}
	// The check and the write happen under the saving lock, which a sign-out holds too: a sign-out between them
	// would otherwise leave the new token behind with no account.
	p.saving.Lock()
	same := p.st.Config().PlexToken == cfg.PlexToken // else signed out or in again meanwhile
	if same {
		err = p.st.SetHidden([]string{"plexToken"}, map[string]any{"plexToken": token})
	}
	p.saving.Unlock()
	if !same {
		return
	}
	if err != nil {
		p.log("WARN", "Could not save the renewed Plex sign-in: "+err.Error())
		return
	}
	applySettings(p.st, p.eng)
	p.log("INFO", "Plex sign-in renewed.")
}

// signOut forgets the sign-in (the token is removed from the settings file).
func (p *plexAccount) signOut() error {
	if err := p.forget(func() error { return p.st.SetHidden(plexSignIn, map[string]any{}) }); err != nil {
		return err
	}
	applySettings(p.st, p.eng)
	p.log("INFO", "Signed out of Plex.")
	return nil
}

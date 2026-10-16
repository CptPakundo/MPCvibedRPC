package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/jellyfin"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
)

// jellyfinSignIn are the settings the Jellyfin / Emby sign-in writes and a sign-out forgets; jellyfinDevice, this
// install's id towards the server, is kept beside them.
var jellyfinSignIn = []string{"jellyfinAddress", "jellyfinToken", "jellyfinUser", "jellyfinUserName", "jellyfinServerName", "jellyfinProduct"}

// jellyfinAccount drives the Jellyfin / Emby sign-in: the user types their server's address, then approves a Quick
// Connect code in a Jellyfin app they are signed in to, or (Emby, or Quick Connect off) types their user name and
// password. Only the server's token is kept, never the password. While a code waits, approval.shown is the code.
type jellyfinAccount struct {
	approval
	st    *store.Store
	eng   *engine.Engine
	log   func(level, msg string)
	every time.Duration // how often a Quick Connect code is checked (0 = every two seconds)

	// guarded by approval.mu
	password bool // the server wants a user name and password (no Quick Connect)
	found    *jellyfin.Server
	address  string // the address the sign-in is for (as found: Emby may need /emby)
}

// state is what the window shows. The token never leaves the program.
func (j *jellyfinAccount) state() map[string]any {
	cfg := j.st.Config()
	j.mu.Lock()
	defer j.mu.Unlock()
	s := map[string]any{"signedIn": cfg.JellyfinToken != "", "user": cfg.JellyfinUserName, "server": cfg.JellyfinServerName,
		"product": cfg.JellyfinProduct, "address": cfg.JellyfinAddress, "waiting": j.waiting, "code": j.shown,
		"password": j.password, "problem": j.problem}
	if j.found != nil && cfg.JellyfinToken == "" {
		s["server"], s["product"], s["address"] = j.found.Name, j.found.Product, j.address
	}
	return s
}

func (j *jellyfinAccount) client(base string) (*jellyfin.Client, error) {
	id, err := installID(j.st, "jellyfinDevice", j.st.Config().JellyfinDevice)
	if err != nil {
		return nil, err
	}
	return &jellyfin.Client{Base: base, DeviceID: id, Version: version}, nil
}

// signIn finds the server at the address and starts a Quick Connect sign-in; a server without Quick Connect asks for
// a user name and password instead (the state says so).
func (j *jellyfinAccount) signIn(address string) (map[string]any, error) {
	j.stopWaiting()
	base, err := jellyfin.Address(address)
	if err != nil {
		return nil, err
	}
	c, err := j.client(base)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv, err := c.Find(ctx)
	if err != nil {
		if errors.Is(err, jellyfin.ErrNotServer) {
			return nil, errors.New("That address answers, but not as a Jellyfin or Emby server. Check the address and port (Jellyfin and Emby use 8096 unless changed).")
		}
		return nil, fmt.Errorf("Could not reach a server at that address (%s). Check the address, and that the server is running.", jellyfin.Reason(err))
	}
	j.mu.Lock()
	j.found, j.address, j.problem, j.password = srv, c.Base, "", srv.Product == "Emby"
	j.mu.Unlock()
	if srv.Product == "Emby" {
		return j.state(), nil
	}
	p, err := c.StartQuickConnect(ctx)
	if err != nil {
		j.mu.Lock()
		j.password = true // Quick Connect is off on this server
		j.mu.Unlock()
		if !errors.Is(err, jellyfin.ErrNoQuickConnect) {
			j.log("WARN", "Jellyfin's Quick Connect did not answer ("+jellyfin.Reason(err)+"); asking for a password instead.")
		}
		return j.state(), nil
	}
	wctx, wcancel := j.begin(10*time.Minute, p.Code) // Jellyfin forgets a code after 10 minutes
	incomplete := 0
	go j.poll(wctx, wcancel, j.every, j.log, "The Quick Connect code was not approved in time. Press Sign in to try again.",
		func() (bool, string) { return j.check(wctx, c, p, &incomplete) })
	j.log("INFO", "Waiting for the Jellyfin Quick Connect code to be approved.")
	return j.state(), nil
}

// check asks the server whether the Quick Connect code was approved; once it is, the sign-in is saved.
func (j *jellyfinAccount) check(ctx context.Context, c *jellyfin.Client, p *jellyfin.Pending, incomplete *int) (done bool, problem string) {
	a, err := c.CheckQuickConnect(ctx, p)
	if errors.Is(err, jellyfin.ErrExpired) {
		return true, "The Quick Connect code expired. Press Sign in to try again."
	}
	if errors.Is(err, jellyfin.ErrIncomplete) {
		if *incomplete++; *incomplete >= 3 {
			return true, "Sign-in failed: " + err.Error() + ". Press Sign in to try again."
		}
		return false, ""
	}
	if err != nil || a == nil {
		return false, "" // not approved yet (or the server did not answer this time)
	}
	saved, err := j.save(ctx, func() error {
		j.mu.Lock()
		srv := j.found
		j.mu.Unlock()
		return j.keep(c, srv, a)
	})
	if saved && err != nil {
		return true, "Could not save the sign-in: " + err.Error()
	}
	return true, ""
}

// signInPassword signs in with a user name and password at the address found by signIn (or a new one).
func (j *jellyfinAccount) signInPassword(address, user, password string) (map[string]any, error) {
	if user == "" {
		return nil, errors.New("Enter your user name.")
	}
	// Only a Quick Connect that is waiting ends here: stopWaiting would also forget the server found and the password
	// form, and after a wrong password the box would fall back to the address form.
	j.mu.Lock()
	j.end()
	j.mu.Unlock()
	base, err := jellyfin.Address(address)
	if err != nil {
		return nil, err
	}
	c, err := j.client(base)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv, err := c.Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("Could not reach a server at that address (%s).", jellyfin.Reason(err))
	}
	a, err := c.SignIn(ctx, user, password)
	if errors.Is(err, jellyfin.ErrLogin) {
		return nil, errors.New("The server refused that user name or password.")
	} else if err != nil {
		return nil, fmt.Errorf("Could not sign in (%s).", jellyfin.Reason(err))
	}
	if err := j.keep(c, srv, a); err != nil {
		return nil, errors.New("Could not save the sign-in: " + err.Error())
	}
	j.clearProblem()
	return j.state(), nil
}

// keep saves the sign-in and starts following the server.
func (j *jellyfinAccount) keep(c *jellyfin.Client, srv *jellyfin.Server, a *jellyfin.Auth) error {
	err := j.st.SetHidden(append(jellyfinSignIn, "jellyfinDevice"), map[string]any{"jellyfinAddress": c.Base, "jellyfinToken": a.Token,
		"jellyfinUser": a.UserID, "jellyfinUserName": a.UserName, "jellyfinServerName": srv.Name, "jellyfinProduct": srv.Product,
		"jellyfinDevice": c.DeviceID})
	if err != nil {
		return err
	}
	j.mu.Lock()
	j.found, j.address, j.password = nil, "", false
	j.mu.Unlock()
	applySettings(j.st, j.eng)
	j.log("INFO", "Signed in to "+srv.Product+".")
	return nil
}

// stopWaiting cancels a sign-in that waits for approval, and forgets a server found for a sign-in.
func (j *jellyfinAccount) stopWaiting() {
	j.saving.Lock()
	defer j.saving.Unlock()
	j.mu.Lock()
	j.end()
	j.password, j.found, j.address = false, nil, ""
	j.mu.Unlock()
}

// revoke ends the saved sign-in's session on the server, when there is one and the server answers (signing out, and
// Restore defaults, which forgets the sign-in).
func (j *jellyfinAccount) revoke() {
	cfg := j.st.Config()
	if cfg.JellyfinToken == "" || cfg.JellyfinAddress == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := &jellyfin.Client{Base: cfg.JellyfinAddress, DeviceID: cfg.JellyfinDevice, Version: version}
	if err := c.SignOut(ctx, cfg.JellyfinToken); err != nil && !errors.Is(err, jellyfin.ErrToken) {
		j.log("WARN", "The server did not confirm the sign-out ("+jellyfin.Reason(err)+"); the sign-in is forgotten here anyway.")
	}
}

// signOut ends the session on the server (when it answers) and forgets the sign-in.
func (j *jellyfinAccount) signOut() error {
	j.stopWaiting()
	j.clearProblem()
	product := j.st.Config().JellyfinProduct
	j.revoke()
	if err := j.st.SetHidden(jellyfinSignIn, map[string]any{}); err != nil {
		return err
	}
	applySettings(j.st, j.eng)
	if product == "" {
		product = "Jellyfin"
	}
	j.log("INFO", "Signed out of "+product+".")
	return nil
}

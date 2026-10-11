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
	"github.com/CptPakundo/MPCvibedRPC/internal/jellyfin"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
)

// jellyfinKeys are the settings the Jellyfin / Emby sign-in writes (jellyfinDevice, this install's id towards the
// server, outlives a sign-out).
var jellyfinKeys = []string{"jellyfinAddress", "jellyfinToken", "jellyfinUser", "jellyfinUserName", "jellyfinServerName", "jellyfinProduct", "jellyfinDevice"}

// jellyfinAccount drives the Jellyfin / Emby sign-in: the user types their server's address, then approves a Quick
// Connect code in a Jellyfin app they are signed in to, or (Emby, or Quick Connect off) types their user name and
// password. Only the server's token is kept, never the password.
type jellyfinAccount struct {
	st    *store.Store
	eng   *engine.Engine
	log   func(level, msg string)
	every time.Duration // how often a Quick Connect code is checked (0 = every two seconds)

	mu       sync.Mutex
	waiting  bool   // a Quick Connect code waits for the user's approval
	code     string // that code
	password bool   // the server wants a user name and password (no Quick Connect)
	found    *jellyfin.Server
	address  string // the address the sign-in is for (as found: Emby may need /emby)
	problem  string // why the last sign-in did not work
	cancel   context.CancelFunc
}

// state is what the window shows. The token never leaves the program.
func (j *jellyfinAccount) state() map[string]any {
	cfg := j.st.Config()
	j.mu.Lock()
	defer j.mu.Unlock()
	s := map[string]any{"signedIn": cfg.JellyfinToken != "", "user": cfg.JellyfinUserName, "server": cfg.JellyfinServerName,
		"product": cfg.JellyfinProduct, "address": cfg.JellyfinAddress, "waiting": j.waiting, "code": j.code,
		"password": j.password, "problem": j.problem}
	if j.found != nil && cfg.JellyfinToken == "" {
		s["server"], s["product"], s["address"] = j.found.Name, j.found.Product, j.address
	}
	return s
}

// device is this install's id towards the server, made once.
func (j *jellyfinAccount) device() (string, error) {
	if id := j.st.Config().JellyfinDevice; id != "" {
		return id, nil
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	return id, j.st.SetHidden([]string{"jellyfinDevice"}, map[string]any{"jellyfinDevice": id})
}

func (j *jellyfinAccount) client(base string) (*jellyfin.Client, error) {
	id, err := j.device()
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
	j.found, j.address, j.problem, j.password = srv, c.Base, "", false
	j.mu.Unlock()
	if srv.Product == "Emby" {
		j.mu.Lock()
		j.password = true
		j.mu.Unlock()
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
	wctx, wcancel := context.WithTimeout(context.Background(), 10*time.Minute) // Jellyfin forgets a code after 10 minutes
	j.mu.Lock()
	j.cancel, j.waiting, j.code = wcancel, true, p.Code
	j.mu.Unlock()
	go j.wait(wctx, c, p)
	j.log("INFO", "Waiting for the Jellyfin Quick Connect code to be approved.")
	return j.state(), nil
}

func (j *jellyfinAccount) finish(ctx context.Context, problem string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if ctx.Err() == context.Canceled {
		return // cancelled, or replaced by a newer sign-in that now owns the state
	}
	j.waiting, j.code, j.problem = false, "", problem
	if problem != "" {
		j.log("WARN", problem)
	}
}

// wait checks the Quick Connect code until it is approved, expires or the sign-in is cancelled.
func (j *jellyfinAccount) wait(ctx context.Context, c *jellyfin.Client, p *jellyfin.Pending) {
	every := j.every
	if every <= 0 {
		every = 2 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				j.finish(ctx, "The Quick Connect code was not approved in time. Press Sign in to try again.")
			}
			return
		case <-t.C:
		}
		a, err := c.CheckQuickConnect(ctx, p)
		if errors.Is(err, jellyfin.ErrExpired) {
			j.finish(ctx, "The Quick Connect code expired. Press Sign in to try again.")
			return
		}
		if err != nil || a == nil {
			continue // not approved yet (or the server did not answer this time)
		}
		j.mu.Lock()
		stale := ctx.Err() != nil
		srv := j.found
		j.mu.Unlock()
		if stale {
			return
		}
		if err := j.save(c, srv, a); err != nil {
			j.finish(ctx, "Could not save the sign-in: "+err.Error())
			return
		}
		j.finish(ctx, "")
		j.mu.Lock()
		if j.cancel != nil {
			j.cancel() // ends the time limit of this sign-in
			j.cancel = nil
		}
		j.mu.Unlock()
		return
	}
}

// signInPassword signs in with a user name and password at the address found by signIn (or a new one).
func (j *jellyfinAccount) signInPassword(address, user, password string) (map[string]any, error) {
	if user == "" {
		return nil, errors.New("Enter your user name.")
	}
	// Only a Quick Connect that is waiting ends here: stopWaiting would also forget the server found and the password
	// form, and after a wrong password the box would fall back to the address form.
	j.mu.Lock()
	if j.cancel != nil {
		j.cancel()
		j.cancel = nil
	}
	j.waiting, j.code = false, ""
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
	if err := j.save(c, srv, a); err != nil {
		return nil, errors.New("Could not save the sign-in: " + err.Error())
	}
	j.mu.Lock()
	j.problem = ""
	j.mu.Unlock()
	return j.state(), nil
}

// save keeps the sign-in and starts following the server.
func (j *jellyfinAccount) save(c *jellyfin.Client, srv *jellyfin.Server, a *jellyfin.Auth) error {
	err := j.st.SetHidden(jellyfinKeys, map[string]any{"jellyfinAddress": c.Base, "jellyfinToken": a.Token, "jellyfinUser": a.UserID,
		"jellyfinUserName": a.UserName, "jellyfinServerName": srv.Name, "jellyfinProduct": srv.Product, "jellyfinDevice": c.DeviceID})
	if err != nil {
		return err
	}
	j.mu.Lock()
	j.found, j.address, j.password = nil, "", false
	j.mu.Unlock()
	j.eng.ApplySettings(j.st.Config())
	j.log("INFO", "Signed in to "+srv.Product+".")
	return nil
}

// stopWaiting cancels a sign-in that waits for approval, and forgets a server found for a sign-in.
func (j *jellyfinAccount) stopWaiting() {
	j.mu.Lock()
	if j.cancel != nil {
		j.cancel()
		j.cancel = nil
	}
	j.waiting, j.code, j.password, j.found, j.address = false, "", false, nil, ""
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
	j.mu.Lock()
	j.problem = ""
	j.mu.Unlock()
	product := j.st.Config().JellyfinProduct
	j.revoke()
	keys := jellyfinKeys[:len(jellyfinKeys)-1] // jellyfinDevice stays: the next sign-in is the same device on the server
	if err := j.st.SetHidden(keys, map[string]any{}); err != nil {
		return err
	}
	j.eng.ApplySettings(j.st.Config())
	if product == "" {
		product = "Jellyfin"
	}
	j.log("INFO", "Signed out of "+product+".")
	return nil
}

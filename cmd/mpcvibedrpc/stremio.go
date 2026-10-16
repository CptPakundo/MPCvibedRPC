package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/stremio"
)

// stremioAccount drives "Sign in to Stremio": a code the user approves on stremio.com in their browser, after which
// the account's key is saved with the settings and the engine follows what the account watches.
type stremioAccount struct {
	approval
	st      *store.Store
	eng     *engine.Engine
	log     func(level, msg string)
	api     string            // tests: replaces Stremio's services
	openURL func(string) bool // tests: replaces the browser
	every   time.Duration     // how often the approval is checked (0 = every two seconds)
}

func (s *stremioAccount) client() *stremio.Client {
	return &stremio.Client{APIBase: s.api, LinkBase: s.api}
}

// state is what the window shows. The key itself never leaves the program.
func (s *stremioAccount) state() map[string]any {
	cfg := s.st.Config()
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"signedIn": cfg.StremioKey != "", "waiting": s.waiting, "url": s.shown, "problem": s.problem}
}

// signIn asks Stremio for a code, opens the page that approves it, and waits for the approval in the background.
func (s *stremioAccount) signIn() (map[string]any, error) {
	c := s.client()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	l, err := c.NewLink(ctx)
	cancel()
	if errors.Is(err, stremio.ErrBadLink) {
		return nil, errors.New("Stremio handed out a sign-in page that is not on stremio.com, so it was not opened.")
	} else if err != nil {
		return nil, fmt.Errorf("Could not reach Stremio to sign in (%v).", err)
	}
	wctx, wcancel := s.begin(15*time.Minute, l.URL)
	go s.poll(wctx, wcancel, s.every, s.log, "The Stremio sign-in was not approved in time. Press Sign in to Stremio to try again.",
		func() (bool, string) { return s.check(wctx, c, l) })
	openPage(s.openURL, l.URL)
	s.log("INFO", "Waiting for the Stremio sign-in to be approved in the browser.")
	return s.state(), nil
}

// check asks Stremio whether the code was approved; once it is, the key is saved.
func (s *stremioAccount) check(ctx context.Context, c *stremio.Client, l *stremio.Link) (done bool, problem string) {
	key, err := c.CheckLink(ctx, l)
	if err != nil || key == "" {
		return false, "" // not approved yet (or Stremio did not answer this time)
	}
	saved, err := s.save(ctx, func() error { return s.st.SetHidden([]string{"stremioKey"}, map[string]any{"stremioKey": key}) })
	if !saved {
		return true, ""
	}
	if err != nil {
		return true, "Could not save the Stremio sign-in: " + err.Error()
	}
	applySettings(s.st, s.eng)
	s.log("INFO", "Signed in to Stremio.")
	return true, ""
}

// revoke ends the saved key's session on Stremio's side, when there is one and Stremio answers (signing out, and
// Restore defaults, which forgets the key).
func (s *stremioAccount) revoke() {
	key := s.st.Config().StremioKey
	if key == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.client().SignOut(ctx, key); err != nil && !errors.Is(err, stremio.ErrKey) {
		s.log("WARN", fmt.Sprintf("Stremio did not confirm the sign-out (%v); the key is forgotten here anyway.", err))
	}
}

// signOut ends the session on Stremio's side (when it answers) and forgets the key.
func (s *stremioAccount) signOut() error {
	s.stopWaiting()
	s.clearProblem()
	s.revoke()
	if err := s.st.SetHidden([]string{"stremioKey"}, map[string]any{}); err != nil {
		return err
	}
	applySettings(s.st, s.eng)
	s.log("INFO", "Signed out of Stremio.")
	return nil
}

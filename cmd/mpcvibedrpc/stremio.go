package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/stremio"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// stremioAccount drives "Sign in to Stremio": a code the user approves on stremio.com in their browser, after which
// the account's key is saved with the settings and the engine follows what the account watches.
type stremioAccount struct {
	st      *store.Store
	eng     *engine.Engine
	log     func(level, msg string)
	api     string            // tests: replaces Stremio's services
	openURL func(string) bool // tests: replaces the browser
	every   time.Duration     // how often the approval is checked (0 = every two seconds)

	saving sync.Mutex // held while a finished sign-in is saved, so a sign-out cannot slip in between

	mu      sync.Mutex
	waiting bool   // a sign-in waits for the user's approval
	url     string // where to approve it
	problem string // why the last sign-in did not work
	cancel  context.CancelFunc
}

func (s *stremioAccount) client() *stremio.Client {
	return &stremio.Client{APIBase: s.api, LinkBase: s.api}
}

// state is what the window shows. The key itself never leaves the program.
func (s *stremioAccount) state() map[string]any {
	cfg := s.st.Config()
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"signedIn": cfg.StremioKey != "", "waiting": s.waiting, "url": s.url, "problem": s.problem}
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
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Minute)
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel, s.waiting, s.url, s.problem = wcancel, true, l.URL, ""
	s.mu.Unlock()
	go s.wait(wctx, c, l)
	open := s.openURL
	if open == nil {
		open = winsys.OpenURL
	}
	open(l.URL)
	s.log("INFO", "Waiting for the Stremio sign-in to be approved in the browser.")
	return s.state(), nil
}

func (s *stremioAccount) finish(ctx context.Context, problem string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() == context.Canceled {
		return // cancelled, or replaced by a newer sign-in that now owns the state
	}
	s.waiting, s.url, s.problem = false, "", problem
	if problem != "" {
		s.log("WARN", problem)
	}
}

// wait checks the code every two seconds until it is approved, the time is up or the sign-in is cancelled.
func (s *stremioAccount) wait(ctx context.Context, c *stremio.Client, l *stremio.Link) {
	every := s.every
	if every <= 0 {
		every = 2 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				s.finish(ctx, "The Stremio sign-in was not approved in time. Press Sign in to Stremio to try again.")
			}
			return
		case <-t.C:
		}
		key, err := c.CheckLink(ctx, l)
		if err != nil || key == "" {
			continue // not approved yet (or Stremio did not answer this time)
		}
		s.saving.Lock()
		if ctx.Err() != nil {
			s.saving.Unlock()
			return // cancelled: signed out, or replaced by a newer sign-in
		}
		err = s.st.SetHidden([]string{"stremioKey"}, map[string]any{"stremioKey": key})
		s.saving.Unlock()
		if err != nil {
			s.finish(ctx, "Could not save the Stremio sign-in: "+err.Error())
			return
		}
		s.eng.ApplySettings(s.st.Config())
		s.log("INFO", "Signed in to Stremio.")
		s.finish(ctx, "")
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel() // ends the time limit of this sign-in
			s.cancel = nil
		}
		s.mu.Unlock()
		return
	}
}

// stopWaiting cancels a sign-in that waits for approval.
func (s *stremioAccount) stopWaiting() {
	s.saving.Lock()
	defer s.saving.Unlock()
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.waiting, s.url = false, ""
	s.mu.Unlock()
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
	s.mu.Lock()
	s.problem = ""
	s.mu.Unlock()
	s.revoke()
	if err := s.st.SetHidden([]string{"stremioKey"}, map[string]any{}); err != nil {
		return err
	}
	s.eng.ApplySettings(s.st.Config())
	s.log("INFO", "Signed out of Stremio.")
	return nil
}

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
	"github.com/CptPakundo/MPCvibedRPC/internal/server"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// approval is the part the Plex, Stremio and Jellyfin sign-ins share: a sign-in that waits until the user approves it
// somewhere else (a page in their browser, or a code in a Jellyfin app), checked every few seconds in the background.
type approval struct {
	saving sync.Mutex // held while a finished sign-in is saved, so a sign-out cannot slip in between

	mu      sync.Mutex // guards the fields below (and the account's own state)
	waiting bool       // a sign-in waits for the user's approval
	shown   string     // what the window shows meanwhile: the page to approve it on, or the code
	problem string     // why the last sign-in did not work
	cancel  context.CancelFunc
}

// begin starts waiting for an approval of shown, for at most limit; a sign-in still waiting is replaced.
func (a *approval) begin(limit time.Duration, shown string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.cancel, a.waiting, a.shown, a.problem = cancel, true, shown, ""
	a.mu.Unlock()
	return ctx, cancel
}

// finish ends the wait with problem ("" when it worked), unless the sign-in was cancelled or replaced by a newer one,
// which then owns the state.
func (a *approval) finish(ctx context.Context, problem string, log func(level, msg string)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ctx.Err() == context.Canceled {
		return
	}
	a.waiting, a.shown, a.problem = false, "", problem
	if problem != "" {
		log("WARN", problem)
	}
}

// end stops a waiting sign-in (a.mu held).
func (a *approval) end() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.waiting, a.shown = false, ""
}

// stopWaiting cancels a sign-in that waits for approval.
func (a *approval) stopWaiting() {
	a.saving.Lock()
	defer a.saving.Unlock()
	a.mu.Lock()
	a.end()
	a.mu.Unlock()
}

func (a *approval) clearProblem() {
	a.mu.Lock()
	a.problem = ""
	a.mu.Unlock()
}

// poll calls check every interval (two seconds when 0) until it reports done, the time limit passes (the window then
// shows expired) or the sign-in is cancelled. check returns done with the problem to show ("" when it worked).
func (a *approval) poll(ctx context.Context, cancel context.CancelFunc, every time.Duration, log func(level, msg string),
	expired string, check func() (done bool, problem string)) {
	defer cancel() // ends this sign-in's time limit
	if every <= 0 {
		every = 2 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				a.finish(ctx, expired, log)
			}
			return
		case <-t.C:
		}
		if done, problem := check(); done {
			a.finish(ctx, problem, log)
			return
		}
	}
}

// forget is a sign-out: it ends a sign-in that still waits for approval and runs clear (which removes the saved
// sign-in) under the saving lock, so a sign-in or a renewal finishing at the same moment cannot write its token after
// the sign-out.
func (a *approval) forget(clear func() error) error {
	a.saving.Lock()
	defer a.saving.Unlock()
	a.mu.Lock()
	a.end()
	a.problem = ""
	a.mu.Unlock()
	return clear()
}

// save runs fn under the saving lock, unless the sign-in was cancelled meanwhile (signed out, or replaced by a newer
// sign-in): then it reports false and fn does not run.
func (a *approval) save(ctx context.Context, fn func() error) (bool, error) {
	a.saving.Lock()
	defer a.saving.Unlock()
	if ctx.Err() != nil {
		return false, nil
	}
	return true, fn()
}

// openPage opens u with open, or in the default browser when open is nil (tests replace the browser).
func openPage(open func(string) bool, u string) {
	if open == nil {
		open = winsys.OpenURL
	}
	open(u)
}

// installID is this install's identifier towards a service (the setting key; current is its saved value), made once.
// It outlives a sign-out, so signing in again is the same device to the service.
func installID(st *store.Store, key, current string) (string, error) {
	if current != "" {
		return current, nil
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	return id, st.SetHidden([]string{key}, map[string]any{key: id})
}

// account is what the window's endpoints need from each sign-in.
type account interface {
	state() map[string]any
	stopWaiting()
	signOut() error
}

// addAccount adds the endpoints every sign-in has: <name> (what the window shows), <name>-cancel and <name>-signout.
func addAccount(h map[string]server.Handler, name string, a account) {
	h[name] = server.Handler{Get: true, Fn: func(*jsonx.Obj) (any, error) { return a.state(), nil }}
	h[name+"-cancel"] = server.Handler{Fn: func(*jsonx.Obj) (any, error) { a.stopWaiting(); return a.state(), nil }}
	h[name+"-signout"] = server.Handler{Fn: func(*jsonx.Obj) (any, error) {
		if err := a.signOut(); err != nil {
			return nil, err
		}
		return a.state(), nil
	}}
}

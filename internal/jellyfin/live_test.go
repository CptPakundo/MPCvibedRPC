package jellyfin

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveJellyfin runs against a real Jellyfin or Emby server (tools/ci/live-jellyfin.sh starts the official images on
// CI, with a stand-in player that plays a sample episode). It only runs on request:
//
//	MPCRPC_LIVE_JELLYFIN_ADDRESS=http://127.0.0.1:8096 MPCRPC_LIVE_JELLYFIN_USER=... MPCRPC_LIVE_JELLYFIN_PASSWORD=...
//	MPCRPC_LIVE_JELLYFIN_PRODUCT=Jellyfin MPCRPC_LIVE_TITLE="Sample Show" go test -run TestLiveJellyfin ./internal/jellyfin
//
// It signs in with the password (and, on Jellyfin, with Quick Connect, approved with the first sign-in), follows what
// the user plays, and signs out again.
func TestLiveJellyfin(t *testing.T) {
	addr := os.Getenv("MPCRPC_LIVE_JELLYFIN_ADDRESS")
	if addr == "" {
		t.Skip("set MPCRPC_LIVE_JELLYFIN_ADDRESS to run against a real Jellyfin or Emby server")
	}
	user, pw, product := os.Getenv("MPCRPC_LIVE_JELLYFIN_USER"), os.Getenv("MPCRPC_LIVE_JELLYFIN_PASSWORD"), os.Getenv("MPCRPC_LIVE_JELLYFIN_PRODUCT")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := &Client{Base: addr, DeviceID: "ci-mpcvibedrpc", Version: "0.0.0"}
	srv, err := c.Find(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("server: %+v at %s", srv, c.Base)
	if product != "" && srv.Product != product {
		t.Fatalf("product %q, want %q", srv.Product, product)
	}
	if _, err := c.SignIn(ctx, user, pw+"-wrong"); !errors.Is(err, ErrLogin) {
		t.Fatalf("a wrong password: %v", err)
	}
	a, err := c.SignIn(ctx, user, pw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("signed in as %s (%s)", a.UserName, a.UserID)

	if srv.Product == "Jellyfin" {
		qc := &Client{Base: c.Base, DeviceID: "ci-mpcvibedrpc-qc", Version: "0.0.0"}
		p, err := qc.StartQuickConnect(ctx)
		if err != nil {
			t.Fatalf("Quick Connect: %v", err)
		}
		if a2, err := qc.CheckQuickConnect(ctx, p); err != nil || a2 != nil {
			t.Fatalf("approved before anyone approved it: %+v %v", a2, err)
		}
		// the user approves the code where they are signed in (here: with the password sign-in)
		if _, err := c.do(ctx, "POST", "/QuickConnect/Authorize?code="+p.Code, a.Token, nil, nil); err != nil {
			t.Fatalf("approving the code: %v", err)
		}
		var qa *Auth
		for qa == nil && ctx.Err() == nil {
			if qa, err = qc.CheckQuickConnect(ctx, p); err != nil {
				t.Fatalf("checking the code: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
		}
		if qa == nil || !sameID(qa.UserID, a.UserID) || qa.Token == a.Token {
			t.Fatalf("Quick Connect sign-in: %+v", qa)
		}
		if err := qc.SignOut(ctx, qa.Token); err != nil {
			t.Fatalf("sign-out: %v", err)
		}
		if _, err := qc.do(ctx, "GET", "/Sessions", qa.Token, nil, nil); !errors.Is(err, ErrToken) {
			t.Fatalf("a signed-out token still works: %v", err)
		}
		t.Log("Quick Connect: approved, signed in and signed out")
	}

	var raw []map[string]any
	if _, err := c.do(ctx, "GET", "/Sessions", a.Token, nil, &raw); err != nil {
		t.Fatal(err)
	}
	for _, s := range raw {
		if s["NowPlayingItem"] != nil {
			t.Logf("session: UserId=%v Client=%v LastPlaybackCheckIn=%v PlayState=%v NowPlayingItem.Type=%v",
				s["UserId"], s["Client"], s["LastPlaybackCheckIn"], s["PlayState"], s["NowPlayingItem"].(map[string]any)["Type"])
		}
	}
	w := &Watcher{Client: c, Token: a.Token, UserID: a.UserID, Product: srv.Product}
	title := os.Getenv("MPCRPC_LIVE_TITLE")
	for {
		if err := w.poll(ctx); err != nil {
			t.Fatal(err)
		}
		if in := w.Now(); in != nil && in.State == 2 && strings.Contains(in.File, title) {
			t.Logf("playing: %q at %d of %d ms", in.File, in.Position, in.Duration)
			if in.Duration <= 0 || in.Position <= 0 {
				t.Fatalf("position and length: %+v", in)
			}
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("the stand-in player's video was not found: %+v", w.Now())
		}
		time.Sleep(time.Second)
	}
	if err := c.SignOut(ctx, a.Token); err != nil {
		t.Fatalf("sign-out: %v", err)
	}
}

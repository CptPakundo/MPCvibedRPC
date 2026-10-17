package plex

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeKeyTV is plex.tv's newer sign-in as developer.plex.tv describes it: a PIN that carries the install's JWK, a
// device JWT to collect the token, and nonce + signed JWT for a new one. Every signature is checked against the JWK.
type fakeKeyTV struct {
	mu       sync.Mutex
	jwk      map[string]any
	approved bool
	nonce    string
	issued   int
	problems []string
}

func (f *fakeKeyTV) fail(s string) { f.problems = append(f.problems, s) }

// verify checks a device JWT: the registered key signed it, for plex.tv, from clientID, with the nonce (if any).
func (f *fakeKeyTV) verify(jwt, clientID, nonce string) bool {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 || f.jwk == nil {
		return false
	}
	pub, _ := base64.RawURLEncoding.DecodeString(fmt.Sprint(f.jwk["x"]))
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), sig) {
		return false
	}
	var head, claims map[string]any
	h, _ := base64.RawURLEncoding.DecodeString(parts[0])
	c, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if json.Unmarshal(h, &head) != nil || json.Unmarshal(c, &claims) != nil {
		return false
	}
	if head["alg"] != "EdDSA" || head["kid"] != f.jwk["kid"] || claims["aud"] != "plex.tv" || claims["iss"] != clientID {
		return false
	}
	if strings.Contains(fmt.Sprint(claims["scope"]), "email") {
		return false // the program never asks for the address
	}
	return nonce == "" || claims["nonce"] == nonce
}

// token is a JWT-shaped token that runs out in 7 days (only its exp matters to the program).
func token(n int) string {
	c, _ := json.Marshal(map[string]any{"exp": time.Now().Add(7 * 24 * time.Hour).Unix(), "n": n})
	return "e30." + base64.RawURLEncoding.EncodeToString(c) + ".sig"
}

func (f *fakeKeyTV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.Header.Get("X-Plex-Client-Identifier")
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/v2/pins":
		var body struct {
			Strong bool           `json:"strong"`
			JWK    map[string]any `json:"jwk"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || !body.Strong || body.JWK["kty"] != "OKP" || body.JWK["crv"] != "Ed25519" || body.JWK["alg"] != "EdDSA" {
			f.fail("bad pin request")
			w.WriteHeader(400)
			return
		}
		f.jwk = body.JWK
		w.Write([]byte(`{"id":42,"code":"samplecode"}`))
	case r.Method == "GET" && r.URL.Path == "/api/v2/pins/42":
		if !f.verify(r.URL.Query().Get("deviceJWT"), id, "") {
			f.fail("bad device JWT for the pin")
			w.WriteHeader(401)
			return
		}
		if !f.approved {
			w.Write([]byte(`{"id":42,"authToken":null}`))
			return
		}
		f.issued++
		fmt.Fprintf(w, `{"id":42,"authToken":%q}`, token(f.issued))
	case r.Method == "GET" && r.URL.Path == "/api/v2/auth/nonce":
		f.nonce = fmt.Sprintf("nonce-%d", time.Now().UnixNano())
		fmt.Fprintf(w, `{"nonce":%q}`, f.nonce)
	case r.Method == "POST" && r.URL.Path == "/api/v2/auth/token":
		var body struct {
			JWT string `json:"jwt"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || f.nonce == "" || !f.verify(body.JWT, id, f.nonce) {
			f.fail("bad refresh")
			w.WriteHeader(401)
			return
		}
		f.nonce = "" // used up
		f.issued++
		fmt.Fprintf(w, `{"auth_token":%q}`, token(f.issued))
	default:
		w.WriteHeader(404)
	}
}

func TestKeySignInAndRefresh(t *testing.T) {
	tv := &fakeKeyTV{}
	srv := httptest.NewServer(tv)
	defer srv.Close()
	c := &Client{ClientID: "sample-install", Version: "0.0.0", TVBase: srv.URL, ResourcesBase: srv.URL}
	ctx := context.Background()

	k, err := NewDeviceKey()
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseDeviceKey(k.String())
	if err != nil || again.String() != k.String() || again.ID != k.ID {
		t.Fatalf("a saved key does not read back: %v", err)
	}
	pin, err := c.NewKeyPin(ctx, k)
	if err != nil || pin.Code != "samplecode" {
		t.Fatalf("pin: %+v %v", pin, err)
	}
	if got, err := c.CheckKeyPin(ctx, pin, k); err != nil || got != "" {
		t.Fatalf("before approval: %q %v", got, err)
	}
	tv.mu.Lock()
	tv.approved = true
	tv.mu.Unlock()
	tok, err := c.CheckKeyPin(ctx, pin, again) // the key as read back from the settings
	if err != nil || tok == "" {
		t.Fatalf("after approval: %q %v", tok, err)
	}
	if exp := TokenExpiry(tok); time.Until(exp) < 6*24*time.Hour {
		t.Fatalf("token runs out at %v", exp)
	}
	fresh, err := c.Refresh(ctx, again)
	if err != nil || fresh == "" || fresh == tok {
		t.Fatalf("refresh: %q %v", fresh, err)
	}

	other, _ := NewDeviceKey() // a key plex.tv does not know is refused
	if _, err := c.Refresh(ctx, other); err == nil {
		t.Fatal("a refresh signed with another key was accepted")
	}
	tv.mu.Lock()
	defer tv.mu.Unlock()
	if len(tv.problems) != 1 || tv.problems[0] != "bad refresh" {
		t.Fatalf("plex.tv saw problems: %q", tv.problems)
	}
}

func TestTokenExpiryAndBadKeys(t *testing.T) {
	if !TokenExpiry("an-older-long-lived-token").IsZero() {
		t.Error("an older token has no expiry")
	}
	if got := TokenExpiry(token(1)); got.Before(time.Now().Add(6 * 24 * time.Hour)) {
		t.Errorf("expiry %v", got)
	}
	for _, s := range []string{"", "nodot", ".abc", "id.!!!", "id." + base64.RawURLEncoding.EncodeToString([]byte("short"))} {
		if _, err := ParseDeviceKey(s); err == nil {
			t.Errorf("ParseDeviceKey(%q) accepted", s)
		}
	}
}

// A 498 from Plex is ErrExpired.
func TestExpiredIs498(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(498) }))
	defer srv.Close()
	c := &Client{ClientID: "sample-install", ResourcesBase: srv.URL}
	k, _ := NewDeviceKey()
	if _, err := c.Refresh(context.Background(), k); err != ErrExpired {
		t.Fatalf("got %v; want ErrExpired", err)
	}
}

package plex

// Plex's newer sign-in (developer.plex.tv, "Authenticating with Plex"): each install has an Ed25519 key of its own,
// plex.tv learns its public half (a JWK) with the PIN, and hands out a token (a JWT) that lasts 7 days. Before it
// runs out, the install asks plex.tv for a nonce, signs a short "device JWT" with its key, and trades that for a new
// token. Older long-lived tokens still work; this is used only when the plexJwt setting is on.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrExpired means Plex refused a token because it ran out (HTTP 498); a refresh gets a new one.
var ErrExpired = errors.New("the Plex sign-in ran out")

// DeviceKey is this install's signing key on plex.tv.
type DeviceKey struct {
	ID  string // the JWK's kid
	key ed25519.PrivateKey
}

// NewDeviceKey makes a new key with a random id.
func NewDeviceKey() (*DeviceKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	return &DeviceKey{ID: hex.EncodeToString(id), key: priv}, nil
}

// ParseDeviceKey reads a key saved with String.
func ParseDeviceKey(s string) (*DeviceKey, error) {
	id, seed, ok := strings.Cut(s, ".")
	if !ok || id == "" {
		return nil, errors.New("plex: not a saved key")
	}
	b, err := base64.RawURLEncoding.DecodeString(seed)
	if err != nil || len(b) != ed25519.SeedSize {
		return nil, errors.New("plex: not a saved key")
	}
	return &DeviceKey{ID: id, key: ed25519.NewKeyFromSeed(b)}, nil
}

// String is the key as it is saved in the settings: its id and its private seed. It is a secret.
func (k *DeviceKey) String() string {
	return k.ID + "." + base64.RawURLEncoding.EncodeToString(k.key.Seed())
}

// JWK is the public half as plex.tv takes it.
func (k *DeviceKey) JWK() map[string]any {
	pub := k.key.Public().(ed25519.PublicKey)
	return map[string]any{"kty": "OKP", "crv": "Ed25519", "x": base64.RawURLEncoding.EncodeToString(pub), "kid": k.ID, "alg": "EdDSA", "use": "sig"}
}

// scope is what the program asks to know about the account: its name, for the window. Never the email address.
const scope = "username,friendly_name"

// sign makes a device JWT for clientID: plex.tv is the audience, the install the issuer, valid for an hour; nonce is
// left out when empty (the PIN exchange takes none).
func (k *DeviceKey) sign(clientID, nonce string, now time.Time) (string, error) {
	head, err := json.Marshal(map[string]any{"alg": "EdDSA", "kid": k.ID, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claims := map[string]any{"aud": "plex.tv", "iss": clientID, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "scope": scope}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	msg := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body)
	return msg + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(k.key, []byte(msg))), nil
}

// TokenExpiry reads when a token from plex.tv runs out (the "exp" of its JWT, not checked: only plex.tv can). A
// token that is not a JWT (an older long-lived one) gives the zero time.
func TokenExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var c struct {
		Exp json.Number `json:"exp"`
	}
	if json.Unmarshal(b, &c) != nil {
		return time.Time{}
	}
	n, err := strconv.ParseInt(c.Exp.String(), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

// NewKeyPin starts a sign-in that hands plex.tv this install's public key with the PIN.
func (c *Client) NewKeyPin(ctx context.Context, k *DeviceKey) (*Pin, error) {
	var p Pin
	if _, err := c.send(ctx, "POST", c.resources()+"/api/v2/pins", "", map[string]any{"strong": true, "jwk": k.JWK()}, &p); err != nil {
		return nil, err
	}
	if p.ID == 0 || p.Code == "" {
		return nil, errors.New("plex.tv did not hand out a sign-in code")
	}
	return &p, nil
}

// CheckKeyPin returns the account's token (a JWT) once the user has approved the sign-in, "" while they have not yet.
func (c *Client) CheckKeyPin(ctx context.Context, p *Pin, k *DeviceKey) (string, error) {
	jwt, err := k.sign(c.ClientID, "", time.Now())
	if err != nil {
		return "", err
	}
	var r struct {
		AuthToken *string `json:"authToken"`
	}
	status, err := c.send(ctx, "GET", fmt.Sprintf("%s/api/v2/pins/%d?deviceJWT=%s", c.resources(), p.ID, url.QueryEscape(jwt)), "", nil, &r)
	if status == http.StatusNotFound {
		return "", ErrPinExpired
	}
	if err != nil {
		return "", err
	}
	if r.AuthToken == nil {
		return "", nil
	}
	return *r.AuthToken, nil
}

// Refresh trades a signed nonce for a new token. It works before and after the old one ran out.
func (c *Client) Refresh(ctx context.Context, k *DeviceKey) (string, error) {
	var n struct {
		Nonce string `json:"nonce"`
	}
	if _, err := c.send(ctx, "GET", c.resources()+"/api/v2/auth/nonce", "", nil, &n); err != nil {
		return "", err
	}
	if n.Nonce == "" {
		return "", errors.New("plex.tv gave no nonce")
	}
	jwt, err := k.sign(c.ClientID, n.Nonce, time.Now())
	if err != nil {
		return "", err
	}
	var t struct {
		Token string `json:"auth_token"`
	}
	if _, err := c.send(ctx, "POST", c.resources()+"/api/v2/auth/token", "", map[string]string{"jwt": jwt}, &t); err != nil {
		return "", err
	}
	if t.Token == "" {
		return "", errors.New("plex.tv gave no new token")
	}
	return t.Token, nil
}

// Package plex reads what the signed-in user plays on a Plex Media Server. The account part signs in through plex.tv
// (a PIN the user approves in the browser) and lists the servers the account can use; the watcher part follows one
// server's play notifications (watcher.go). Only reading is done: nothing is ever played, paused or changed.
package plex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
)

// Product is how the program introduces itself to Plex (it is shown in the account's list of authorized devices).
const Product = "MPCvibedRPC"

var (
	// ErrPinExpired means the sign-in code ran out before it was approved (or plex.tv no longer knows it).
	ErrPinExpired = errors.New("the sign-in code expired")
	// ErrToken means plex.tv or the server refused the saved sign-in.
	ErrToken = errors.New("Plex refused the sign-in")
)

// Client talks to plex.tv and to servers on behalf of one install of the program.
type Client struct {
	HTTP     *http.Client // nil = http.DefaultClient
	ClientID string       // X-Plex-Client-Identifier: one per install, kept in the settings
	Version  string       // the program's version

	// Where plex.tv is; tests point these at a fake. Empty means the real services.
	TVBase        string // https://plex.tv
	ResourcesBase string // https://clients.plex.tv
}

func (c *Client) tv() string {
	if c.TVBase != "" {
		return strings.TrimRight(c.TVBase, "/")
	}
	return "https://plex.tv"
}

func (c *Client) resources() string {
	if c.ResourcesBase != "" {
		return strings.TrimRight(c.ResourcesBase, "/")
	}
	return "https://clients.plex.tv"
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func platform() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	}
	return "Linux"
}

// header sets what Plex expects from every app. The token goes in a header, never in the address.
func (c *Client) header(req *http.Request, token string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", Product)
	req.Header.Set("X-Plex-Version", c.Version)
	req.Header.Set("X-Plex-Client-Identifier", c.ClientID)
	req.Header.Set("X-Plex-Platform", platform())
	req.Header.Set("X-Plex-Device-Name", Product)
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
}

// getJSON asks for url and decodes the answer into v. A 401 is ErrToken; status gives the code for the caller's
// own handling of anything else.
func (c *Client) getJSON(ctx context.Context, method, u, token string, v any) (status int, err error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return 0, err
	}
	c.header(req, token)
	res, err := c.http().Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return res.StatusCode, err
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return res.StatusCode, ErrToken
	case res.StatusCode < 200 || res.StatusCode > 299:
		return res.StatusCode, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v != nil {
		if err := json.Unmarshal(body, v); err != nil {
			return res.StatusCode, fmt.Errorf("unexpected answer: %w", err)
		}
	}
	return res.StatusCode, nil
}

// Pin is a sign-in in progress: the user approves Code on plex.tv, then CheckPin returns the account's token.
type Pin struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
}

// NewPin starts a sign-in.
func (c *Client) NewPin(ctx context.Context) (*Pin, error) {
	var p Pin
	if _, err := c.getJSON(ctx, "POST", c.tv()+"/api/v2/pins?strong=true", "", &p); err != nil {
		return nil, err
	}
	if p.ID == 0 || p.Code == "" {
		return nil, errors.New("plex.tv did not hand out a sign-in code")
	}
	return &p, nil
}

// AuthURL is the page where the user approves the sign-in.
func (c *Client) AuthURL(p *Pin) string {
	q := url.Values{}
	q.Set("clientID", c.ClientID)
	q.Set("code", p.Code)
	q.Set("context[device][product]", Product)
	return "https://app.plex.tv/auth#?" + q.Encode()
}

// CheckPin returns the token once the user has approved the sign-in, "" while they have not yet.
func (c *Client) CheckPin(ctx context.Context, p *Pin) (string, error) {
	var r struct {
		AuthToken *string `json:"authToken"`
	}
	status, err := c.getJSON(ctx, "GET", fmt.Sprintf("%s/api/v2/pins/%d", c.tv(), p.ID), "", &r)
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

// Account is who signed in. Only the id (to tell the user's own playback from other people's on a server they own)
// and a name for the window are kept.
type Account struct {
	ID   int64
	Name string
}

// Account asks plex.tv who the token belongs to.
func (c *Client) Account(ctx context.Context, token string) (*Account, error) {
	var u struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Title    string `json:"title"`
	}
	if _, err := c.getJSON(ctx, "GET", c.tv()+"/api/v2/user", token, &u); err != nil {
		return nil, err
	}
	name := u.Username
	if name == "" {
		name = u.Title
	}
	return &Account{ID: u.ID, Name: name}, nil
}

// Server is a Plex Media Server the account can use.
type Server struct {
	ID          string       `json:"clientIdentifier"` // the server's machine identifier
	Name        string       `json:"name"`
	Owned       bool         `json:"owned"`       // the user's own server (it shows everybody's playback, so the user's is picked out)
	Token       string       `json:"accessToken"` // the token for this server (on a shared server it differs from the account's)
	Provides    string       `json:"provides"`
	Connections []Connection `json:"connections"`
}

// Connection is one way of reaching a server.
type Connection struct {
	URI   string `json:"uri"`
	Local bool   `json:"local"`
	Relay bool   `json:"relay"`
}

// Servers lists the servers the account can use, its own first.
func (c *Client) Servers(ctx context.Context, token string) ([]Server, error) {
	var all []Server
	if _, err := c.getJSON(ctx, "GET", c.resources()+"/api/v2/resources?includeHttps=1&includeRelay=1", token, &all); err != nil {
		return nil, err
	}
	var own, shared []Server
	for _, s := range all {
		if !providesServer(s.Provides) || s.ID == "" {
			continue
		}
		if s.Owned {
			own = append(own, s)
		} else {
			shared = append(shared, s)
		}
	}
	return append(own, shared...), nil
}

func providesServer(p string) bool {
	for _, x := range strings.Split(p, ",") {
		if strings.TrimSpace(x) == "server" {
			return true
		}
	}
	return false
}

// Addresses lists where to try the server, best first: addresses on the local network, then the others, then
// Plex's relay (slow and limited, but it works when nothing else does).
func (s Server) Addresses() []string {
	var out []string
	for _, pass := range []func(Connection) bool{
		func(c Connection) bool { return c.Local && !c.Relay },
		func(c Connection) bool { return !c.Local && !c.Relay },
		func(c Connection) bool { return c.Relay },
	} {
		for _, c := range s.Connections {
			if pass(c) && c.URI != "" {
				out = append(out, strings.TrimRight(c.URI, "/"))
			}
		}
	}
	return out
}

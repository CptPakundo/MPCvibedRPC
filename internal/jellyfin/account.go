// Package jellyfin reads what the signed-in user plays on their own Jellyfin or Emby server, on any device. The account
// part signs in with Quick Connect (a code the user approves in a Jellyfin app they are signed in to) or with a user
// name and password, and signs out again; the watcher part asks the server's session list what plays (watcher.go).
// Only reading is done: nothing on the server is ever changed.
package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/remote"
)

var (
	// ErrToken means the server refused the saved sign-in (it was signed out there, or the user was removed).
	ErrToken = errors.New("the server refused the sign-in")
	// ErrLogin means the server refused the user name or password.
	ErrLogin = errors.New("the server refused the user name or password")
	// ErrNoQuickConnect means Quick Connect is off on the server (or it is an Emby server, which has none).
	ErrNoQuickConnect = errors.New("Quick Connect is not available on this server")
	// ErrNotServer means the address answers, but not as a Jellyfin or Emby server.
	ErrNotServer = errors.New("this address does not answer as a Jellyfin or Emby server")
)

// errUnexpected is an answer that is not what the server should send (the message goes on with what was wrong), and
// errNoSignIn a sign-in that came without a token or user.
var (
	errUnexpected = errors.New("unexpected answer")
	errNoSignIn   = errors.New("the server did not hand out a sign-in")
)

// httpError is a status the server answered with that is neither a success nor a refusal of the sign-in.
type httpError struct{ Status int }

func (e *httpError) Error() string { return fmt.Sprintf("HTTP %d", e.Status) }

// isStatus tells whether err is an answer with that status.
func isStatus(err error, status int) bool {
	var he *httpError
	return errors.As(err, &he) && he.Status == status
}

// Client talks to one server.
type Client struct {
	HTTP     *http.Client // nil = a client with a 15-second limit
	Base     string       // the server's address, such as http://192.168.1.20:8096 (see Address)
	DeviceID string       // this install's identifier towards the server
	Version  string       // the program's version
}

// defaultHTTP is the shared client (internal/remote): Go keeps the X-Emby headers (which carry the token) on a redirect
// elsewhere, and a sign-in would send the password along, so a redirect to another address is refused.
var defaultHTTP = remote.NewClient(15 * time.Second)

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTP
}

// Address turns what the user typed into a server address: http:// is added when no scheme is given, and the address
// must be a plain http(s) address (a path is kept, for a server behind a reverse proxy).
func Address(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("Enter your server's address, for example http://192.168.1.20:8096.")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("That is not a server address. It looks like http://192.168.1.20:8096.")
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.EscapedPath(), "/"), nil
}

// auth is the client description servers want with every request; token is added once signed in. Jellyfin reads the
// Authorization header ("MediaBrowser" scheme); Emby reads X-Emby-Authorization ("Emby" scheme) and X-Emby-Token.
func (c *Client) auth(req *http.Request, token string) {
	q := func(s string) string { return `"` + strings.NewReplacer(`"`, "", ",", " ").Replace(s) + `"` }
	parts := `Client=` + q("MPCvibedRPC") + `, Device=` + q("MPCvibedRPC on "+remote.OSName()) + `, DeviceId=` + q(c.DeviceID) + `, Version=` + q(c.Version)
	if token != "" {
		parts += `, Token=` + q(token)
		req.Header.Set("X-Emby-Token", token)
	}
	req.Header.Set("Authorization", "MediaBrowser "+parts)
	req.Header.Set("X-Emby-Authorization", "Emby "+parts)
	req.Header.Set("Accept", "application/json")
}

// do sends a request and decodes the JSON answer into v (when v is not nil). It also returns the server's clock, from
// the Date header (zero when there is none).
func (c *Client) do(ctx context.Context, method, path, token string, body, v any) (time.Time, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return time.Time{}, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.Base+path, rd)
	if err != nil {
		return time.Time{}, err
	}
	c.auth(req, token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, raw, err := remote.Fetch(ctx, c.http(), req, 16<<20)
	if err != nil {
		return time.Time{}, err
	}
	date, _ := http.ParseTime(res.Header.Get("Date"))
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return date, ErrToken
	case res.StatusCode < 200 || res.StatusCode > 299:
		return date, &httpError{res.StatusCode}
	}
	if v != nil {
		if err := json.Unmarshal(raw, v); err != nil {
			return date, fmt.Errorf("%w: %v", errUnexpected, err)
		}
	}
	return date, nil
}

// Server is what a server says about itself before anyone signs in.
type Server struct {
	ID      string
	Name    string
	Version string
	Product string // Jellyfin or Emby
}

// Find asks the address who it is. An Emby server that only answers under /emby is found there, and Base is changed
// to match.
func (c *Client) Find(ctx context.Context) (*Server, error) {
	var lastErr error
	for _, prefix := range []string{"", "/emby"} {
		cc := *c
		cc.Base = c.Base + prefix
		var r struct {
			ID          string `json:"Id"`
			ServerName  string `json:"ServerName"`
			Version     string `json:"Version"`
			ProductName string `json:"ProductName"`
		}
		if _, err := cc.do(ctx, "GET", "/System/Info/Public", "", nil, &r); err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, err
			}
			continue
		}
		if r.ID == "" {
			lastErr = ErrNotServer
			continue
		}
		c.Base = cc.Base
		s := &Server{ID: r.ID, Name: r.ServerName, Version: r.Version, Product: "Emby"}
		if strings.Contains(strings.ToLower(r.ProductName), "jellyfin") {
			s.Product = "Jellyfin"
		}
		return s, nil
	}
	var he *httpError
	if errors.As(lastErr, &he) || errors.Is(lastErr, errUnexpected) {
		return nil, ErrNotServer
	}
	return nil, lastErr
}

// Auth is a sign-in: the token the server gave this install and whose it is.
type Auth struct {
	Token    string
	UserID   string
	UserName string
}

type authResult struct {
	AccessToken string `json:"AccessToken"`
	User        struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	} `json:"User"`
}

func (r *authResult) auth() (*Auth, error) {
	if r.AccessToken == "" || r.User.ID == "" {
		return nil, errNoSignIn
	}
	return &Auth{Token: r.AccessToken, UserID: r.User.ID, UserName: r.User.Name}, nil
}

// SignIn signs in with a user name and password. The password is only passed on to the server, never kept.
func (c *Client) SignIn(ctx context.Context, user, password string) (*Auth, error) {
	var r authResult
	_, err := c.do(ctx, "POST", "/Users/AuthenticateByName", "", map[string]string{"Username": user, "Pw": password}, &r)
	if errors.Is(err, ErrToken) {
		return nil, ErrLogin
	}
	if err != nil {
		return nil, err
	}
	return r.auth()
}

// Pending is a Quick Connect sign-in waiting for the user: they enter Code in a Jellyfin app they are signed in to.
type Pending struct {
	Code   string `json:"Code"`
	Secret string `json:"Secret"`
}

// StartQuickConnect asks the server for a Quick Connect code.
func (c *Client) StartQuickConnect(ctx context.Context) (*Pending, error) {
	var p Pending
	_, err := c.do(ctx, "POST", "/QuickConnect/Initiate", "", nil, &p)
	if errors.Is(err, ErrToken) || isStatus(err, http.StatusNotFound) {
		return nil, ErrNoQuickConnect // switched off (401), or no Quick Connect at all (Emby)
	}
	if err != nil {
		return nil, err
	}
	if p.Code == "" || p.Secret == "" {
		return nil, ErrNoQuickConnect
	}
	return &p, nil
}

// CheckQuickConnect returns the sign-in once the user has approved the code, nil while they have not yet.
func (c *Client) CheckQuickConnect(ctx context.Context, p *Pending) (*Auth, error) {
	var r struct {
		Authenticated bool `json:"Authenticated"`
	}
	_, err := c.do(ctx, "GET", "/QuickConnect/Connect?secret="+url.QueryEscape(p.Secret), "", nil, &r)
	if err != nil {
		if isStatus(err, http.StatusNotFound) {
			return nil, ErrExpired
		}
		return nil, err
	}
	if !r.Authenticated {
		return nil, nil
	}
	var a authResult
	if _, err := c.do(ctx, "POST", "/Users/AuthenticateWithQuickConnect", "", map[string]string{"Secret": p.Secret}, &a); err != nil {
		return nil, fmt.Errorf("%w (%s)", ErrIncomplete, Reason(err))
	}
	return a.auth()
}

// ErrIncomplete means the user approved the code but the server did not hand over the sign-in.
var ErrIncomplete = errors.New("the code was approved, but the server did not complete the sign-in")

// ErrExpired means the server no longer knows the Quick Connect code (it lasts a few minutes).
var ErrExpired = errors.New("the Quick Connect code expired")

// SignOut ends this install's session on the server, so the token is worth nothing afterwards.
func (c *Client) SignOut(ctx context.Context, token string) error {
	_, err := c.do(ctx, "POST", "/Sessions/Logout", token, nil, nil)
	return err
}

// Reason says briefly why a request failed, without the server's address (which the log and the diagnostics must not
// hold).
func Reason(err error) string {
	var he *httpError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrToken), errors.Is(err, ErrLogin), errors.Is(err, ErrNotServer), errors.Is(err, ErrNoQuickConnect), errors.Is(err, ErrExpired), errors.Is(err, ErrIncomplete):
		return err.Error()
	case errors.As(err, &he), errors.Is(err, errUnexpected), errors.Is(err, errNoSignIn):
		return err.Error()
	}
	if s := remote.NetReason(err); s != "" {
		return s
	}
	return remote.NoConnection
}

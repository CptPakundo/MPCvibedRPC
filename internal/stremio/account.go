// Package stremio reads what the signed-in user watches in Stremio, on any device. The account part signs in with a
// link code the user approves on stremio.com (the way Stremio's TV apps sign in) and signs out again; the watcher part
// follows the account's library, which Stremio's player updates while it plays (watcher.go). Only reading is done:
// nothing in the library or the account is ever changed.
package stremio

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
)

var (
	// ErrKey means Stremio refused the saved sign-in (it was signed out elsewhere, or it expired).
	ErrKey = errors.New("Stremio refused the sign-in")
	// ErrBadLink means the sign-in page Stremio handed out is not on stremio.com (it is never opened then).
	ErrBadLink = errors.New("Stremio handed out an unexpected sign-in page")
)

// Client talks to Stremio's account services.
type Client struct {
	HTTP *http.Client // nil = http.DefaultClient
	// Where the services are; tests point these at a fake. Empty means the real services.
	APIBase  string // https://api.strem.io
	LinkBase string // https://link.stremio.com
}

func (c *Client) api() string {
	if c.APIBase != "" {
		return strings.TrimRight(c.APIBase, "/")
	}
	return "https://api.strem.io"
}

func (c *Client) link() string {
	if c.LinkBase != "" {
		return strings.TrimRight(c.LinkBase, "/")
	}
	return "https://link.stremio.com"
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// apiError is how Stremio's services answer a request they refuse: {"error":{"message":..,"code":..}}.
type apiError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *apiError) Error() string { return "Stremio: " + e.Message }

// call sends a request and decodes the "result" of the answer into v. A refused session is ErrKey.
func (c *Client) call(ctx context.Context, method, u string, body any, v any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return err
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return ErrKey
	}
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *apiError       `json:"error"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		if res.StatusCode < 200 || res.StatusCode > 299 {
			return fmt.Errorf("HTTP %d", res.StatusCode)
		}
		return fmt.Errorf("unexpected answer: %w", err)
	}
	if r.Error != nil {
		m := strings.ToLower(r.Error.Message)
		if strings.Contains(m, "session") || strings.Contains(m, "auth") {
			return ErrKey
		}
		return r.Error
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v != nil && len(r.Result) > 0 {
		if err := json.Unmarshal(r.Result, v); err != nil {
			return fmt.Errorf("unexpected answer: %w", err)
		}
	}
	return nil
}

// Link is a sign-in in progress: the user opens URL and approves Code, then CheckLink returns the account's key.
type Link struct {
	Code string `json:"code"`
	URL  string `json:"link"`
}

// NewLink starts a sign-in.
func (c *Client) NewLink(ctx context.Context) (*Link, error) {
	var l Link
	if err := c.call(ctx, "GET", c.link()+"/api/v2/create?type=Create", nil, &l); err != nil {
		return nil, err
	}
	if l.Code == "" || l.URL == "" {
		return nil, errors.New("Stremio did not hand out a sign-in code")
	}
	if !stremioPage(l.URL) {
		return nil, ErrBadLink
	}
	return &l, nil
}

// stremioPage is true for an https page on stremio.com (the only kind of page a sign-in may open).
func stremioPage(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "stremio.com" || strings.HasSuffix(h, ".stremio.com")
}

// CheckLink returns the account's key once the user has approved the sign-in, "" while they have not yet.
func (c *Client) CheckLink(ctx context.Context, l *Link) (string, error) {
	var r struct {
		AuthKey string `json:"authKey"`
	}
	err := c.call(ctx, "GET", c.link()+"/api/v2/read?type=Read&code="+url.QueryEscape(l.Code), nil, &r)
	var refused *apiError
	if err != nil && !errors.Is(err, ErrKey) && !errors.As(err, &refused) {
		return "", err // the service did not answer; a refusal only means "not approved yet"
	}
	return r.AuthKey, nil
}

// SignOut ends the session of key on Stremio's side, so the key is worth nothing afterwards.
func (c *Client) SignOut(ctx context.Context, key string) error {
	return c.call(ctx, "POST", c.api()+"/api/logout", map[string]string{"authKey": key}, nil)
}

// Item is one entry of the account's library (only the parts used). Stremio's player updates the entry of what it
// plays: on start, pause and stop, and every 90 seconds while it plays.
type Item struct {
	ID      string `json:"_id"`
	Name    string `json:"name"`
	Type    string `json:"type"` // movie, series, channel, tv, other
	Removed bool   `json:"removed"`
	MTime   string `json:"_mtime"`
	State   struct {
		TimeOffset int64  `json:"timeOffset"` // ms
		Duration   int64  `json:"duration"`   // ms
		VideoID    string `json:"video_id"`   // the episode of a series: "<id>:<season>:<episode>"
	} `json:"state"`
}

// mtimes lists every library entry with when it last changed (ms since 1970).
func (c *Client) mtimes(ctx context.Context, key string) (map[string]int64, error) {
	var list [][2]json.RawMessage
	err := c.call(ctx, "POST", c.api()+"/api/datastoreMeta", map[string]any{"authKey": key, "collection": "libraryItem"}, &list)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(list))
	for _, e := range list {
		var id string
		var ms float64
		if json.Unmarshal(e[0], &id) != nil || json.Unmarshal(e[1], &ms) != nil {
			continue
		}
		out[id] = int64(ms)
	}
	return out, nil
}

// items fetches library entries.
func (c *Client) items(ctx context.Context, key string, ids []string) ([]Item, error) {
	var list []Item
	err := c.call(ctx, "POST", c.api()+"/api/datastoreGet", map[string]any{"authKey": key, "collection": "libraryItem", "ids": ids, "all": false}, &list)
	return list, err
}

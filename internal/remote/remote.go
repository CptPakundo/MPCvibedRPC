// Package remote holds the shared pieces of the Plex, Stremio and Jellyfin readers.
package remote

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"
)

// LogFunc is where a watcher writes its log lines. A nil LogFunc discards them.
type LogFunc func(level, msg string)

// Print writes one line.
func (f LogFunc) Print(level, msg string) {
	if f != nil {
		f(level, msg)
	}
}

// Clock is a watcher's time source; a nil Clock is the real time. Tests set one to move time by hand.
type Clock func() time.Time

// Now is the clock's time.
func (c Clock) Now() time.Time {
	if c != nil {
		return c()
	}
	return time.Now()
}

// Base is embedded in a watcher. It keeps the reason the service cannot be followed, which the window shows.
type Base struct {
	mu      sync.Mutex
	problem string
}

// Problem says why the service cannot be followed right now ("" when all is well).
func (b *Base) Problem() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.problem
}

// SetProblem records why the service cannot be followed and logs a warning when the text changes.
func (b *Base) SetProblem(p string, log LogFunc) {
	b.mu.Lock()
	changed := p != b.problem
	b.problem = p
	b.mu.Unlock()
	if changed && p != "" {
		log.Print("WARN", p)
	}
}

// OSName is the operating system as the services are told about it.
func OSName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	}
	return "Linux"
}

// NoConnection is the wording for a request that did not get through.
const NoConnection = "no connection"

// NetReason words the failures every service reports alike, without the server's address (a failed connection is a
// *url.Error that carries it). It returns "" for any other error.
func NetReason(err error) string {
	var ne net.Error
	switch {
	case errors.As(err, new(*tls.CertificateVerificationError)):
		return "the server's certificate is not trusted (use its http:// address, or give it a valid certificate)"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return "no answer in time"
	}
	return ""
}

// SameOrigin tells whether a redirect stays on the host of the first request and does not drop from https to http.
func SameOrigin(req *http.Request, via []*http.Request) bool {
	return req.URL.Host == via[0].URL.Host && !(via[0].URL.Scheme == "https" && req.URL.Scheme != "https")
}

// NewClient is the HTTP client of the Plex, Stremio and Jellyfin readers. It has no Transport of its own, so every
// request goes through http.DefaultTransport as it is at that moment (internal/conns wraps it at start-up to count
// connections: never set or clone a transport here). A redirect to another host, or from https to http, is refused:
// Go would send the headers that carry a sign-in on to the new address, and a POST body (a key, a password) again on a
// 307 or 308. timeout limits a whole request; 0 means none (an event stream).
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: sameServer}
}

func sameServer(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	if !SameOrigin(req, via) {
		return errors.New("the server sent the request to another address")
	}
	return nil
}

// Fetch sends req on c under ctx and reads at most limit bytes of the answer. The response is nil when no answer came;
// otherwise its status and headers are valid (its body is read and closed), also with a read error. What the status
// means is up to the caller.
func Fetch(ctx context.Context, c *http.Client, req *http.Request, limit int64) (*http.Response, []byte, error) {
	res, err := c.Do(req.WithContext(ctx))
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, limit))
	return res, body, err
}

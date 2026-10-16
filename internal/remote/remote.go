// Package remote holds the shared pieces of the Plex, Stremio and Jellyfin readers.
package remote

import (
	"context"
	"crypto/tls"
	"errors"
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

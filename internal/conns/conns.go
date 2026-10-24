// Package conns counts the program's web requests to other computers, by host, for "Recent connections" on the
// Privacy tab: the host, how often and when last, since the program started. Never a path, a query or anything
// about a title. Requests to this computer (players, the window) are not counted.
package conns

import (
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// Entry is one host.
type Entry struct {
	Host  string `json:"host"`
	Count int    `json:"count"`
	Last  int64  `json:"last"`  // Unix milliseconds
	How   string `json:"how"`   // core.HostAsked for a listed address, "" for one you entered (your own server)
	Label string `json:"label"` // the service of a listed address ("TVmaze", "GitHub"), else ""
}

var (
	mu    sync.Mutex
	hosts = map[string]*Entry{}
	now   = time.Now
	// base is the standard transport as it was before main wrapped it (so Wrap(nil) never wraps the wrapper)
	base = http.DefaultTransport
)

const maxHosts = 100 // a long-running copy with many server addresses keeps the first hundred

type counting struct{ next http.RoundTripper }

func (c counting) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL != nil {
		Note(r.URL.Hostname())
	}
	return c.next.RoundTrip(r)
}

// Wrap returns a transport that counts every request it sends.
func Wrap(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = base
	}
	return counting{next}
}

// Note counts one request to host, unless it is this computer.
func Note(host string) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || local(host) {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	e := hosts[host]
	if e == nil {
		if len(hosts) >= maxHosts {
			return
		}
		e = &Entry{Host: host}
		if how, ok := core.KnownHost(host); ok {
			e.How, e.Label = how, labelOf(host)
		}
		hosts[host] = e
	}
	e.Count++
	e.Last = now().UnixMilli()
}

func local(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func labelOf(host string) string {
	under := func(h string) bool { return host == h || strings.HasSuffix(host, "."+h) }
	for _, s := range core.Sources {
		if under(s.Host) {
			return s.Label
		}
	}
	for _, h := range core.Hosts {
		if under(h.Host) {
			return h.Service
		}
	}
	return ""
}

// List returns the hosts, the most recent first.
func List() []Entry {
	mu.Lock()
	out := make([]Entry, 0, len(hosts))
	for _, e := range hosts {
		out = append(out, *e)
	}
	mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Last != out[j].Last {
			return out[i].Last > out[j].Last
		}
		return out[i].Host < out[j].Host
	})
	return out
}

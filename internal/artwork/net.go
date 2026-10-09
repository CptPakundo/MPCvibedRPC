package artwork

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Every provider call goes through getJSON: responses are cached and shared between identical in-flight requests,
// a timeout / 5xx / 429 is retried once, requests to one host are spaced out, and a host that keeps failing is left
// alone for a minute (so rapid re-testing doesn't hammer a struggling service).

const (
	respTTL    = 10 * time.Minute
	breakAfter = 3
	breakFor   = time.Minute
	cacheMax   = 400
)

// FetchError describes a failed request the way the JavaScript errors did.
type FetchError struct {
	Name       string // "TimeoutError", "TypeError", "AbortError", "Error"
	Msg        string
	Status     int
	RetryAfter int // seconds
}

func (e *FetchError) Error() string { return e.Msg }

// MockError lets a test transport raise an error with a given name (see FetchError).
type MockError struct{ Name, Msg string }

func (e *MockError) Error() string { return e.Msg }

func transient(err error) bool {
	var fe *FetchError
	if !errors.As(err, &fe) {
		return false
	}
	return fe.Name == "TimeoutError" || (fe.Name == "TypeError" && fe.Msg == "fetch failed") || fe.Name == "AbortError" ||
		fe.Status == 429 || fe.Status >= 500
}

type reqInit struct {
	Method           string
	Headers          map[string]string
	Body             string
	TimeoutMs        int
	NoRetryOnTimeout bool // retryTimeout: false
}

type flight struct {
	done chan struct{}
	val  any
	err  error
}

type cacheEntry struct {
	val   any
	until time.Time
}

type failState struct {
	n     int
	until time.Time
}

// Net is the shared HTTP layer of one Artwork instance.
type Net struct {
	Client *http.Client
	gaps   map[string]int
	// blocked, when set, says that a host must not be contacted (a service switched off in the settings).
	blocked func(host string) bool

	mu       sync.Mutex
	cache    map[string]cacheEntry
	order    []string
	inflight map[string]*flight
	fails    map[string]failState
	next     map[string]time.Time
}

// clearCache drops the cached responses (not the failure breaker or the request spacing).
func (n *Net) clearCache() {
	n.mu.Lock()
	n.cache = map[string]cacheEntry{}
	n.order = nil
	n.mu.Unlock()
}

func newNet(client *http.Client, gaps map[string]int) *Net {
	if client == nil {
		client = &http.Client{}
	}
	return &Net{Client: client, gaps: gaps, cache: map[string]cacheEntry{}, inflight: map[string]*flight{}, fails: map[string]failState{}, next: map[string]time.Time{}}
}

func hostOf(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return p.Host
}

func (n *Net) doRequest(ctx context.Context, u string, in reqInit) (*http.Response, error) {
	method := in.Method
	if method == "" {
		method = "GET"
	}
	if n.blocked != nil && n.blocked(hostOf(u)) {
		return nil, &FetchError{Name: "Disabled", Msg: "turned off in the privacy settings"}
	}
	var body io.Reader
	if in.Body != "" {
		body = stringsReader(in.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, &FetchError{Name: "TypeError", Msg: err.Error()}
	}
	// Wikipedia answers 403 to Go's default client string; send a descriptive one unless the caller set its own.
	req.Header.Set("User-Agent", "MPCvibedRPC/1.0 (Discord rich presence for MPC-HC)")
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}
	res, err := n.Client.Do(req)
	if err != nil {
		return nil, classify(ctx, err)
	}
	return res, nil
}

func classify(ctx context.Context, err error) error {
	var me *MockError
	if errors.As(err, &me) {
		return &FetchError{Name: me.Name, Msg: me.Msg}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &FetchError{Name: "TimeoutError", Msg: "The operation was aborted due to timeout"}
	}
	if errors.Is(err, context.Canceled) {
		return &FetchError{Name: "AbortError", Msg: "This operation was aborted"}
	}
	return &FetchError{Name: "TypeError", Msg: "fetch failed"}
}

func (n *Net) reserveGap(host string) {
	gap := n.gaps[host]
	if gap == 0 {
		return
	}
	n.mu.Lock()
	now := time.Now()
	at := n.next[host]
	if at.Before(now) {
		at = now
	}
	n.next[host] = at.Add(time.Duration(gap) * time.Millisecond)
	n.mu.Unlock()
	if d := time.Until(at); d > 0 {
		time.Sleep(d)
	}
}

func (n *Net) fetchOnce(u string, in reqInit) (any, error) {
	n.reserveGap(hostOf(u))
	ms := in.TimeoutMs
	if ms == 0 {
		ms = 3500
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ms)*time.Millisecond)
	defer cancel()
	res, err := n.doRequest(ctx, u, in)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		ra, _ := strconv.Atoi(res.Header.Get("Retry-After"))
		return nil, &FetchError{Name: "Error", Msg: fmt.Sprintf("HTTP %d", res.StatusCode), Status: res.StatusCode, RetryAfter: ra}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, classify(ctx, err)
	}
	v, err := parseJSON(string(b))
	if err != nil {
		return nil, &FetchError{Name: "SyntaxError", Msg: err.Error()}
	}
	return v, nil
}

// getJSON fetches and decodes JSON, with caching, de-duplication, one retry and a per-host circuit breaker.
func (n *Net) getJSON(u string, in reqInit) (any, error) {
	method := in.Method
	if method == "" {
		method = "GET"
	}
	key := method + " " + u + " " + in.Body
	n.mu.Lock()
	if hit, ok := n.cache[key]; ok && hit.until.After(time.Now()) {
		n.mu.Unlock()
		return hit.val, nil
	}
	if f, ok := n.inflight[key]; ok {
		n.mu.Unlock()
		<-f.done
		return f.val, f.err
	}
	host := hostOf(u)
	if f, ok := n.fails[host]; ok && f.n >= breakAfter && f.until.After(time.Now()) {
		n.mu.Unlock()
		return nil, errors.New(host + " is failing; paused")
	}
	fl := &flight{done: make(chan struct{})}
	n.inflight[key] = fl
	n.mu.Unlock()

	val, err := func() (any, error) {
		v, err := n.fetchOnce(u, in)
		if err != nil {
			var fe *FetchError
			errors.As(err, &fe)
			if !transient(err) || (in.NoRetryOnTimeout && fe.Status == 0) {
				return nil, err
			}
			wait := 400 * time.Millisecond
			if fe.RetryAfter > 0 {
				wait = time.Duration(fe.RetryAfter) * time.Second
			}
			if wait > 2*time.Second {
				wait = 2 * time.Second
			}
			time.Sleep(wait)
			v, err = n.fetchOnce(u, in)
			if err != nil {
				return nil, err
			}
		}
		return v, nil
	}()

	n.mu.Lock()
	if err == nil {
		delete(n.fails, host)
		if len(n.cache) >= cacheMax && len(n.order) > 0 {
			delete(n.cache, n.order[0])
			n.order = n.order[1:]
		}
		if _, exists := n.cache[key]; !exists {
			n.order = append(n.order, key)
		}
		n.cache[key] = cacheEntry{val, time.Now().Add(respTTL)}
	} else if transient(err) {
		c := n.fails[host]
		n.fails[host] = failState{c.n + 1, time.Now().Add(breakFor)}
	}
	delete(n.inflight, key)
	fl.val, fl.err = val, err
	n.mu.Unlock()
	close(fl.done)
	return val, err
}

// fetchText is a plain GET that returns the body text (used for the wiki pages).
func (n *Net) fetchText(u string, headers map[string]string, timeout time.Duration) (status int, text string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err := n.doRequest(ctx, u, reqInit{Headers: headers})
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return 0, "", classify(ctx, err)
	}
	return res.StatusCode, string(b), nil
}

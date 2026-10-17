package artwork

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A catalog that answers only after the request's timeout (TVmaze's search of a new query): with LateMs the caller
// gets a pending error at once, the request goes on, its answer is cached and onLate fires; the breaker never trips.
func TestLateAnswerIsCached(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		w.Write([]byte(`[{"show":{"name":"Sample Show"}}]`))
	}))
	defer srv.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()

	n := newNet(nil, nil)
	late := make(chan struct{}, 1)
	n.onLate = func() { late <- struct{}{} }
	u := srv.URL + "/search/shows?q=sample"
	in := reqInit{TimeoutMs: 50, LateMs: 5000}

	start := time.Now()
	if _, err := n.getJSON(u, in); !pending(err) || !unavailable(err) {
		t.Fatalf("first ask: want a pending error, got %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("first ask took %v; it must give up at its timeout", d)
	}
	for i := 0; i < breakAfter+1; i++ { // the same request again while it is still going on: no new request, no pause
		if _, err := n.getJSON(u, in); !pending(err) {
			t.Fatalf("ask %d while waiting: want a pending error, got %v", i+2, err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server asked %d times while the first request was still going on; want 1", got)
	}
	close(release)
	select {
	case <-late:
	case <-time.After(5 * time.Second):
		t.Fatal("onLate was not called after the late answer")
	}
	v, err := n.getJSON(u, in)
	if err != nil {
		t.Fatalf("after the late answer: %v", err)
	}
	if arr := asArr(v); len(arr) != 1 || str(get(arr[0], "show", "name")) != "Sample Show" {
		t.Fatalf("cached answer = %v", v)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server asked %d times; the late answer should have been cached", got)
	}
}

// A request without LateMs keeps the old behaviour: timeout, one retry, counted by the breaker.
func TestTimeoutWithoutLateMsIsRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(300 * time.Millisecond)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	n := newNet(nil, nil)
	n.onLate = func() { t.Error("onLate called for a request without LateMs") }
	_, err := n.getJSON(srv.URL+"/x", reqInit{TimeoutMs: 50})
	if err == nil || pending(err) || !transient(err) {
		t.Fatalf("want a plain timeout, got %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("server asked %d times; want 2 (one retry)", got)
	}
	if n.fails[hostOf(srv.URL)].n != 1 {
		t.Fatalf("breaker count = %d; want 1", n.fails[hostOf(srv.URL)].n)
	}
}

// A late answer that turns out to be a failure is counted by the breaker then, and onLate is not called.
func TestLateFailureCountsOnce(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	n := newNet(nil, nil)
	n.onLate = func() { t.Error("onLate called for a failed late answer") }
	u := srv.URL + "/search/shows?q=sample"
	if _, err := n.getJSON(u, reqInit{TimeoutMs: 50, LateMs: 5000}); !pending(err) {
		t.Fatalf("want a pending error, got %v", err)
	}
	n.mu.Lock()
	sf := n.slow[u2key(u)]
	n.mu.Unlock()
	if sf == nil {
		t.Fatal("no background request recorded")
	}
	close(release)
	<-sf.done
	if !transient(sf.err) {
		t.Fatalf("late result = %v; want the 502", sf.err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.fails[hostOf(u)].n != 1 {
		t.Fatalf("breaker count = %d; want 1", n.fails[hostOf(u)].n)
	}
	if _, ok := n.cache[u2key(u)]; ok {
		t.Fatal("a failure was cached")
	}
}

func u2key(u string) string { return "GET " + u + " " }

// Titles that missed because a service was slow are asked again once a late answer is in; real misses are not.
func TestLateAnswerClearsSoftMisses(t *testing.T) {
	a := &Artwork{misses: map[string]time.Time{}, soft: map[string]bool{}}
	a.softMiss("slow")
	a.softMiss("again")
	a.misses["again"] = time.Now().Add(30 * time.Minute) // later found to be a real miss...
	delete(a.soft, "again")                              // ...as baseArt records it
	a.misses["none"] = time.Now().Add(30 * time.Minute)
	if !a.missed("slow") {
		t.Fatal("a soft miss must hold until a late answer")
	}
	a.lateAnswer()
	if a.missed("slow") {
		t.Fatal("the soft miss was not cleared")
	}
	if !a.missed("again") || !a.missed("none") {
		t.Fatal("a real miss was cleared")
	}
	if len(a.soft) != 0 {
		t.Fatalf("soft = %v; want empty", a.soft)
	}
}

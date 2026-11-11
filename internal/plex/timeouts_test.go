package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A server that takes the connection and never sends headers must not hold the watcher: the event stream has no
// overall time limit, so the silence limit has to cover the wait for the headers too.
func TestStalledStreamConnectGivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/identity") {
			rw.Header().Set("Content-Type", "application/json")
			rw.Write([]byte(`{"MediaContainer":{"machineIdentifier":""}}`))
			return
		}
		select { // the notifications: accepted, never answered
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()
	w := &Watcher{Client: &Client{ClientID: "test-client"}, Address: srv.URL}
	w.limits.stream = 300 * time.Millisecond
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- w.follow(context.Background()) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "notifications") {
			t.Fatalf("got %v; want the notifications error", err)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("gave up after %v", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher is stuck waiting for the stream's headers")
	}
}

// Data lines with no blank line after them must not be collected without limit: the stream ends with an error, and
// the caller reconnects.
func TestOversizedEventEndsTheStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/identity") {
			rw.Header().Set("Content-Type", "application/json")
			rw.Write([]byte(`{"MediaContainer":{"machineIdentifier":""}}`))
			return
		}
		rw.Header().Set("Content-Type", "text/event-stream")
		rw.Write([]byte("event: playing\n"))
		line := "data: " + strings.Repeat("x", 60<<10) + "\n"
		for i := 0; i < 40 && r.Context().Err() == nil; i++ { // 2.4 MiB, never a blank line
			if _, err := rw.Write([]byte(line)); err != nil {
				return
			}
			rw.(http.Flusher).Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	w := &Watcher{Client: &Client{ClientID: "test-client"}, Address: srv.URL}
	done := make(chan error, 1)
	go func() { done <- w.follow(context.Background()) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("got %v; want the too-large error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher keeps collecting an event that never ends")
	}
}

// plex.tv's server list is asked under a time limit of its own.
func TestServersLookupTimesOut(t *testing.T) {
	tv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer tv.Close()
	w := &Watcher{Client: &Client{ClientID: "test-client", TVBase: tv.URL, ResourcesBase: tv.URL}, Token: "account-token"}
	w.limits.lookup = 200 * time.Millisecond
	start := time.Now()
	_, err := w.find(context.Background())
	if err == nil || !strings.Contains(err.Error(), "plex.tv") {
		t.Fatalf("got %v; want the plex.tv error", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("gave up after %v", d)
	}
}

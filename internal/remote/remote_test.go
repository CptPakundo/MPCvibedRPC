package remote

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientKeepsTheDefaultTransport(t *testing.T) {
	c := NewClient(3 * time.Second)
	if c.Transport != nil {
		t.Error("the client must use http.DefaultTransport as it is at request time (internal/conns wraps it)")
	}
	if c.Timeout != 3*time.Second {
		t.Errorf("timeout %v", c.Timeout)
	}
}

// A redirect to another host is refused, also for a POST that carries a key (307 sends the body again).
func TestClientRefusesRedirectToAnotherHost(t *testing.T) {
	var other int
	elsewhere := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { other++ }))
	defer elsewhere.Close()
	home := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inside" {
			rw.Write([]byte("ok"))
			return
		}
		http.Redirect(rw, r, elsewhere.URL+"/x", http.StatusTemporaryRedirect)
	}))
	defer home.Close()
	c := NewClient(0)

	req, _ := http.NewRequest("POST", home.URL+"/out", strings.NewReader(`{"authKey":"secret"}`))
	req.Header.Set("X-Plex-Token", "secret")
	if res, _, err := Fetch(context.Background(), c, req, 1<<10); err == nil || res != nil {
		t.Fatalf("followed the redirect: %v", err)
	}
	if other != 0 {
		t.Errorf("the other host was asked %d times", other)
	}

	// a redirect inside the same host is still followed
	in := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(rw, r, "/end", http.StatusFound)
			return
		}
		rw.Write([]byte("arrived"))
	}))
	defer in.Close()
	req, _ = http.NewRequest("GET", in.URL+"/start", nil)
	res, body, err := Fetch(context.Background(), c, req, 1<<10)
	if err != nil || res.StatusCode != 200 || string(body) != "arrived" {
		t.Fatalf("same-host redirect: %v %v %q", res, err, body)
	}
}

func TestFetchLimitsTheBodyAndHonoursTheContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hang" {
			<-r.Context().Done()
			return
		}
		io.WriteString(rw, strings.Repeat("a", 100))
	}))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	res, body, err := Fetch(context.Background(), NewClient(0), req, 10)
	if err != nil || res.StatusCode != 200 || len(body) != 10 {
		t.Fatalf("got %v %v %d bytes", res, err, len(body))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req, _ = http.NewRequest("GET", srv.URL+"/hang", nil)
	if res, _, err := Fetch(ctx, NewClient(0), req, 10); err == nil || res != nil {
		t.Fatalf("a hanging server was waited for: %v", err)
	}
}

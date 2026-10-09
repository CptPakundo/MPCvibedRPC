package server

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
)

type valErr struct{}

func (valErr) Error() string      { return "nope" }
func (valErr) IsValidation() bool { return true }

func start(t *testing.T) (*Server, string) {
	s := New(Options{Token: "tok", Page: "<html>__TOKEN__</html>", Icon: []byte("ICO"), Version: "9.9.9", Handlers: map[string]Handler{
		"status": {Get: true, Fn: func(*jsonx.Obj) (any, error) { return map[string]int{"a": 1}, nil }},
		"echo":   {Fn: func(b *jsonx.Obj) (any, error) { return map[string]any{"got": b.M["x"]}, nil }},
		"nil":    {Fn: func(*jsonx.Obj) (any, error) { return nil, nil }},
		"bad":    {Fn: func(*jsonx.Obj) (any, error) { return nil, valErr{} }},
		"boom":   {Fn: func(*jsonx.Obj) (any, error) { return nil, errors.New("kaput") }},
		"panic":  {Fn: func(*jsonx.Obj) (any, error) { panic("oops") }},
	}})
	if _, err := s.Listen(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, "http://127.0.0.1:" + itoa(s.Port())
}

func itoa(n int) string { return strconv.Itoa(n) }

func do(t *testing.T, method, url, body string, hdr map[string]string, host string) (int, string, http.Header) {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if host != "" {
		req.Host = host
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), res.Header
}

func TestServer(t *testing.T) {
	s, base := start(t)
	base = "http://127.0.0.1:" + strings.TrimPrefix(base, "http://127.0.0.1:")
	_ = s
	tok := map[string]string{"X-Token": "tok"}

	if c, b, h := do(t, "GET", base+"/api/ping", "", nil, ""); c != 200 || b != `{"app":"MPCvibedRPC","version":"9.9.9"}` || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Security-Policy") == "" {
		t.Fatal(c, b)
	}
	if c, b, _ := do(t, "GET", base+"/icon.ico", "", nil, ""); c != 200 || b != "ICO" {
		t.Fatal(c, b)
	}
	if c, b, _ := do(t, "GET", base+"/?t=tok", "", nil, ""); c != 200 || b != "<html>tok</html>" {
		t.Fatal(c, b)
	}
	if c, _, _ := do(t, "GET", base+"/", "", nil, ""); c != 403 {
		t.Fatal(c)
	}
	if c, _, _ := do(t, "GET", base+"/nothing", "", nil, ""); c != 404 {
		t.Fatal(c)
	}
	// host check (DNS rebinding)
	if c, _, _ := do(t, "GET", base+"/api/ping", "", nil, "evil.example"); c != 403 {
		t.Fatal(c)
	}
	port := base[strings.LastIndex(base, ":")+1:]
	if c, _, _ := do(t, "GET", base+"/api/ping", "", nil, "localhost:"+port); c != 200 {
		t.Fatal(c)
	}
	// auth
	if c, _, _ := do(t, "GET", base+"/api/status", "", nil, ""); c != 401 {
		t.Fatal(c)
	}
	if c, _, _ := do(t, "GET", base+"/api/status", "", map[string]string{"X-Token": "wrong"}, ""); c != 401 {
		t.Fatal(c)
	}
	if c, b, _ := do(t, "GET", base+"/api/status", "", tok, ""); c != 200 || b != `{"a":1}` {
		t.Fatal(c, b)
	}
	// method must match
	if c, _, _ := do(t, "POST", base+"/api/status", "{}", tok, ""); c != 404 {
		t.Fatal(c)
	}
	if c, _, _ := do(t, "GET", base+"/api/echo", "", tok, ""); c != 404 {
		t.Fatal(c)
	}
	if c, _, _ := do(t, "POST", base+"/api/unknown", "{}", tok, ""); c != 404 {
		t.Fatal(c)
	}
	if c, b, _ := do(t, "POST", base+"/api/echo", `{"x":[1,"a"]}`, tok, ""); c != 200 || b != `{"got":[1,"a"]}` {
		t.Fatal(c, b)
	}
	if c, b, _ := do(t, "POST", base+"/api/nil", "", tok, ""); c != 200 || b != `{"ok":true}` {
		t.Fatal(c, b)
	}
	if c, _, _ := do(t, "POST", base+"/api/echo", `{bad`, tok, ""); c != 400 {
		t.Fatal(c)
	}
	if c, b, _ := do(t, "POST", base+"/api/bad", `{}`, tok, ""); c != 400 || b != `{"error":"nope"}` {
		t.Fatal(c, b)
	}
	if c, b, _ := do(t, "POST", base+"/api/boom", `{}`, tok, ""); c != 500 || b != `{"error":"kaput"}` {
		t.Fatal(c, b)
	}
	if c, b, _ := do(t, "POST", base+"/api/panic", `{}`, tok, ""); c != 500 || !strings.Contains(b, "oops") {
		t.Fatal(c, b)
	}
	if c, _, _ := do(t, "POST", base+"/api/echo", strings.Repeat("a", 70000), tok, ""); c != 413 {
		t.Fatal(c)
	}
}

func TestAliveEndsWhenServerCloses(t *testing.T) {
	s, base := start(t)
	req, _ := http.NewRequest("GET", base+"/api/alive", nil)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != 401 {
		t.Fatalf("no token: %v %v", res, err)
	}
	req.Header.Set("X-Token", "tok")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("alive: %v %v", res, err)
	}
	defer res.Body.Close()
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, res.Body); close(done) }() // blocks while the program runs
	select {
	case <-done:
		t.Fatal("stream ended before the server closed")
	case <-time.After(300 * time.Millisecond):
	}
	s.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end when the server closed")
	}
}

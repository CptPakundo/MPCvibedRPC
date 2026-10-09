// Package server is the local control surface: a tiny HTTP server on 127.0.0.1 that serves the settings page and
// its API. It is guarded by a random per-launch token (header X-Token, or ?t= for the page itself) and a Host check,
// so no website can drive it.
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/jsonx"
)

// NewToken makes a random token.
func NewToken() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Handler is one API action. Get handlers answer GET, the others POST. A nil result is answered as {"ok":true}.
type Handler struct {
	Get bool
	Fn  func(body *jsonx.Obj) (any, error)
}

// Options configures a Server.
type Options struct {
	Token    string
	Page     string // the settings page; __TOKEN__ is replaced by the token
	Icon     []byte
	Handlers map[string]Handler
	Version  string
}

// Server serves the settings window and API.
type Server struct {
	o    Options
	port int
	srv  *http.Server
}

// Validation is implemented by errors that should be answered as a 400.
type Validation interface{ IsValidation() bool }

// Cover images for the preview card come straight from the catalogs' image hosts (https only, no referrer).
const csp = "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src 'self' data: https:"

func New(o Options) *Server { return &Server{o: o} }

// Port is the port Listen settled on.
func (s *Server) Port() int { return s.port }

func send(w http.ResponseWriter, code int, body any, ctype string) {
	var data []byte
	switch b := body.(type) {
	case string:
		data = []byte(b)
	case []byte:
		data = b
	default:
		data, _ = json.Marshal(body)
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", csp)
	w.WriteHeader(code)
	w.Write(data)
}

const jsonType = "application/json; charset=utf-8"

func (s *Server) hostOK(r *http.Request) bool {
	h := strings.ToLower(r.Host)
	p := strconv.Itoa(s.port)
	return h == "127.0.0.1:"+p || h == "localhost:"+p
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.hostOK(r) {
		send(w, 403, map[string]string{"error": "Forbidden"}, jsonType)
		return
	}
	p := r.URL.Path
	switch {
	case r.Method == "GET" && p == "/api/ping":
		send(w, 200, map[string]string{"app": "MPCvibedRPC", "version": s.o.Version}, jsonType)
		return
	case r.Method == "GET" && p == "/icon.ico":
		send(w, 200, s.o.Icon, "image/x-icon")
		return
	case r.Method == "GET" && p == "/":
		if r.URL.Query().Get("t") != s.o.Token {
			send(w, 403, "<h3>Open MPCvibedRPC from its tray icon or by launching the program again.</h3>", "text/html; charset=utf-8")
			return
		}
		send(w, 200, strings.Replace(s.o.Page, "__TOKEN__", s.o.Token, 1), "text/html; charset=utf-8")
		return
	}
	if !strings.HasPrefix(p, "/api/") {
		send(w, 404, map[string]string{"error": "Not found"}, jsonType)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Token")), []byte(s.o.Token)) != 1 {
		send(w, 401, map[string]string{"error": "Unauthorized"}, jsonType)
		return
	}
	if r.Method == "GET" && p == "/api/alive" {
		// Held open for as long as the program runs; the page closes itself when this connection ends.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(200)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		<-r.Context().Done()
		return
	}
	h, ok := s.o.Handlers[p[5:]]
	if !ok || (r.Method == "GET") != h.Get {
		send(w, 404, map[string]string{"error": "Unknown action"}, jsonType)
		return
	}
	body := jsonx.NewObj()
	if r.Method == "POST" {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
		if err != nil {
			send(w, 413, map[string]string{"error": "Request too large"}, jsonType)
			return
		}
		if len(raw) > 0 {
			v, err := jsonx.Parse(string(raw))
			if err != nil {
				send(w, 400, map[string]string{"error": "Bad JSON"}, jsonType)
				return
			}
			if o, ok := v.(*jsonx.Obj); ok {
				body = o
			}
		}
	}
	res, err := func() (res any, err error) {
		defer func() {
			if p := recover(); p != nil {
				err = errors.New(toString(p))
			}
		}()
		return h.Fn(body)
	}()
	if err != nil {
		code := 500
		var v Validation
		if errors.As(err, &v) && v.IsValidation() {
			code = 400
		}
		send(w, code, map[string]string{"error": err.Error()}, jsonType)
		return
	}
	if res == nil {
		res = map[string]bool{"ok": true}
	}
	send(w, 200, res, jsonType)
}

func toString(v any) string {
	switch x := v.(type) {
	case error:
		return x.Error()
	case string:
		return x
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// Listen binds 127.0.0.1:preferred, or any free port when that one is taken, and serves in the background.
func (s *Server) Listen(preferred int) (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(preferred))
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.srv = &http.Server{Handler: s}
	go s.srv.Serve(ln)
	return s.port, nil
}

// Close stops the server.
func (s *Server) Close() {
	if s.srv != nil {
		s.srv.Close()
	}
}

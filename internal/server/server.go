// Package server is the local control surface: a tiny HTTP server on 127.0.0.1 that serves the settings page and
// its API. It is guarded by a random per-launch token (header X-Token) and a Host check, so no website can drive it.
// The page, its style sheet and its script are public files: they hold nothing secret. The page hands the token to
// its script in a meta tag when it is asked for with a one-time code (?c=, what the program opens its window with) or
// with the token itself (?t=, for tests and development); without either the script has no token and says so.
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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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
	Page     string // the settings page; __TOKEN__ is replaced by the token (it sits in a meta tag the script reads)
	CSS      string // the page's style sheet, served as /ui.css
	JS       string // the page's script, served as /ui.js
	Icon     []byte
	Handlers map[string]Handler
	Version  string
	// OnWindow is told when a settings window opens (true) and closes (false): each open window holds /api/alive.
	OnWindow func(open bool)
}

// Server serves the settings window and API.
type Server struct {
	o    Options
	port int
	srv  *http.Server

	mu    sync.Mutex
	codes map[string]time.Time // one-time codes for the page and when each stops being good (NewCode)
}

// codeLife is how long a one-time code is good for: the time a browser may take to start and ask for the page.
const codeLife = 2 * time.Minute

// NewCode makes a code that opens the page with its token once. The program starts the browser with it instead of
// the token: a browser's command line can be read by other users of the computer (ps), and the token is good for as
// long as the program runs, while a code is spent by the browser's first request.
func (s *Server) NewCode() string {
	code := NewToken()
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.codes == nil {
		s.codes = map[string]time.Time{}
	}
	for c, end := range s.codes {
		if now.After(end) {
			delete(s.codes, c)
		}
	}
	s.codes[code] = now.Add(codeLife)
	return code
}

// spend reports whether code is a good one-time code, and uses it up.
func (s *Server) spend(code string) bool {
	if code == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	end, ok := s.codes[code]
	delete(s.codes, code)
	return ok && time.Now().Before(end)
}

// Validation is implemented by errors that should be answered as a 400.
type Validation interface{ IsValidation() bool }

// Cover images for the preview card come straight from the catalogs' image hosts (https only, no referrer).
// The page's own style sheet and script only: no inline ones.
const csp = "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data: https:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

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
		send(w, 200, map[string]any{"app": "MPCvibedRPC", "version": s.o.Version, "pid": os.Getpid()}, jsonType)
		return
	case r.Method == "GET" && p == "/icon.ico":
		send(w, 200, s.o.Icon, "image/x-icon")
		return
	case r.Method == "GET" && p == "/ui.css":
		send(w, 200, s.o.CSS, "text/css; charset=utf-8")
		return
	case r.Method == "GET" && p == "/ui.js":
		send(w, 200, s.o.JS, "text/javascript; charset=utf-8")
		return
	case r.Method == "GET" && p == "/":
		q := r.URL.Query()
		token := ""
		if s.spend(q.Get("c")) || subtle.ConstantTimeCompare([]byte(q.Get("t")), []byte(s.o.Token)) == 1 {
			token = s.o.Token
		}
		// Without a code the page still comes, with no token in it: a window that reloads has kept its token (the
		// script takes the code out of its address), and anyone else is told how to open the window.
		send(w, 200, strings.Replace(s.o.Page, "__TOKEN__", token, 1), "text/html; charset=utf-8")
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
		if s.o.OnWindow != nil {
			s.o.OnWindow(true)
			defer s.o.OnWindow(false)
		}
		<-r.Context().Done()
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		send(w, 405, map[string]string{"error": "Method not allowed"}, jsonType)
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

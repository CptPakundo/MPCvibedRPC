package vlchttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeVLC serves status.json and playlist.json like VLC's web interface does, behind its password check.
type fakeVLC struct {
	mu       sync.Mutex
	password string
	status   string
	playlist string
	plReads  int
	srv      *httptest.Server
}

func newFakeVLC(t *testing.T, password, status, playlist string) *fakeVLC {
	f := &fakeVLC{password: password, status: status, playlist: playlist}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.password == "" {
			w.WriteHeader(http.StatusForbidden) // VLC without a password: "Password unset, insecure web interface disabled"
			return
		}
		user, pw, ok := r.BasicAuth()
		if !ok || user != "" || pw != f.password {
			w.Header().Set("WWW-Authenticate", `Basic realm="VLC stream"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.RawQuery != "" {
			t.Errorf("a command was sent to VLC: %s", r.URL.RawQuery)
		}
		switch r.URL.Path {
		case "/requests/status.json":
			_, _ = w.Write([]byte(f.status))
		case "/requests/playlist.json":
			f.plReads++
			_, _ = w.Write([]byte(f.playlist))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeVLC) port() int { return f.srv.Listener.Addr().(*net.TCPAddr).Port }

func (f *fakeVLC) set(status, playlist string) {
	f.mu.Lock()
	f.status, f.playlist = status, playlist
	f.mu.Unlock()
}

func (f *fakeVLC) reads() int { f.mu.Lock(); defer f.mu.Unlock(); return f.plReads }

func read(t *testing.T, name string) string {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Recorded from VLC 3.0.24 playing a local file (the folder was renamed).
func TestRecordedPlaying(t *testing.T) {
	f := newFakeVLC(t, "pw", read(t, "vlc3-status-playing.json"), read(t, "vlc3-playlist.json"))
	r := &Reader{}
	in, err := r.Query(context.Background(), f.port(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	if in.File != "Sample Show S01E02.mkv" || in.FullDir != `D:\Videos\Sample Show\Season 1` || in.State != 2 {
		t.Errorf("got %+v", in)
	}
	if len(in.Dir) < 2 || in.Dir[0] != "Season 1" || in.Dir[1] != "Sample Show" {
		t.Errorf("folders, nearest first: %q", in.Dir)
	}
	// position is the fraction times the length (8.49 s), finer than VLC 3's whole-second "time" (8)
	if in.Position != 8496 || in.Duration != 1499000 || in.Rate != 1 {
		t.Errorf("position %d, duration %d, rate %v", in.Position, in.Duration, in.Rate)
	}
	// the playlist is only read again when the item changes
	for i := 0; i < 3; i++ {
		if _, err := r.Query(context.Background(), f.port(), "pw"); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.reads(); n != 1 {
		t.Errorf("playlist read %d times, want 1", n)
	}
	other := strings.Replace(read(t, "vlc3-status-playing.json"), `"currentplid":3`, `"currentplid":4`, 1)
	f.set(other, strings.Replace(read(t, "vlc3-playlist.json"), `"id":"3"`, `"id":"4"`, 1))
	if _, err := r.Query(context.Background(), f.port(), "pw"); err != nil {
		t.Fatal(err)
	}
	if n := f.reads(); n != 2 {
		t.Errorf("a new item should read the playlist again (%d reads)", n)
	}
}

func TestRecordedPausedAndStopped(t *testing.T) {
	paused := strings.Replace(read(t, "vlc3-status-playing.json"), `"state":"playing"`, `"state":"paused"`, 1)
	f := newFakeVLC(t, "pw", paused, read(t, "vlc3-playlist.json"))
	r := &Reader{}
	in, err := r.Query(context.Background(), f.port(), "pw")
	if err != nil || in.State != 1 || in.File != "Sample Show S01E02.mkv" {
		t.Fatalf("paused: %+v %v", in, err)
	}
	f.set(read(t, "vlc3-status-stopped.json"), read(t, "vlc3-playlist.json"))
	in, err = r.Query(context.Background(), f.port(), "pw")
	if err != nil || in.State != 0 || in.File != "" {
		t.Fatalf("stopped: %+v %v", in, err)
	}
}

func TestSpeed(t *testing.T) {
	st := strings.Replace(read(t, "vlc3-status-playing.json"), `"rate":1,`, `"rate":1.5015015602112,`, 1)
	f := newFakeVLC(t, "pw", st, read(t, "vlc3-playlist.json"))
	in, err := (&Reader{}).Query(context.Background(), f.port(), "pw")
	if err != nil || in.Rate < 1.5 || in.Rate > 1.51 {
		t.Fatalf("rate: %+v %v", in, err)
	}
}

// A network stream: the title is shown, there is no folder.
func TestStream(t *testing.T) {
	st := `{"apiversion":3,"state":"playing","time":12,"length":0,"position":0,"rate":1,"currentplid":5,
		"information":{"category":{"meta":{"filename":"live","title":"Sample Channel","now_playing":"Sample Show"}}}}`
	pl := `{"type":"node","id":"0","children":[{"type":"node","id":"1","name":"Playlist","children":[
		{"type":"leaf","id":"5","name":"live","uri":"https://example.com/live","current":"current","duration":-1}]}]}`
	f := newFakeVLC(t, "pw", st, pl)
	in, err := (&Reader{}).Query(context.Background(), f.port(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	if in.File != "Sample Channel" || in.FullDir != "" || len(in.Dir) != 0 || in.Position != 12000 || in.Duration != 0 {
		t.Errorf("got %+v", in)
	}
}

// VLC 4 (development builds): "time" has a fraction and playlist.json is a flat list.
func TestVLC4(t *testing.T) {
	st := `{"apiversion":4,"state":"playing","time":61.25,"length":1500,"position":0,"rate":1,"currentplid":7,
		"information":{"category":{"meta":{"filename":"Sample Movie (2020).mkv","title":"Sample Movie"}}}}`
	pl := `[{"type":"leaf","id":"6","name":"other","uri":"file:///D:/Videos/Other.mkv","duration":10},
		{"type":"leaf","id":"7","name":"Sample Movie","uri":"file:///D:/Videos/Movies/Sample%20Movie%20(2020).mkv","current":"current","duration":1500}]`
	f := newFakeVLC(t, "pw", st, pl)
	in, err := (&Reader{}).Query(context.Background(), f.port(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	if in.File != "Sample Movie (2020).mkv" || in.FullDir != `D:\Videos\Movies` || in.Position != 61250 || in.Duration != 1500000 {
		t.Errorf("got %+v", in)
	}
}

// Without the playlist the file name is still known (no folder).
func TestNoPlaylist(t *testing.T) {
	f := newFakeVLC(t, "pw", read(t, "vlc3-status-playing.json"), "not json")
	in, err := (&Reader{}).Query(context.Background(), f.port(), "pw")
	if err != nil || in.File != "Sample Show S01E02.mkv" || in.FullDir != "" {
		t.Fatalf("got %+v %v", in, err)
	}
}

func TestErrors(t *testing.T) {
	f := newFakeVLC(t, "pw", read(t, "vlc3-status-playing.json"), read(t, "vlc3-playlist.json"))
	if _, err := (&Reader{}).Query(context.Background(), f.port(), "wrong"); !errors.Is(err, ErrPassword) {
		t.Errorf("wrong password: %v", err)
	}
	none := newFakeVLC(t, "", "", "")
	if _, err := (&Reader{}).Query(context.Background(), none.port(), "pw"); !errors.Is(err, ErrNoPassword) {
		t.Errorf("VLC without a password: %v", err)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ok":true}`)) }))
	defer other.Close()
	if _, err := (&Reader{}).Query(context.Background(), other.Listener.Addr().(*net.TCPAddr).Port, "pw"); !errors.Is(err, ErrNotVLC) {
		t.Errorf("something else on the port: %v", err)
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if _, err := (&Reader{}).Query(context.Background(), port, "pw"); err == nil || errors.Is(err, ErrNotVLC) || errors.Is(err, ErrPassword) {
		t.Errorf("nothing on port %s: %v", strconv.Itoa(port), err)
	}
}

func TestLocalPath(t *testing.T) {
	for in, want := range map[string]string{
		"file:///C:/Videos/Sample%20Show/a.mkv":  `C:\Videos\Sample Show\a.mkv`,
		"file://nas/share/Sample%20Movie.mkv":    `\\nas\share\Sample Movie.mkv`,
		"file:///home/user/Videos/Sample%23.mkv": "/home/user/Videos/Sample#.mkv",
		"file://localhost/C:/Videos/b%2Bc.mkv":   `C:\Videos\b+c.mkv`,
		"https://example.com/file:///C:/x.mkv":   "",
		"dvd:///D:/":                             "",
	} {
		got, ok := LocalPath(in)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v, want %q", in, got, ok, want)
		}
	}
}

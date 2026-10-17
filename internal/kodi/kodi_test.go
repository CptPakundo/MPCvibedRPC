package kodi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeKodi answers Kodi's JSON-RPC methods with answers shaped like Kodi's (JSON-RPC API v12/v13).
func fakeKodi(t *testing.T, players, item string, speed int) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "kodi" || p != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/jsonrpc" || r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(400)
			return
		}
		var result string
		switch req.Method {
		case "Player.GetActivePlayers":
			result = players
		case "Player.GetItem":
			result = `{"item":` + item + `}`
		case "Player.GetProperties":
			b, _ := json.Marshal(map[string]any{"speed": speed,
				"time":      map[string]int{"hours": 0, "minutes": 1, "seconds": 2, "milliseconds": 500},
				"totaltime": map[string]int{"hours": 0, "minutes": 24, "seconds": 0, "milliseconds": 0}})
			result = string(b)
		default:
			w.Write([]byte(`{"id":1,"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found."}}`))
			return
		}
		w.Write([]byte(`{"id":1,"jsonrpc":"2.0","result":` + result + `}`))
	}))
	t.Cleanup(s.Close)
	return s
}

const videoPlayer = `[{"playerid":1,"playertype":"internal","type":"video"}]`

func TestQuery(t *testing.T) {
	for _, c := range []struct {
		name, players, item string
		speed               int
		file, dir           string
		state               int
		where               string
	}{
		{"a library episode", videoPlayer,
			`{"id":7,"type":"episode","label":"Episode title","title":"Episode title","showtitle":"Sample Show","season":1,"episode":2,"year":2020,"file":"smb://nas/shows/Sample Show/S01E02.mkv"}`,
			1, "Sample Show - S01E02 - Episode title", "", 2, "smb://nas/shows/Sample Show/S01E02.mkv"},
		{"a library movie, paused", videoPlayer,
			`{"id":3,"type":"movie","label":"Sample Movie","title":"Sample Movie","showtitle":"","season":-1,"episode":-1,"year":2020,"file":"/storage/movies/Sample.Movie.2020.mkv"}`,
			0, "Sample Movie (2020)", "", 1, "/storage/movies/Sample.Movie.2020.mkv"},
		{"a file outside the library", videoPlayer,
			`{"type":"unknown","label":"Sample.Show.S01E02.mkv","title":"","showtitle":"","season":-1,"episode":-1,"year":0,"file":"D:\\Videos\\Sample Show\\Sample.Show.S01E02.mkv"}`,
			1, "Sample.Show.S01E02.mkv", `D:\Videos\Sample Show`, 2, `D:\Videos\Sample Show\Sample.Show.S01E02.mkv`},
		{"a song", `[{"playerid":0,"playertype":"internal","type":"audio"}]`,
			`{"type":"song","label":"Sample Song","title":"Sample Song","file":"/music/01 Sample Artist - Sample Song.flac"}`,
			1, "01 Sample Artist - Sample Song.flac", "/music", 2, "/music/01 Sample Artist - Sample Song.flac"},
		{"nothing playing", `[]`, `{}`, 1, "", "", 0, ""},
	} {
		s := fakeKodi(t, c.players, c.item, c.speed)
		in, err := (&Reader{}).Query(context.Background(), s.URL, "kodi", "secret")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		where := ""
		if len(in.Where) > 0 {
			where = in.Where[0]
		}
		if in.File != c.file || in.FullDir != c.dir || in.State != c.state || where != c.where {
			t.Errorf("%s: %+v", c.name, in)
		}
		if c.state > 0 && (in.Position != 62500 || in.Duration != 1440000) {
			t.Errorf("%s: position %d of %d", c.name, in.Position, in.Duration)
		}
	}
}

func TestQueryProblems(t *testing.T) {
	s := fakeKodi(t, videoPlayer, `{}`, 1)
	if _, err := (&Reader{}).Query(context.Background(), s.URL, "kodi", "wrong"); !errors.Is(err, ErrPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	notKodi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>hello</html>")) }))
	defer notKodi.Close()
	if _, err := (&Reader{}).Query(context.Background(), notKodi.URL, "kodi", "secret"); !errors.Is(err, ErrNotKodi) {
		t.Fatalf("not Kodi: %v", err)
	}
}

func TestBase(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:8080":                 "http://127.0.0.1:8080",
		" http://192.168.1.30:8080/ ":    "http://192.168.1.30:8080",
		"http://kodi.local:8080/jsonrpc": "http://kodi.local:8080",
	} {
		if got := Base(in); got != want {
			t.Errorf("Base(%q) = %q, want %q", in, got, want)
		}
	}
}

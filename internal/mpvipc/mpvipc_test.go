package mpvipc

import (
	"bufio"
	"encoding/json"
	"net"
	"reflect"
	"testing"
	"time"
)

// fakeMpv answers get_property requests from a table, in reverse order and with an event in between, like a busy
// mpv may. A missing property gets mpv's "property unavailable" error.
func fakeMpv(t *testing.T, conn net.Conn, values map[string]any) {
	t.Helper()
	go func() {
		defer conn.Close()
		r := bufio.NewReader(conn)
		var reqs []map[string]any
		for len(reqs) < len(props) {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(line, &m) == nil {
				reqs = append(reqs, m)
			}
		}
		conn.Write([]byte(`{"event":"playback-restart"}` + "\n"))
		for i := len(reqs) - 1; i >= 0; i-- {
			cmd := reqs[i]["command"].([]any)
			out := map[string]any{"request_id": reqs[i]["request_id"], "error": "success"}
			if v, ok := values[cmd[1].(string)]; ok {
				out["data"] = v
			} else {
				out["error"] = "property unavailable"
			}
			b, _ := json.Marshal(out)
			conn.Write(append(b, '\n'))
		}
	}()
}

func query(t *testing.T, values map[string]any) *Info {
	t.Helper()
	a, b := net.Pipe()
	fakeMpv(t, b, values)
	in, err := Query(a, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return &Info{in.File, in.Dir, in.FullDir, in.State, in.Position, in.Duration, in.Rate}
}

type Info struct {
	File     string
	Dir      []string
	FullDir  string
	State    int
	Position int
	Duration int
	Rate     float64
}

func TestQueryPlayingFile(t *testing.T) {
	got := query(t, map[string]any{
		"path": `D:\Videos\Sample Show\Season 1\Sample.Show.S01E02.mkv`, "filename": "Sample.Show.S01E02.mkv",
		"pause": false, "time-pos": 65.25, "duration": 1499.8, "speed": 1.5, "idle-active": false,
		"media-title": "Sample.Show.S01E02.mkv", "working-directory": `C:\Users\x`,
	})
	want := &Info{"Sample.Show.S01E02.mkv", []string{"Season 1", "Sample Show", "Videos"}, `D:\Videos\Sample Show\Season 1`, 2, 65250, 1499800, 1.5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestQueryPausedRelativePath(t *testing.T) {
	got := query(t, map[string]any{
		"path": "Sample Movie (2001).mkv", "filename": "Sample Movie (2001).mkv", "pause": true,
		"time-pos": 10.0, "duration": 600.0, "speed": 1.0, "idle-active": false, "working-directory": "/home/user/Movies",
	})
	if got.State != 1 || got.FullDir != "/home/user/Movies" || got.File != "Sample Movie (2001).mkv" || got.Position != 10000 {
		t.Errorf("got %+v", got)
	}
}

func TestQueryIdleAndMissingProperties(t *testing.T) {
	got := query(t, map[string]any{"idle-active": true, "pause": false})
	if got.State != -1 || got.File != "" {
		t.Errorf("idle: %+v", got)
	}
	// nothing at all known (an old or limited player): treated as idle, not as an error
	if got := query(t, map[string]any{}); got.State != -1 {
		t.Errorf("empty: %+v", got)
	}
}

func TestQueryStreamUsesTitleAndNoFolder(t *testing.T) {
	got := query(t, map[string]any{
		"path": "https://example.com/watch?v=abc", "filename": "watch?v=abc", "media-title": "Sample Stream Title",
		"pause": false, "time-pos": 1.0, "speed": 1.0, "idle-active": false,
	})
	if got.File != "Sample Stream Title" || got.FullDir != "" || len(got.Dir) != 0 || got.Duration != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestQueryBadSpeedFallsBackToOne(t *testing.T) {
	got := query(t, map[string]any{"path": "/v/a.mkv", "pause": false, "speed": 0.0, "idle-active": false})
	if got.Rate != 1 || got.File != "a.mkv" {
		t.Errorf("got %+v", got)
	}
}

func TestQueryGivesUpOnAHungPlayer(t *testing.T) {
	a, b := net.Pipe()
	go func() { // reads the requests and never answers
		r := bufio.NewReader(b)
		for {
			if _, err := r.ReadBytes('\n'); err != nil {
				return
			}
		}
	}()
	defer b.Close()
	start := time.Now()
	if _, err := Query(a, 300*time.Millisecond); err == nil {
		t.Fatal("a player that never answers must give an error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestQueryClosedEarly(t *testing.T) {
	a, b := net.Pipe()
	go func() {
		r := bufio.NewReader(b)
		r.ReadBytes('\n')
		b.Write([]byte(`{"request_id":1,"error":"success","data":"/v/a.mkv"}` + "\n"))
		b.Close()
	}()
	if _, err := Query(a, time.Second); err == nil {
		t.Fatal("a half answer must give an error")
	}
}

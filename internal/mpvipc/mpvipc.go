// Package mpvipc reads what an mpv-compatible player is playing through mpv's JSON IPC (--input-ipc-server): mpv
// itself, and MPC-QT, which always offers the same interface on a pipe of its own.
package mpvipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// The properties asked for, by request id.
var props = []string{"path", "filename", "pause", "time-pos", "duration", "speed", "idle-active", "media-title", "working-directory"}

type reply struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
	ID    *int            `json:"request_id"`
}

// Query asks the player on conn what it is playing and closes conn. It gives up after timeout. A reachable player
// with nothing loaded gives an Info with State -1.
func Query(conn io.ReadWriteCloser, timeout time.Duration) (*core.Info, error) {
	defer conn.Close()
	timer := time.AfterFunc(timeout, func() { conn.Close() }) // also ends a read that is waiting
	defer timer.Stop()

	var req strings.Builder
	for i, p := range props {
		b, _ := json.Marshal(map[string]any{"command": []string{"get_property", p}, "request_id": i + 1})
		req.Write(b)
		req.WriteByte('\n')
	}
	if _, err := io.WriteString(conn, req.String()); err != nil {
		return nil, err
	}

	got := map[string]reply{}
	r := bufio.NewReaderSize(conn, 64*1024)
	for len(got) < len(props) {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if len(line) == 0 {
				return nil, errors.New("mpv IPC: no complete answer")
			}
		}
		var rp reply
		if json.Unmarshal(line, &rp) == nil && rp.ID != nil && *rp.ID >= 1 && *rp.ID <= len(props) {
			got[props[*rp.ID-1]] = rp // events (no request id) are skipped
		}
		if err != nil && len(got) < len(props) {
			return nil, errors.New("mpv IPC: no complete answer")
		}
	}
	return toInfo(got), nil
}

func str(got map[string]reply, k string) string {
	var s string
	if rp, ok := got[k]; ok && rp.Error == "success" {
		_ = json.Unmarshal(rp.Data, &s)
	}
	return s
}

func num(got map[string]reply, k string) (float64, bool) {
	var f float64
	if rp, ok := got[k]; ok && rp.Error == "success" && json.Unmarshal(rp.Data, &f) == nil {
		return f, true
	}
	return 0, false
}

func flag(got map[string]reply, k string) bool {
	var b bool
	if rp, ok := got[k]; ok && rp.Error == "success" {
		_ = json.Unmarshal(rp.Data, &b)
	}
	return b
}

func isAbs(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) ||
		(len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/'))
}

// toInfo turns the answers into the same Info the MPC web interface gives.
func toInfo(got map[string]reply) *core.Info {
	path := str(got, "path")
	if path == "" || flag(got, "idle-active") {
		return core.NewInfo("", "", "", -1, 0, 0, 1)
	}
	file, filePath, dir := str(got, "filename"), "", ""
	if i := strings.Index(path, "://"); i > 0 && !strings.HasPrefix(strings.ToLower(path), "file://") {
		// a stream: the title is the best name there is, and there is no folder
		if t := str(got, "media-title"); t != "" {
			file = t
		}
	} else {
		filePath = strings.TrimPrefix(path, "file://")
		if !isAbs(filePath) {
			if wd := str(got, "working-directory"); wd != "" {
				sep := "/"
				if strings.Contains(wd, `\`) {
					sep = `\`
				}
				filePath = strings.TrimRight(wd, `\/`) + sep + filePath
			}
		}
		if i := strings.LastIndexAny(filePath, `\/`); i >= 0 {
			dir = filePath[:i]
			if file == "" {
				file = filePath[i+1:]
			}
		} else if file == "" {
			file = filePath
		}
	}
	state := 2
	if flag(got, "pause") {
		state = 1
	}
	pos, _ := num(got, "time-pos")
	dur, _ := num(got, "duration")
	rate, ok := num(got, "speed")
	if !ok || !(rate > 0) {
		rate = 1
	}
	return core.NewInfo(file, filePath, dir, state, int(pos*1000), int(dur*1000), rate)
}

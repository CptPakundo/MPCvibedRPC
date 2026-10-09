// Package vlchttp reads what VLC is playing through its web interface (the Lua HTTP interface, "extraintf=http").
// Only the read-only pages are asked for: /requests/status.json, and /requests/playlist.json for the full path of
// the item playing (status.json only has the file name). No command is ever sent.
package vlchttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

var (
	// ErrPassword means VLC answered but refused the password (401).
	ErrPassword = errors.New("VLC refused the password")
	// ErrNoPassword means VLC's web interface has no password of its own, so VLC keeps it closed (403).
	ErrNoPassword = errors.New("VLC's web interface has no password set, so VLC keeps it closed")
	// ErrNotVLC means something answered on the port, but not VLC.
	ErrNotVLC = errors.New("the port answers, but not as VLC")
)

// Reader asks one VLC. It remembers the path of the item playing, so the playlist is only read when the item changes.
type Reader struct {
	Client *http.Client // nil = http.DefaultClient

	mu       sync.Mutex
	lastKey  string
	lastPath string // the item's URI ("" when not found)
}

type status struct {
	State       string   `json:"state"`
	Time        *float64 `json:"time"`
	Length      float64  `json:"length"`
	Position    float64  `json:"position"`
	Rate        float64  `json:"rate"`
	CurrentPLID *int     `json:"currentplid"`
	APIVersion  *int     `json:"apiversion"`
	Information *struct {
		Category map[string]json.RawMessage `json:"category"`
	} `json:"information"`
}

func (r *Reader) get(ctx context.Context, port int, password, page string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+strconv.Itoa(port)+"/requests/"+page, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("", password) // VLC has no user name, only a password
	c := r.Client
	if c == nil {
		c = http.DefaultClient
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return nil, ErrPassword
	case res.StatusCode == http.StatusForbidden:
		return nil, ErrNoPassword
	case res.StatusCode < 200 || res.StatusCode > 299:
		return nil, ErrNotVLC
	}
	return io.ReadAll(io.LimitReader(res.Body, 4<<20))
}

// Query asks VLC on port what it is playing. A reachable VLC with nothing playing gives an Info with State 0.
func (r *Reader) Query(ctx context.Context, port int, password string) (*core.Info, error) {
	body, err := r.get(ctx, port, password, "status.json")
	if err != nil {
		return nil, err
	}
	var s status
	if json.Unmarshal(body, &s) != nil || s.APIVersion == nil || s.State == "" {
		return nil, ErrNotVLC
	}
	state := 0
	switch s.State {
	case "playing":
		state = 2
	case "paused":
		state = 1
	}
	meta := map[string]any{}
	if s.Information != nil {
		_ = json.Unmarshal(s.Information.Category["meta"], &meta)
	}
	metaStr := func(k string) string { v, _ := meta[k].(string); return strings.TrimSpace(v) }
	name := metaStr("filename")
	if state == 0 || (name == "" && metaStr("title") == "" && metaStr("now_playing") == "") {
		return core.NewInfo("", "", "", state, 0, 0, 1), nil
	}

	// the full path is only in the playlist; it is read again only when the item changes
	key := name
	if s.CurrentPLID != nil {
		key = strconv.Itoa(*s.CurrentPLID) + "|" + name
	}
	r.mu.Lock()
	uri, known := r.lastPath, r.lastKey == key
	r.mu.Unlock()
	if !known {
		uri = ""
		if pl, err := r.get(ctx, port, password, "playlist.json"); err == nil {
			uri = currentURI(pl, s.CurrentPLID)
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		r.mu.Lock()
		r.lastKey, r.lastPath = key, uri
		r.mu.Unlock()
	}

	file, filePath, dir := name, "", ""
	if p, ok := LocalPath(uri); ok {
		filePath = p
		if i := strings.LastIndexAny(p, `\/`); i >= 0 {
			dir = p[:i]
			if file == "" {
				file = p[i+1:]
			}
		}
	} else if uri != "" || name == "" {
		// a stream (or a disc): the title is the best name there is, and there is no folder
		for _, k := range []string{"title", "now_playing"} {
			if t := metaStr(k); t != "" {
				file = t
				break
			}
		}
	}

	dur := s.Length
	pos := 0.0
	if s.Time != nil {
		pos = *s.Time
	}
	if dur > 0 && s.Position > 0 && s.Position <= 1 {
		pos = s.Position * dur // VLC 3 rounds "time" down to whole seconds; the fraction is finer
	}
	rate := s.Rate
	if !(rate > 0) {
		rate = 1
	}
	return core.NewInfo(file, filePath, dir, state, int(pos*1000), int(dur*1000), rate), nil
}

// currentURI finds the URI of the item playing in playlist.json: a tree of nodes in VLC 3, a flat list in VLC 4.
func currentURI(body []byte, plid *int) string {
	var root any
	if json.Unmarshal(body, &root) != nil {
		return ""
	}
	want := ""
	if plid != nil {
		want = strconv.Itoa(*plid)
	}
	var found, byID string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			uri, _ := x["uri"].(string)
			if c, _ := x["current"].(string); c == "current" && found == "" {
				found = uri
			}
			if id, _ := x["id"].(string); want != "" && id == want && byID == "" {
				byID = uri
			}
			for _, k := range []string{"children", "_array"} {
				if c, ok := x[k]; ok {
					walk(c)
				}
			}
		}
	}
	walk(root)
	if found != "" {
		return found
	}
	return byID
}

// LocalPath turns a file:// URI into a path (C:\..., \\server\share\... or /...); ok is false for anything else.
func LocalPath(uri string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(uri), "file://") {
		return "", false
	}
	u, err := url.Parse(uri)
	if err != nil || u.Path == "" {
		return "", false
	}
	p := u.Path
	switch {
	case u.Host != "" && u.Host != "localhost":
		return `\\` + u.Host + strings.ReplaceAll(p, "/", `\`), true
	case len(p) >= 3 && p[0] == '/' && p[2] == ':':
		return strings.ReplaceAll(p[1:], "/", `\`), true
	}
	return p, true
}

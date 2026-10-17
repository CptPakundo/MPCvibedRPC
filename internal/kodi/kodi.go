// Package kodi reads what Kodi is playing through its JSON-RPC web server (Settings > Services > Control > "Allow
// remote control via HTTP"). Only read-only methods are called (Player.GetActivePlayers, Player.GetItem,
// Player.GetProperties); nothing is ever sent that changes the player.
package kodi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

var (
	// ErrPassword means Kodi answered but refused the user name or password (401).
	ErrPassword = errors.New("Kodi refused the user name or password")
	// ErrNotKodi means something answered at the address, but not as Kodi.
	ErrNotKodi = errors.New("the address answers, but not as Kodi")
)

// Reader asks one Kodi.
type Reader struct {
	Client *http.Client // nil = http.DefaultClient
}

// Base turns the address from the settings ("127.0.0.1:8080", "http://192.168.1.30:8080/") into the server's base URL.
func Base(address string) string {
	a := strings.TrimRight(strings.TrimSpace(address), "/")
	a = strings.TrimSuffix(a, "/jsonrpc")
	if !strings.Contains(a, "://") {
		a = "http://" + a
	}
	return a
}

func (r *Reader) call(ctx context.Context, base, user, password, method string, params, out any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/jsonrpc", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, password)
	c := r.Client
	if c == nil {
		c = http.DefaultClient
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return ErrPassword
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return ErrNotKodi
	}
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&reply) != nil || reply.Error != nil || reply.Result == nil {
		return ErrNotKodi
	}
	if json.Unmarshal(reply.Result, out) != nil {
		return ErrNotKodi
	}
	return nil
}

// item is what Player.GetItem says about the item playing. A file that is not in Kodi's library has type "unknown"
// (season and episode -1).
type item struct {
	Type      string `json:"type"` // movie, episode, musicvideo, song, unknown, ...
	Label     string `json:"label"`
	Title     string `json:"title"`
	ShowTitle string `json:"showtitle"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	Year      int    `json:"year"`
	File      string `json:"file"`
}

type kodiTime struct {
	Hours        int `json:"hours"`
	Minutes      int `json:"minutes"`
	Seconds      int `json:"seconds"`
	Milliseconds int `json:"milliseconds"`
}

func (t kodiTime) ms() int {
	return ((t.Hours*60+t.Minutes)*60+t.Seconds)*1000 + t.Milliseconds
}

// Query asks Kodi at address what it is playing. A reachable Kodi with nothing playing gives an Info with State 0.
func (r *Reader) Query(ctx context.Context, address, user, password string) (*core.Info, error) {
	base := Base(address)
	var players []struct {
		PlayerID int    `json:"playerid"`
		Type     string `json:"type"` // video, audio, picture
	}
	if err := r.call(ctx, base, user, password, "Player.GetActivePlayers", map[string]any{}, &players); err != nil {
		return nil, err
	}
	id := -1
	for _, want := range []string{"video", "audio"} {
		for _, p := range players {
			if p.Type == want && id < 0 {
				id = p.PlayerID
			}
		}
	}
	if id < 0 {
		return core.NewInfo("", "", "", 0, 0, 0, 1), nil
	}
	var it struct {
		Item item `json:"item"`
	}
	props := []string{"title", "showtitle", "season", "episode", "year", "file"}
	if err := r.call(ctx, base, user, password, "Player.GetItem", map[string]any{"playerid": id, "properties": props}, &it); err != nil {
		return nil, err
	}
	var st struct {
		Time      kodiTime `json:"time"`
		TotalTime kodiTime `json:"totaltime"`
		Speed     int      `json:"speed"`
	}
	if err := r.call(ctx, base, user, password, "Player.GetProperties", map[string]any{"playerid": id, "properties": []string{"time", "totaltime", "speed"}}, &st); err != nil {
		return nil, err
	}
	state, rate := 2, 1.0
	if st.Speed == 0 {
		state = 1
	} else if st.Speed > 1 {
		rate = float64(st.Speed)
	}
	return info(it.Item, state, st.Time.ms(), st.TotalTime.ms(), rate), nil
}

// info writes a library item the way a well-named file would be called ("Sample Show - S01E02 - Title", "Sample
// Movie (2020)"), so the usual parsing and lookups apply; a file outside the library is read like any local file.
// Kodi's path of the file goes to Where, for the Never show list.
func info(it item, state, pos, dur int, rate float64) *core.Info {
	file := it.File
	name, path := file, ""
	if strings.Contains(file, "://") {
		u := strings.SplitN(file, "?", 2)[0] // a network share or an add-on: named by the last part, no folder
		name = u[strings.LastIndex(u, "/")+1:]
	} else if i := strings.LastIndexAny(file, `\/`); i >= 0 {
		name, path = file[i+1:], file
	}
	title := core.CleanTitle(it.Title)
	switch {
	case it.Type == "episode" && it.ShowTitle != "" && it.Season >= 0 && it.Episode >= 0:
		s, e := it.Season, it.Episode
		name, path = core.EpisodeName(core.CleanTitle(it.ShowTitle), title, &s, &e), ""
	case it.Type == "movie" && title != "":
		name, path = core.MovieName(title, it.Year), ""
	case name == "" && title != "":
		name = title
	case name == "":
		name = core.CleanTitle(it.Label)
	}
	in := core.NewInfo(name, path, "", state, pos, dur, rate)
	if file != "" {
		in.Where = []string{file}
	}
	return in
}

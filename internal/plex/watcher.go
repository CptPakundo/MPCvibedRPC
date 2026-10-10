package plex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// How long a player may stay silent before what it played is forgotten (players report every few seconds while they
// play or pause), and how long the event stream may stay silent before it is reopened (the server pings every few
// seconds).
const (
	sessionTimeout = 3 * time.Minute
	streamTimeout  = 45 * time.Second
)

// Watcher follows one server's play notifications (Server-Sent Events from /:/eventsource/notifications) and knows
// what the user is playing there. Run it in a goroutine; Now is safe to call at any time.
type Watcher struct {
	Client  *Client
	Token   string // the account's token
	Account int64  // the account's id: on a server the user owns, only their own playback counts
	Server  string // the chosen server's machine identifier ("" = the account's only server, or its first own one)
	Address string // a fixed address for the server ("" = the addresses plex.tv lists)
	Log     func(level, msg string)

	mu       sync.Mutex
	conn     bool   // the event stream is open
	name     string // the server's name
	problem  string // why the server cannot be followed (for the window)
	players  map[string]*playback
	items    map[string]*Item // metadata by ratingKey
	owned    bool
	base     string
	srvToken string
	now      func() time.Time
	retry    time.Duration // the first wait before reconnecting (tests shorten it)
}

type playback struct {
	ratingKey string
	state     string // playing, paused, buffering
	offset    int    // ms, at "at"
	at        time.Time
	mine      int // 0 unknown, 1 the user's, -1 someone else's
	item      *Item
}

type notification struct {
	ClientIdentifier string `json:"clientIdentifier"`
	SessionKey       string `json:"sessionKey"`
	RatingKey        string `json:"ratingKey"`
	Key              string `json:"key"`
	ViewOffset       int    `json:"viewOffset"`
	State            string `json:"state"`
}

func (w *Watcher) log(level, msg string) {
	if w.Log != nil {
		w.Log(level, msg)
	}
}

func (w *Watcher) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

// Problem says why Plex cannot be followed right now ("" when all is well).
func (w *Watcher) Problem() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.problem
}

func (w *Watcher) setProblem(p string) {
	w.mu.Lock()
	changed := p != w.problem
	w.problem = p
	w.mu.Unlock()
	if changed && p != "" {
		w.log("WARN", p)
	}
}

// Now is what the user plays on the server: nil while the server is not followed, an Info with State -1 when nothing
// of theirs is playing, else the video (one playing beats one paused; the most recent report wins).
func (w *Watcher) Now() *core.Info {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.conn {
		return nil
	}
	now := w.clock()
	var best *playback
	for id, p := range w.players {
		if now.Sub(p.at) > sessionTimeout {
			delete(w.players, id) // the player went quiet without saying it stopped
			continue
		}
		if p.item == nil || p.mine < 0 || (w.owned && p.mine == 0) {
			continue
		}
		if best == nil {
			best = p
		} else if pp, bp := p.state != "paused", best.state != "paused"; pp != bp {
			if pp {
				best = p
			}
		} else if p.at.After(best.at) {
			best = p
		}
	}
	if best == nil {
		return core.NewInfo("", "", "", -1, 0, 0, 1)
	}
	pos, state := best.offset, 1
	if best.state != "paused" {
		state = 2
		if best.state == "playing" {
			pos += int(now.Sub(best.at) / time.Millisecond) // reports come every few seconds; the time runs on between them
		}
	}
	dur := best.item.Duration
	if dur > 0 && pos > dur {
		pos = dur
	}
	return core.NewInfo(best.item.Name(), "", "", state, pos, dur, 1)
}

// Run follows the server until ctx ends, reconnecting after a pause when the connection drops.
func (w *Watcher) Run(ctx context.Context) {
	first := 5 * time.Second
	if w.retry > 0 {
		first = w.retry
	}
	wait := first
	for ctx.Err() == nil {
		started := time.Now()
		err := w.follow(ctx)
		w.mu.Lock()
		w.conn = false
		w.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, ErrToken) {
			w.setProblem("Plex refused the sign-in. Sign in to Plex again (Advanced > Plex).")
		} else if err != nil {
			w.setProblem(err.Error())
		}
		if time.Since(started) > 2*time.Minute {
			wait = first // it had been working: try again soon
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait < 2*time.Minute {
			wait *= 2
		}
	}
}

// find looks up the server on plex.tv: its addresses, whether the user owns it, and the token for it.
func (w *Watcher) find(ctx context.Context) (Server, error) {
	list, err := w.Client.Servers(ctx, w.Token)
	if err != nil {
		if errors.Is(err, ErrToken) {
			return Server{}, err
		}
		return Server{}, fmt.Errorf("Could not ask plex.tv for your Plex servers (%v).", err)
	}
	for _, s := range list {
		if w.Server == "" || s.ID == w.Server {
			return s, nil
		}
	}
	if w.Server == "" {
		return Server{}, errors.New("Your Plex account has no server to follow.")
	}
	return Server{}, errors.New("The chosen Plex server is no longer in your Plex account. Choose another one (Advanced > Plex).")
}

// reach finds an address of the server that answers as that server.
func (w *Watcher) reach(ctx context.Context, s Server, token string) (string, error) {
	addrs := s.Addresses()
	if w.Address != "" {
		addrs = []string{strings.TrimRight(w.Address, "/")}
	}
	for _, a := range addrs {
		var id struct {
			MediaContainer struct {
				MachineIdentifier string `json:"machineIdentifier"`
			} `json:"MediaContainer"`
		}
		c, cancel := context.WithTimeout(ctx, 4*time.Second)
		_, err := w.Client.getJSON(c, "GET", a+"/identity", token, &id)
		cancel()
		if err == nil && (s.ID == "" || id.MediaContainer.MachineIdentifier == s.ID) {
			return a, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	if w.Address != "" {
		return "", fmt.Errorf("The Plex server%s does not answer at the address set in Advanced > Plex server address.", quoted(s.Name))
	}
	return "", fmt.Errorf("Could not reach the Plex server%s. Is it running, and can this computer reach it?", quoted(s.Name))
}

// quoted is " \"name\"" (or nothing): server names can be personal, and the diagnostics leave out what is quoted.
func quoted(name string) string {
	if name == "" {
		return ""
	}
	return ` "` + name + `"`
}

// follow connects and reads the event stream until it ends.
func (w *Watcher) follow(ctx context.Context) error {
	var s Server
	if w.Address != "" && w.Server == "" {
		s = Server{} // a fixed address with no server chosen: take whatever answers there
	} else {
		var err error
		if s, err = w.find(ctx); err != nil {
			if w.Address == "" || errors.Is(err, ErrToken) {
				return err
			}
			s = Server{ID: w.Server} // plex.tv is unreachable, but the address is known
		}
	}
	token := s.Token
	if token == "" {
		token = w.Token
	}
	base, err := w.reach(ctx, s, token)
	if err != nil {
		return err
	}

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(sctx, "GET", base+"/:/eventsource/notifications?filters=playing", nil)
	if err != nil {
		return err
	}
	w.Client.header(req, token)
	req.Header.Set("Accept", "text/event-stream")
	res, err := w.Client.stream().Do(req)
	if err != nil {
		return fmt.Errorf("Could not open the Plex server's notifications (%v).", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return ErrToken
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("The Plex server refused its notifications (HTTP %d).", res.StatusCode)
	}

	w.mu.Lock()
	first := w.base != base
	w.conn, w.name, w.owned, w.base, w.srvToken = true, s.Name, s.Owned, base, token
	if w.players == nil {
		w.players = map[string]*playback{}
	}
	if w.items == nil {
		w.items = map[string]*Item{}
	}
	w.problem = ""
	w.mu.Unlock()
	if first {
		w.log("INFO", "Following the Plex server"+quoted(s.Name)+".")
	}

	// a stream that goes quiet (the server pings every few seconds) is reopened
	alive := make(chan struct{}, 1)
	go func() {
		t := time.NewTimer(streamTimeout)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				return
			case <-alive:
				if !t.Stop() {
					<-t.C
				}
				t.Reset(streamTimeout)
			case <-t.C:
				cancel()
				return
			}
		}
	}()

	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	event, data := "", ""
	for sc.Scan() {
		select {
		case alive <- struct{}{}:
		default:
		}
		line := sc.Text()
		switch {
		case line == "":
			if event == "playing" && data != "" {
				w.handle(sctx, data)
			}
			event, data = "", ""
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			data += strings.TrimPrefix(line[5:], " ")
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	return errors.New("The Plex server closed its notifications; reconnecting.")
}

// handle takes one "playing" event. Its data holds one notification, or a list of them.
func (w *Watcher) handle(ctx context.Context, data string) {
	var one struct {
		N json.RawMessage `json:"PlaySessionStateNotification"`
	}
	if json.Unmarshal([]byte(data), &one) != nil || len(one.N) == 0 {
		return
	}
	var list []notification
	if one.N[0] == '[' {
		_ = json.Unmarshal(one.N, &list)
	} else {
		var n notification
		if json.Unmarshal(one.N, &n) == nil {
			list = []notification{n}
		}
	}
	for _, n := range list {
		w.take(ctx, n)
	}
}

func (w *Watcher) take(ctx context.Context, n notification) {
	id := n.ClientIdentifier
	if id == "" {
		id = "session " + n.SessionKey
	}
	rk := n.RatingKey
	if rk == "" {
		rk = strings.TrimPrefix(n.Key, "/library/metadata/")
	}
	w.mu.Lock()
	if n.State == "stopped" || rk == "" || strings.Contains(rk, "/") {
		delete(w.players, id)
		w.mu.Unlock()
		return
	}
	p := w.players[id]
	if p == nil || p.ratingKey != rk {
		p = &playback{ratingKey: rk}
		w.players[id] = p
	}
	p.state, p.offset, p.at = n.State, n.ViewOffset, w.clock()
	item, owned, needItem, needOwner := w.items[rk], w.owned, false, false
	needItem = item == nil
	needOwner = owned && p.mine == 0
	w.mu.Unlock()

	if needItem {
		item = w.item(ctx, rk)
		if item == nil {
			return
		}
	}
	mine := 1
	if needOwner {
		mine = w.whose(ctx, n.ClientIdentifier, rk)
	}
	w.mu.Lock()
	if q := w.players[id]; q == p {
		if !item.IsVideo() {
			p.mine = -1 // music and photos are not "Watching"
		} else if needOwner || p.mine == 0 {
			p.mine = mine
		}
		p.item = item
	}
	w.mu.Unlock()
}

// item fetches (and remembers) the metadata of what is played.
func (w *Watcher) item(ctx context.Context, rk string) *Item {
	w.mu.Lock()
	base, token := w.base, w.srvToken
	w.mu.Unlock()
	var r struct {
		MediaContainer struct {
			Metadata []Item `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := w.Client.getJSON(c, "GET", base+"/library/metadata/"+url.PathEscape(rk), token, &r); err != nil || len(r.MediaContainer.Metadata) == 0 {
		return nil
	}
	it := r.MediaContainer.Metadata[0]
	w.mu.Lock()
	if len(w.items) > 200 {
		w.items = map[string]*Item{}
	}
	w.items[rk] = &it
	w.mu.Unlock()
	return &it
}

// whose tells, on a server the user owns, whether a player's playback is the user's: 1 yes, -1 someone else's, 0 not
// known yet (the server's session list does not have it yet; the next report asks again).
func (w *Watcher) whose(ctx context.Context, client, rk string) int {
	w.mu.Lock()
	base, token := w.base, w.srvToken
	w.mu.Unlock()
	var r struct {
		MediaContainer struct {
			Metadata []struct {
				RatingKey string `json:"ratingKey"`
				User      struct {
					ID json.Number `json:"id"`
				} `json:"User"`
				Player struct {
					MachineIdentifier string      `json:"machineIdentifier"`
					UserID            json.Number `json:"userID"`
				} `json:"Player"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := w.Client.getJSON(c, "GET", base+"/status/sessions", token, &r); err != nil {
		return 0
	}
	for _, m := range r.MediaContainer.Metadata {
		if m.Player.MachineIdentifier != client || m.RatingKey != rk {
			continue
		}
		uid := m.User.ID.String()
		if uid == "" {
			uid = m.Player.UserID.String() // the server may leave the user out and only say it with the player
		}
		// the owner appears as user 1 on their own server; anyone else under their plex.tv account id
		if uid == "1" || uid == fmt.Sprint(w.Account) {
			return 1
		}
		return -1
	}
	return 0
}

// stream is the HTTP client for the long-lived event stream: the same transport, but no overall time limit.
func (c *Client) stream() *http.Client {
	h := *c.http()
	h.Timeout = 0
	return &h
}

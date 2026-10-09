package artwork

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

type recResp struct {
	OK         bool            `json:"ok"`
	Status     int             `json:"status"`
	RetryAfter string          `json:"retryAfter"`
	JSON       json.RawMessage `json:"json"`
	Text       *string         `json:"text"`
	JSONThrow  string          `json:"jsonThrow"`
	Throw      *struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"throw"`
}

type recReq struct {
	Method string  `json:"method"`
	URL    string  `json:"url"`
	Body   string  `json:"body"`
	Ms     int     `json:"ms"`
	Resp   recResp `json:"resp"`
}

type recLookup struct {
	Media    map[string]any  `json:"media"`
	Requests []recReq        `json:"requests"`
	Result   json.RawMessage `json:"result"`
	Ms       int             `json:"ms"`
}

type recInstance struct {
	ID      int             `json:"id"`
	Cfg     json.RawMessage `json:"cfg"`
	Lookups []recLookup     `json:"lookups"`
}

type recCall struct {
	Fn     string            `json:"fn"`
	Args   []json.RawMessage `json:"args"`
	Result json.RawMessage   `json:"result"`
}

type transcripts struct {
	Instances []recInstance `json:"instances"`
	Calls     []recCall     `json:"calls"`
}

func loadTranscripts(t testing.TB) *transcripts {
	f, err := os.Open("testdata/transcripts.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var tr transcripts
	if err := json.NewDecoder(z).Decode(&tr); err != nil {
		t.Fatal(err)
	}
	return &tr
}

// replayTransport answers requests from a recorded lookup.
type replayTransport struct {
	mu      sync.Mutex
	queues  map[string][]recReq
	missing []string
}

func (rt *replayTransport) load(reqs []recReq) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.queues = map[string][]recReq{}
	rt.missing = nil
	for _, r := range reqs {
		k := r.Method + " " + r.URL + " " + r.Body
		rt.queues[k] = append(rt.queues[k], r)
	}
}

func (rt *replayTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	k := req.Method + " " + req.URL.String() + " " + body
	rt.mu.Lock()
	q := rt.queues[k]
	var r recReq
	if len(q) == 0 {
		rt.missing = append(rt.missing, k)
		rt.mu.Unlock()
		return nil, &MockError{Name: "TypeError", Msg: "fetch failed"}
	}
	r = q[0]
	if len(q) > 1 {
		rt.queues[k] = q[1:]
	}
	rt.mu.Unlock()
	if r.Ms > 0 {
		select {
		case <-time.After(time.Duration(r.Ms) * time.Millisecond):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	if r.Resp.Throw != nil {
		return nil, &MockError{Name: r.Resp.Throw.Name, Msg: r.Resp.Throw.Message}
	}
	status := r.Resp.Status
	if status == 0 {
		status = 200
	}
	var payload string
	switch {
	case r.Resp.Text != nil:
		payload = *r.Resp.Text
	case len(r.Resp.JSON) > 0:
		payload = string(r.Resp.JSON)
	case r.Resp.JSONThrow != "":
		payload = "<not json"
	default:
		payload = "null"
	}
	h := http.Header{}
	if r.Resp.RetryAfter != "" {
		h.Set("Retry-After", r.Resp.RetryAfter)
	}
	return &http.Response{StatusCode: status, Status: fmt.Sprint(status), Header: h, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
}

func mediaFromJSON(m map[string]any) *core.Media {
	o := core.NewMedia()
	str := func(k string) string { s, _ := m[k].(string); return s }
	boo := func(k string) bool { b, _ := m[k].(bool); return b }
	num := func(k string) int { return numOrNA2(m[k]) }
	o.Title, o.Display, o.Ext = str("title"), str("display"), str("ext")
	o.Label, o.EpTitle, o.EpTitleBase = str("label"), str("epTitle"), str("epTitleBase")
	o.Code, o.CodePrefix, o.IndexTitle = str("code"), str("codePrefix"), str("indexTitle")
	o.Year, o.Season, o.Episode, o.Index = num("year"), num("season"), num("episode"), num("index")
	o.CodeEpisode, o.BadgeEpisode = num("codeEpisode"), num("badgeEpisode")
	if d := num("durationMin"); d != core.NA {
		o.DurationMin = d
	}
	o.IsEpisode, o.AnimeHint, o.FromFolder, o.Anime, o.Junk = boo("isEpisode"), boo("animeHint"), boo("fromFolder"), boo("anime"), boo("junk")
	if a, ok := m["parts"].([]any); ok {
		for _, p := range a {
			o.Parts = append(o.Parts, fmt.Sprint(p))
		}
	}
	return o
}

func numOrNA2(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return core.NA
}

// normalise drops the "empty" values JavaScript would have left undefined.
func normalise(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if k == "ids" {
				if im, ok := e.(map[string]any); ok {
					for ik, iv := range im {
						if f, ok := iv.(float64); ok {
							im[ik] = numStr(f)
						}
					}
				}
			}
			n := normalise(e)
			switch y := n.(type) {
			case nil:
				continue
			case bool:
				if !y {
					continue
				}
			case string:
				if y == "" {
					continue
				}
			case float64:
				if y == 0 {
					continue
				}
			case map[string]any:
				if len(y) == 0 {
					continue
				}
			}
			out[k] = n
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalise(e)
		}
		return out
	}
	return v
}

func canonJSON(raw []byte) any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "bad json: " + err.Error()
	}
	return normalise(v)
}

func TestReplayLookups(t *testing.T) {
	tr := loadTranscripts(t)
	bad, total, missing := 0, 0, 0
	for _, inst := range tr.Instances {
		cfg := core.LoadConfig(inst.Cfg)
		rt := &replayTransport{}
		var logs []string
		var logMu sync.Mutex // the finder logs from background goroutines
		aw := New(&cfg, Options{Client: &http.Client{Transport: rt}, Log: func(l, m string) { logMu.Lock(); logs = append(logs, l+": "+m); logMu.Unlock() }})
		for li, lk := range inst.Lookups {
			// The JS test suite only started the next lookup after the previous one had settled.
			for i := 0; i < 300; i++ {
				aw.mu.Lock()
				n := len(aw.inflight)
				aw.mu.Unlock()
				if n == 0 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			rt.load(lk.Requests)
			media := mediaFromJSON(lk.Media)
			got := aw.Lookup(media)
			total++
			var gotJSON []byte = []byte("null")
			if got != nil {
				gotJSON, _ = json.Marshal(got)
			}
			if !reflect.DeepEqual(canonJSON(gotJSON), canonJSON(lk.Result)) {
				bad++
				if bad <= 15 {
					logMu.Lock()
					snap := append([]string(nil), logs...)
					logMu.Unlock()
					t.Errorf("instance %d lookup %d (%q S%v E%v):\n got  %s\n want %s\n logs: %v", inst.ID, li, media.Title, lk.Media["season"], lk.Media["episode"], gotJSON, lk.Result, snap)
				}
			}
			rt.mu.Lock()
			if len(rt.missing) > 0 {
				missing += len(rt.missing)
				if missing <= 15 {
					t.Logf("instance %d lookup %d (%q): unrecorded requests: %v", inst.ID, li, media.Title, rt.missing)
				}
			}
			rt.mu.Unlock()
		}
	}
	t.Logf("%d lookups, %d mismatches, %d unrecorded requests", total, bad, missing)
	if bad > 0 || missing > 0 {
		t.Fail()
	}
}

var _ = bytes.NewReader
var _ = context.Background

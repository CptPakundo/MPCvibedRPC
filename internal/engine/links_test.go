package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// linkSetup plays a movie that a fake Cinemeta knows, so the activity has a cover and a catalog page.
func linkSetup(t *testing.T, edit func(*core.Config)) (*Engine, *disc, *logs) {
	cat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/catalog/movie/") {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"metas":[{"name":"Sample Movie","releaseInfo":"2020","poster":"https://img.example/poster.jpg","imdb_id":"tt0000001"}]}`)
	}))
	t.Cleanup(cat.Close)
	m := newMPC(t)
	m.file, m.filepath = "Sample.Movie.2020.1080p.mkv", `C:\Movies\Sample.Movie.2020.1080p.mkv`
	return newTestEngine(t, withPort(m.port), func(cfg *core.Config, _ *Options) {
		cfg.ShowArtwork = true // the default; the lookups here go to the fake catalog
		cfg.ArtworkSources, cfg.AnimeSources, cfg.CinemetaBase = []string{"cinemeta"}, []string{}, cat.URL
		cfg.RichInfo, cfg.EpisodeTitles = false, false
		if edit != nil {
			edit(cfg)
		}
	})
}

func lastActivity(d *disc) map[string]any {
	l := d.list()
	if len(l) == 0 {
		return nil
	}
	a, _ := l[len(l)-1]["activity"].(map[string]any)
	return a
}

func TestTitleAndCoverLinkToTheCatalogPage(t *testing.T) {
	e, d, _ := linkSetup(t, nil)
	e.Start()
	const page = "https://www.imdb.com/title/tt0000001/"
	eventually(t, "an activity with the cover", func() bool {
		a := lastActivity(d)
		as, _ := a["assets"].(map[string]any)
		return as != nil && as["large_image"] == "https://img.example/poster.jpg"
	})
	a := lastActivity(d)
	as := a["assets"].(map[string]any)
	if a["details_url"] != page || as["large_url"] != page {
		t.Fatalf("the details line and the cover should open the catalog page: %v", a)
	}
	if a["state_url"] != nil {
		t.Fatalf("no state line, so no state link: %v", a)
	}
	if as["small_image"] != "mpc-hc" {
		t.Fatalf("MPC-HC keeps its own corner icon: %v", as)
	}
	if b, _ := a["buttons"].([]any); len(b) != 1 {
		t.Fatalf("the button stays: %v", a)
	}
	eventually(t, "the preview says so", func() bool { p := e.Status().Preview; return p != nil && p.Linked })
}

func TestNoLinksWithTheButtonSwitchOff(t *testing.T) {
	e, d, _ := linkSetup(t, func(c *core.Config) { c.LinkButton = false })
	e.Start()
	eventually(t, "an activity with the cover", func() bool {
		as, _ := lastActivity(d)["assets"].(map[string]any)
		return as != nil && as["large_image"] == "https://img.example/poster.jpg"
	})
	a := lastActivity(d)
	if a["details_url"] != nil || a["state_url"] != nil || a["assets"].(map[string]any)["large_url"] != nil || a["buttons"] != nil {
		t.Fatalf("no links of any kind: %v", a)
	}
}

// A Discord that drops the connection is not a refusal: the engine keeps the Watching activity and sends it again.
func TestDroppedConnectionKeepsWatchingMode(t *testing.T) {
	e, d, l := linkSetup(t, nil)
	d.mu.Lock()
	d.dropNext = true
	d.mu.Unlock()
	e.Start()
	eventually(t, "sent again after the drop", func() bool { return len(d.list()) >= 2 })
	a := lastActivity(d)
	if a["type"] != float64(3) || l.has("Falling back to basic mode") || l.has("Sending them without links") {
		t.Fatalf("a dropped connection must not change the mode: %v", a)
	}
	e.mu.Lock()
	basic, noLinks := e.basicMode, e.noLinks
	e.mu.Unlock()
	if basic || noLinks {
		t.Fatalf("basicMode=%v noLinks=%v", basic, noLinks)
	}
}

// A Discord that refuses the link fields gets the same Watching activity without them, not the basic one.
func TestRefusedLinksAreLeftOut(t *testing.T) {
	e, d, l := linkSetup(t, nil)
	d.rejectLinks = true
	e.Start()
	eventually(t, "links refused", func() bool { return l.has("Sending them without links") })
	eventually(t, "sent without links", func() bool {
		a := lastActivity(d)
		as, _ := a["assets"].(map[string]any)
		return a != nil && a["details_url"] == nil && a["state_url"] == nil && as != nil && as["large_image"] == "https://img.example/poster.jpg"
	})
	a := lastActivity(d)
	if a["type"] != float64(3) || a["assets"].(map[string]any)["large_url"] != nil || l.has("Falling back to basic mode") {
		t.Fatalf("still the Watching activity, without links: %v", a)
	}
	eventually(t, "the preview shows what was sent", func() bool { p := e.Status().Preview; return p != nil && !p.Linked })
}

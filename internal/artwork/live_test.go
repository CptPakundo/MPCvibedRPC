package artwork

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// The weekly canary (.github/workflows/canary.yml) asks every keyless catalog about one long-known title, through the
// same search and episode code the program uses, and fails when a service stops answering or changes the shape of
// its answer: no match, a wrong year, no link, no cover, no episode names. It contacts the real services, so it is
// opt-in (MPCRPC_LIVE_CATALOGS=1). TMDB needs a key and the public Jikan closed, so neither is asked.
//
// A catalog check needs real titles: these are chosen because they are old, listed by every catalog, and unlikely to
// be renamed.
type liveTitle struct {
	file, title string
	year        int
}

var (
	liveMovie  = liveTitle{"Night of the Living Dead (1968).mkv", "Night of the Living Dead", 1968}
	liveSeries = liveTitle{"The Twilight Zone (1959) - S01E01.mkv", "The Twilight Zone", 1959}
	liveAnime  = liveTitle{"Cowboy Bebop - 01.mkv", "Cowboy Bebop", 1998}
)

func TestLiveCatalogs(t *testing.T) {
	if os.Getenv("MPCRPC_LIVE_CATALOGS") != "1" {
		t.Skip("set MPCRPC_LIVE_CATALOGS=1 to ask the real catalog services")
	}
	cfg := core.DefaultConfig()
	// ask retries a service that is down or slow twice, half a minute apart, each time with a fresh finder (a failing
	// host is paused by the first one), so one bad minute does not open an issue. Any other error, or an answer, ends it.
	ask := func(t *testing.T, f func(a *Artwork) error) {
		t.Helper()
		for try := 1; ; try++ {
			err := f(New(&cfg, Options{Log: func(level, msg string) { t.Logf("%s: %s", level, msg) }}))
			if !unavailable(err) || try == 3 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			t.Logf("try %d: %v; again in 30 s", try, err)
			time.Sleep(30 * time.Second)
		}
	}

	// search asks one catalog the way a lookup does and returns the candidate the program would pick.
	search := func(t *testing.T, id, typ string, lt liveTitle) *Cand {
		t.Helper()
		p := buildProviders()[id]
		var cands []*Cand
		ask(t, func(a *Artwork) (err error) {
			if cands, err = p.Search(a, typ, lt.title, core.ParseMedia(lt.file, &cfg, nil)); err != nil {
				err = fmt.Errorf("%s search: %w", p.Label, err)
			}
			return err
		})
		if len(cands) == 0 {
			t.Fatalf("%s search for %q: no results", p.Label, lt.title)
		}
		c := pickMatch(cands, lt.title, lt.year, false, PickOpts{Earliest: p.Earliest, ExactOnly: p.ExactOnly, TrustAlias: p.TrustAlias, Ranked: p.Ranked})
		if c == nil {
			var seen []string
			for _, x := range cands {
				seen = append(seen, x.Titles...)
			}
			t.Fatalf("%s search for %q (%d): nothing matched among %q", p.Label, lt.title, lt.year, seen)
		}
		if c.Year != 0 && c.Year != lt.year {
			t.Errorf("%s: %q matched with year %d, want %d", p.Label, lt.title, c.Year, lt.year)
		}
		if c.URL == "" {
			t.Errorf("%s: %q has no link", p.Label, lt.title)
		}
		if c.Poster == "" {
			t.Errorf("%s: %q has no cover", p.Label, lt.title)
		}
		t.Logf("%s: %q (%d) %s, cover %s", p.Label, c.Titles[0], c.Year, c.URL, c.Poster)
		return c
	}
	// firstEpisode asks one episode source for the first episode and wants a usable name for it.
	firstEpisode := func(t *testing.T, id string, lt liveTitle, ids core.ArtIDs, ctx epCtx) {
		t.Helper()
		media := core.ParseMedia(lt.file, &cfg, nil)
		ctx.query, ctx.seasonNo = lt.title, media.Season
		var list []EpItem
		label := ""
		ask(t, func(a *Artwork) (err error) {
			p := a.res.provs[id]
			label = p.label
			if list, err = p.list(a.res, media, ids, ctx); err != nil {
				err = fmt.Errorf("%s episodes: %w", p.label, err)
			}
			return err
		})
		for _, e := range list {
			if e.Season == 1 && e.Number == 1 {
				if !usable(e.Name) {
					t.Fatalf("%s: episode 1 of %q has no usable name (%q)", label, lt.title, e.Name)
				}
				t.Logf("%s: %d episodes listed, episode 1 is %q", label, len(list), e.Name)
				return
			}
		}
		t.Fatalf("%s: episode 1 of %q is not in its list of %d", label, lt.title, len(list))
	}

	t.Run("cinemeta", func(t *testing.T) {
		c := search(t, "cinemeta", "movie", liveMovie)
		var info *core.ArtInfo
		ask(t, func(a *Artwork) (err error) {
			if info, err = a.fetchInfo("movie", imdbIDFrom(c.URL)); err != nil {
				err = fmt.Errorf("Cinemeta details: %w", err)
			}
			return err
		})
		if !hasInfoData(info) {
			t.Errorf("Cinemeta details for %q: no genres or rating", liveMovie.title)
		} else {
			t.Logf("Cinemeta details: %s", infoSummary(info))
		}
		sc := search(t, "cinemeta", "series", liveSeries)
		firstEpisode(t, "cinemeta", liveSeries, core.ArtIDs{Imdb: imdbIDFrom(sc.URL)}, epCtx{})
	})
	t.Run("imdb", func(t *testing.T) {
		search(t, "imdb", "movie", liveMovie)
		search(t, "imdb", "series", liveSeries)
	})
	t.Run("tvmaze", func(t *testing.T) {
		c := search(t, "tvmaze", "series", liveSeries)
		firstEpisode(t, "tvmaze", liveSeries, core.ArtIDs{Tvmaze: c.Ref}, epCtx{})
	})
	t.Run("anilist", func(t *testing.T) {
		search(t, "anilist", "series", liveAnime)
	})
	t.Run("kitsu", func(t *testing.T) {
		c := search(t, "kitsu", "series", liveAnime)
		firstEpisode(t, "kitsu", liveAnime, core.ArtIDs{Kitsu: c.Ref}, epCtx{anime: true})
	})
	t.Run("wikipedia", func(t *testing.T) {
		search(t, "wikipedia", "series", liveSeries) // its article's lead image is free to use (the film's is not)
	})
	t.Run("bulbapedia", func(t *testing.T) {
		for prefix, page := range bulbapediaPages {
			var pg *bulbaPage
			ask(t, func(a *Artwork) (err error) {
				if pg, err = a.res.bulbaList(page); err != nil {
					err = fmt.Errorf("Bulbapedia %s: %w", page, err)
				}
				return err
			})
			if name := pg.titles[prefix+"001"]; !usable(name) {
				t.Errorf("Bulbapedia %s: no name for %s001 among %d codes", page, prefix, len(pg.titles))
			} else {
				t.Logf("Bulbapedia %s: %d codes, %s001 is %q", page, len(pg.titles), prefix, name)
			}
		}
	})
}

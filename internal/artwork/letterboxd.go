package artwork

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
)

// Letterboxd (letterboxdLink): letterboxd.com/imdb/<id>/ and, for films, /tmdb/<id>/ redirect to the title's page
// (/film/<slug>/) when Letterboxd has it, and answer with a "not found" page otherwise. So one request, with the
// redirect not followed, tells whether there is a page and where. It has no API without a key, and this is the
// only thing asked of it.

var (
	reTmdbMovie = regexp.MustCompile(`^https://www\.themoviedb\.org/movie/(\d+)$`)
	reLbFilm    = regexp.MustCompile(`^/film/[a-z0-9-]+/?$`)
)

// needLetterboxd says whether b still lacks its Letterboxd answer; the caller holds a.mu.
func (j *lookupJob) needLetterboxd(b *core.Art) bool {
	cfg := j.a.cfg
	return b != nil && cfg.LetterboxdLink && cfg.LinkButton && b.Source != "override" && b.Letterboxd == "" &&
		!b.LbChecked && !j.a.missed(j.lbKey)
}

// letterboxd adds the title's Letterboxd page to base, or marks that it has none. A request that fails is tried
// again after a while; a title with no id to ask by counts as having no page.
func (j *lookupJob) letterboxd(base *core.Art) {
	a := j.a
	a.mu.Lock()
	need := j.needLetterboxd(base)
	a.mu.Unlock()
	if !need {
		return
	}
	imdb := ""
	if base.IDs != nil {
		imdb = base.IDs.Imdb
	}
	if imdb == "" {
		imdb = imdbIDFrom(base.URL)
	}
	if imdb == "" {
		if found, _ := a.findImdbId(base, j.media); found != nil {
			imdb = found.id
		}
	}
	var paths []string
	if imdb != "" {
		paths = append(paths, "/imdb/"+imdb+"/")
	}
	if m := reTmdbMovie.FindStringSubmatch(base.URL); m != nil && base.Type == "movie" {
		paths = append(paths, "/tmdb/"+m[1]+"/") // TMDB's show ids are another numbering: films only
	}
	failed := false
	for _, p := range paths {
		page, ok := a.letterboxdPage(p)
		if page != "" {
			base.Letterboxd = page
			a.log("INFO", `Letterboxd page for "`+base.Name+`" found.`)
			return
		}
		failed = failed || !ok
	}
	if failed {
		j.miss(j.lbKey, 30*time.Minute)
		return
	}
	base.LbChecked = true
}

// letterboxdPage asks Letterboxd for one id path. ok is false when it gave no clear answer (an error, a block, an
// unexpected reply), so that the title is asked again later.
func (a *Artwork) letterboxdPage(path string) (page string, ok bool) {
	base := strings.TrimRight(a.cfg.LetterboxdBase, "/")
	u := base + path
	a.net.reserveGap(hostOf(u))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := a.net.doRequest(ctx, u, reqInit{NoRedirect: true})
	if err != nil {
		return "", false
	}
	res.Body.Close()
	switch {
	case res.StatusCode >= 300 && res.StatusCode < 400:
		loc, err := res.Request.URL.Parse(res.Header.Get("Location"))
		if err != nil || !sameHost(loc, base) || !reLbFilm.MatchString(loc.Path) {
			return "", false
		}
		loc.RawQuery, loc.Fragment = "", ""
		return loc.String(), true
	case res.StatusCode == 200 || res.StatusCode == 404:
		return "", true // Letterboxd has no page for this id
	}
	return "", false
}

// sameHost reports whether u is on the host of base.
func sameHost(u *url.URL, base string) bool {
	b, err := url.Parse(base)
	return err == nil && strings.EqualFold(u.Host, b.Host)
}

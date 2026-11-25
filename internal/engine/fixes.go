package engine

import "github.com/CptPakundo/MPRP/internal/artwork"

// matchFinder is the cover and title finder the window's Fix this match dialog searches with: the running one (its
// request gaps and caches are shared with the lookups), else one made for the current settings, with no cache file.
func (e *Engine) matchFinder() *artwork.Artwork {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != nil && e.art != nil {
		return e.art
	}
	cfg := e.cfg
	return artwork.New(&cfg, artwork.Options{Log: e.log, Client: e.opts.HTTPClient})
}

// MatchSearch is the Fix this match dialog's search (artwork.Suggest), with the names of the catalogs it asks (none:
// every catalog is off under Privacy, or lookups are off).
func (e *Engine) MatchSearch(query string) (list []artwork.Suggestion, catalogs []string, err error) {
	f := e.matchFinder()
	for _, p := range f.SuggestCatalogs() {
		catalogs = append(catalogs, p.Label)
	}
	list, err = f.Suggest(query)
	return list, catalogs, err
}

// MatchCovers are the other catalogs' covers of a title pinned in the dialog (artwork.Covers).
func (e *Engine) MatchCovers(name string, year int, kind, except string) []artwork.Cover {
	return e.matchFinder().Covers(name, year, kind, except)
}

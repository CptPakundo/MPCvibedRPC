package artwork

import (
	"testing"

	"github.com/CptPakundo/MPRP/internal/core"
)

// A junk name (a bare number such as a Blu-ray's "00062") is never looked up, unless the user gave it a pinned
// cover or an alias in Fix a match.
func TestFixAMatchWinsOverJunk(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.ArtworkSources, cfg.EpisodeSources, cfg.RichInfo = []string{}, []string{}, false // no catalog is asked
	media := core.ParseMedia("00062.m2ts", &cfg, nil)
	if !media.Junk {
		t.Fatalf("00062.m2ts should be a junk name: %+v", media)
	}
	if art := New(&cfg, Options{}).Lookup(media); art != nil {
		t.Fatalf("a junk name without a fix was looked up: %+v", art)
	}

	const poster = "https://images.example/sample-movie.jpg"
	cfg.ArtworkOverrides = map[string]string{"00062": poster}
	art := New(&cfg, Options{}).Lookup(media)
	if art == nil || art.Poster != poster || art.Source != "override" {
		t.Fatalf("the pinned cover of a junk name was not used: %+v", art)
	}

	cfg.ArtworkOverrides = map[string]string{}
	cfg.ArtworkAliases = map[string]string{"00062": "Sample Movie"}
	if a := New(&cfg, Options{}); !a.fixed(media.Title) || a.fixed("00063") {
		t.Fatal("an alias should mark exactly its own title as fixed")
	}
}

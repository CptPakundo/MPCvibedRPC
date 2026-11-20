package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// Discord shows two buttons: the second opens another page of the title, never the first one again.
func TestSecondButton(t *testing.T) {
	cfg := DefaultConfig()
	imdb := "https://www.imdb.com/title/tt0000001/"
	cases := []struct {
		name      string
		art       *Art
		letterbox bool
		want      []string // the buttons' labels
	}{
		{"IMDb only", &Art{URL: imdb, URLLabel: "IMDb"}, false, []string{"View on IMDb"}},
		{"Letterboxd and IMDb", &Art{URL: imdb, URLLabel: "IMDb", Letterboxd: "https://letterboxd.com/film/sample-movie/"}, true,
			[]string{"View on Letterboxd", "View on IMDb"}},
		{"AniList with an IMDb id", &Art{URL: "https://anilist.co/anime/1", URLLabel: "AniList", IDs: &ArtIDs{Imdb: "tt0000002"}}, false,
			[]string{"View on AniList", "View on IMDb"}},
		{"TVmaze show with an IMDb id", &Art{URL: "https://www.tvmaze.com/shows/7", URLLabel: "TVmaze", Type: "series", IDs: &ArtIDs{Imdb: "tt0000003", Tvmaze: "7"}}, false,
			[]string{"View on TVmaze", "View on IMDb"}},
		{"IMDb show with a TVmaze id", &Art{URL: imdb, URLLabel: "IMDb", Type: "series", IDs: &ArtIDs{Imdb: "tt0000001", Tvmaze: "7"}}, false,
			[]string{"View on IMDb", "View on TVmaze"}},
	}
	for _, c := range cases {
		cfg.LetterboxdLink = c.letterbox
		info := &Info{File: "Sample Movie (2020).mkv", State: 2, Position: 1000, Duration: 6000000, Rate: 1}
		a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, c.art)
		AddLinks(a, &cfg, c.art)
		var got []string
		for _, b := range a.Buttons {
			got = append(got, b.Label)
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: buttons %v, want %v", c.name, got, c.want)
		}
		if len(a.Buttons) == 2 && samePage(a.Buttons[0].URL, a.Buttons[1].URL) {
			t.Errorf("%s: both buttons open %s", c.name, a.Buttons[0].URL)
		}
	}
	off := DefaultConfig()
	off.LinkButton = false
	art := &Art{URL: imdb, Letterboxd: "https://letterboxd.com/film/sample-movie/", IDs: &ArtIDs{Tvmaze: "7"}}
	if u, _ := SecondLink(&off, art); u != "" {
		t.Errorf("linkButton off: second link %q", u)
	}
}

// The player's logo opens the player's site, under the links switch, and goes with the other links when Discord
// refuses them.
func TestLogoLinksToThePlayer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SmallImageKey = PlayerIcon("mpv")
	art := &Art{URL: "https://www.imdb.com/title/tt0000001/", Poster: "https://img.example/p.jpg"}
	info := &Info{File: "Sample Movie (2020).mkv", State: 2, Position: 1000, Duration: 6000000, Rate: 1}
	a := BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
	AddLinks(a, &cfg, art)
	if a.Assets == nil || a.Assets.SmallURL != "https://mpv.io" {
		t.Fatalf("assets %+v", a.Assets)
	}
	if !HasLinks(a) || WithoutLinks(a).Assets.SmallURL != "" {
		t.Error("the logo's link must count as a link and go with the others")
	}
	if PlayerSite("mpc-hc") != "" || PlayerSite("https://img.example/mpv.png") != "" {
		t.Error("only a player logo has a site")
	}
	for name := range playerSites {
		if _, ok := playerIconByFile(name); !ok {
			t.Errorf("%s has a site but no logo", name)
		}
	}
	cfg.LinkButton = false
	b := BuildActivity(info, &cfg, 1_700_000_000_000, nil, art)
	AddLinks(b, &cfg, art)
	if j, _ := json.Marshal(b); strings.Contains(string(j), "small_url") {
		t.Errorf("linkButton off: %s", j)
	}
}

func playerIconByFile(name string) (string, bool) {
	for p, n := range playerIcons {
		if n == name {
			return p, true
		}
	}
	return "", false
}

// A cover address Discord would refuse (over 300 characters) is left out instead of costing the whole status.
func TestLongCoverAddressIsLeftOut(t *testing.T) {
	cfg := DefaultConfig()
	long := "https://img.example/" + strings.Repeat("a", 290) + ".jpg"
	art := &Art{URL: "https://www.imdb.com/title/tt0000001/", Poster: long}
	a := BuildActivity(&Info{File: "Sample Movie (2020).mkv", State: 2, Position: 1000, Duration: 6000000, Rate: 1}, &cfg, 1_700_000_000_000, nil, art)
	if a.Assets != nil && a.Assets.LargeImage == long {
		t.Errorf("a %d-character cover address was sent", len(long))
	}
}

package core

import "testing"

// Under the link switch both text lines and the cover open the catalog page; WithoutLinks takes every link out again.
func TestAddLinksAndWithoutLinks(t *testing.T) {
	const page = "https://www.imdb.com/title/tt0000001/"
	art := &Art{URL: page, Poster: "https://img.example/poster.jpg"}
	newActivity := func() *Activity {
		return &Activity{Details: "Sample Show", State: "S01E02 · Episode title", Assets: &Assets{LargeImage: art.Poster}}
	}

	a := newActivity()
	AddLinks(a, &Config{LinkButton: true}, art)
	if a.DetailsURL != page || a.StateURL != page || a.Assets.LargeURL != page || !HasLinks(a) {
		t.Fatalf("links: %+v %+v", a, a.Assets)
	}
	c := WithoutLinks(a)
	if c.DetailsURL != "" || c.StateURL != "" || c.Assets.LargeURL != "" || HasLinks(c) {
		t.Fatalf("WithoutLinks left a link: %+v %+v", c, c.Assets)
	}
	if a.StateURL != page || a.Assets.LargeURL != page {
		t.Fatal("WithoutLinks changed the original")
	}

	off := newActivity()
	AddLinks(off, &Config{LinkButton: false}, art)
	if HasLinks(off) {
		t.Fatalf("links with the switch off: %+v", off)
	}

	empty := &Activity{Details: "Sample Movie"}
	AddLinks(empty, &Config{LinkButton: true}, art)
	if empty.StateURL != "" {
		t.Fatal("a missing state line got a link")
	}
}

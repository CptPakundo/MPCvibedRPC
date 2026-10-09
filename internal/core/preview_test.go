package core

import "testing"

func TestPreviewOf(t *testing.T) {
	cfg := DefaultConfig()
	if PreviewOf(nil, &cfg) != nil {
		t.Fatal("nil activity, nil preview")
	}
	a := &Activity{Type: 3, Name: "Some Show", Details: "S01E02 · Pilot", State: "Drama · ★ 8.1",
		Timestamps: &Timestamps{Start: 1000, End: 5000},
		Assets:     &Assets{LargeImage: "https://img.example/p.jpg", LargeText: "Some Show", SmallImage: "mpc-hc", SmallText: "Media Player Classic"},
		Buttons:    []Button{{Label: "View on IMDb", URL: "https://imdb.example/x"}}}
	p := PreviewOf(a, &cfg)
	if !p.Watching || p.Name != "Some Show" || p.Details != "S01E02 · Pilot" || p.State != "Drama · ★ 8.1" {
		t.Errorf("text parts: %+v", p)
	}
	if p.LargeImage != "https://img.example/p.jpg" || p.LargeText != "Some Show" || p.SmallText != "Media Player Classic" {
		t.Errorf("images: %+v", p)
	}
	if p.Start != 1000 || p.End != 5000 || p.Button != "View on IMDb" {
		t.Errorf("times/button: %+v", p)
	}

	// an uploaded Discord asset (not an address) gives no image to load; the name falls back to the app name
	b := &Activity{Details: "Movie", Assets: &Assets{LargeImage: "mpc-hc"}}
	q := PreviewOf(b, &cfg)
	if q.Watching || q.Name != cfg.AppName || q.LargeImage != "" {
		t.Errorf("asset-key preview: %+v", q)
	}
	// only web addresses are ever handed to the page
	for _, bad := range []string{"javascript:alert(1)", "file:///etc/passwd", "data:image/png;base64,AAAA", "//host/x.jpg"} {
		if r := PreviewOf(&Activity{Assets: &Assets{LargeImage: bad}}, &cfg); r.LargeImage != "" {
			t.Errorf("%q must not become an image address", bad)
		}
	}
}
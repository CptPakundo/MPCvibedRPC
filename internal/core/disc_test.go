package core

import "testing"

func TestDiscTitle(t *testing.T) {
	cfg := DefaultConfig()
	for _, c := range []struct{ path, file, title string }{
		{`D:\Movies\Sample Movie (2020)\BDMV\STREAM\00062.m2ts`, "00062.m2ts", "Sample Movie"},
		{`D:\Movies\Sample Movie (2020)\Disc 1\BDMV\STREAM\00062.m2ts`, "00062.m2ts", "Sample Movie"},
		{`D:\Rips\Sample.Movie.2020.1080p.BluRay\BDMV\PLAYLIST\00800.mpls`, "00800.mpls", "Sample Movie"},
		{`/Volumes/SAMPLE_MOVIE_D1/BDMV/STREAM/00001.m2ts`, "00001.m2ts", "SAMPLE MOVIE"},
		{`D:\DVD\Sample Movie (1999)\VIDEO_TS\VTS_01_1.VOB`, "VTS_01_1.VOB", "Sample Movie"},
		{`/media/user/SAMPLE_FILM/VIDEO_TS/VTS_02_3.VOB`, "VTS_02_3.VOB", "SAMPLE FILM"},
	} {
		in := NewInfo(c.file, c.path, "", 2, 0, 0, 1)
		m := ParseMedia(in.File, &cfg, in.Dir)
		if m.Title != c.title || m.Junk || !m.FromFolder {
			t.Errorf("%s: title %q junk=%v fromFolder=%v, want %q", c.path, m.Title, m.Junk, m.FromFolder, c.title)
		}
	}
	if m := ParseMedia("00062.m2ts", &cfg, NewInfo("00062.m2ts", `D:\Movies\Sample Movie (2020)\BDMV\STREAM\00062.m2ts`, "", 2, 0, 0, 1).Dir); !has(m.Year) || m.Year != 2020 {
		t.Errorf("the folder's year should be read: %+v", m)
	}

	// No title to be had: the number stays, and stays junk (not looked up unless Fix a match names it).
	for _, p := range []string{
		`E:\BDMV\STREAM\00062.m2ts`,                       // a disc's root: the engine reads its name from the disc
		`E:\PRIVATE\AVCHD\BDMV\STREAM\00000.MTS`,          // a camcorder
		`/media/user/BDROM/BDMV/STREAM/00062.m2ts`,        // a generic label
		`D:\Movies\Disc 2\BDMV\STREAM\00062.m2ts`,         // nothing above "Disc 2"
		`D:\Movies\Sample Movie (2020)\STREAM\00062.m2ts`, // STREAM alone is no disc
	} {
		in := NewInfo("x", p, "", 2, 0, 0, 1)
		if title, ok := DiscTitle(in.Dir); ok {
			t.Errorf("%s: no disc title expected, got %q", p, title)
		}
	}

	// useFolderName off: folders are not read, the disc's too.
	off := DefaultConfig()
	off.UseFolderName = false
	if m := ParseMedia("00062.m2ts", &off, []string{"STREAM", "BDMV", "Sample Movie (2020)"}); m.Title != "00062" || !m.Junk {
		t.Errorf("useFolderName off: %+v", m)
	}
}

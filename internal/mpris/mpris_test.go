package mpris

import (
	"strings"
	"testing"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/core"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus"
	"github.com/CptPakundo/MPCvibedRPC/internal/dbus/dbustest"
)

func TestPlayerName(t *testing.T) {
	cases := map[string]string{
		"org.mpris.MediaPlayer2.vlc":                                  "VLC",
		"org.mpris.MediaPlayer2.vlc.instance4242":                     "VLC",
		"org.mpris.MediaPlayer2.mpv":                                  "mpv",
		"org.mpris.MediaPlayer2.mpv.instance77":                       "mpv",
		"org.mpris.MediaPlayer2.io.github.celluloid_player.Celluloid": "Celluloid",
		"org.mpris.MediaPlayer2.haruna":                               "Haruna",
		"org.mpris.MediaPlayer2.smplayer.instance12":                  "SMPlayer",
		"org.mpris.MediaPlayer2.com.github.rafostar.Clapper":          "Clapper",
		"org.mpris.MediaPlayer2.org.gnome.Showtime":                   "Showtime",
		// not video players, or not MPRIS at all
		"org.mpris.MediaPlayer2.spotify":                    "",
		"org.mpris.MediaPlayer2.firefox.instance_1_23":      "",
		"org.mpris.MediaPlayer2.chromium.instance5":         "",
		"org.mpris.MediaPlayer2.rhythmbox":                  "",
		"org.mpris.MediaPlayer2.vlcfan":                     "", // a part must match exactly
		"org.freedesktop.DBus":                              "",
		"org.example.vlc":                                   "",
		"org.mpris.MediaPlayer2.Player":                     "",
		"org.mpris.MediaPlayer2.instance1":                  "",
		"org.mpris.MediaPlayer2.org.kde.dragonplayer":       "Dragon Player",
		"org.mpris.MediaPlayer2.io.github.mpc_qt.mpc-qt":    "MPC-QT",
		"org.mpris.MediaPlayer2.Totem":                      "GNOME Videos",
		"org.mpris.MediaPlayer2.kodi":                       "Kodi",
		"org.mpris.MediaPlayer2.org.kde.haruna.instance123": "Haruna",
	}
	for in, want := range cases {
		if got := PlayerName(in); got != want {
			t.Errorf("PlayerName(%q) = %q, want %q", in, got, want)
		}
	}
}

func meta(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func TestToInfo(t *testing.T) {
	// as VLC sends it: a file URL with escapes, the length in microseconds, the position likewise
	in := ToInfo(map[string]any{
		"PlaybackStatus": "Playing", "Rate": 1.0, "Position": int64(61_500_000),
		"Metadata": meta("xesam:url", "file:///home/someone/Videos/Sample%20Show/Sample%20Show%20S01E02.mkv",
			"xesam:title", "Sample Show S01E02.mkv", "mpris:length", int64(1_440_000_000)),
	})
	want := core.NewInfo("Sample Show S01E02.mkv", "/home/someone/Videos/Sample Show/Sample Show S01E02.mkv", "/home/someone/Videos/Sample Show", 2, 61500, 1440000, 1)
	if !same(in, want) {
		t.Errorf("VLC-style: %+v, want %+v", in, want)
	}
	if in.Dir[0] != "Sample Show" {
		t.Errorf("folder: %v", in.Dir)
	}

	// paused, with an unsigned length and a double speed
	in = ToInfo(map[string]any{
		"PlaybackStatus": "Paused", "Rate": 2.0, "Position": int64(1_000_000),
		"Metadata": meta("xesam:url", "file:///media/Sample%20Movie%20(2020).mp4", "mpris:length", uint64(6_000_000_000)),
	})
	if in.State != 1 || in.Duration != 6_000_000 || in.Rate != 2 || in.File != "Sample Movie (2020).mp4" || in.Position != 1000 {
		t.Errorf("paused: %+v", in)
	}

	// a stream: the title names it
	in = ToInfo(map[string]any{
		"PlaybackStatus": "Playing", "Metadata": meta("xesam:url", "https://example.com/live.m3u8?token=x", "xesam:title", "Sample Stream"),
	})
	if in.File != "Sample Stream" || in.FullDir != "" || in.State != 2 || in.Rate != 1 {
		t.Errorf("stream: %+v", in)
	}
	// a stream without a title
	in = ToInfo(map[string]any{"PlaybackStatus": "Playing", "Metadata": meta("xesam:url", "https://example.com/v.mp4")})
	if in.File != "https://example.com/v.mp4" {
		t.Errorf("untitled stream: %+v", in)
	}
	// a title without a URL (some players leave the URL out)
	in = ToInfo(map[string]any{"PlaybackStatus": "Playing", "Metadata": meta("xesam:title", "Sample Show S01E03")})
	if in.File != "Sample Show S01E03" || in.State != 2 {
		t.Errorf("title only: %+v", in)
	}

	// nothing loaded, stopped, or a nonsense speed
	for _, p := range []map[string]any{
		{"PlaybackStatus": "Stopped", "Metadata": meta("xesam:url", "file:///a.mkv")},
		{"PlaybackStatus": "Playing", "Metadata": map[string]any{}},
		{"PlaybackStatus": "Playing"},
		{},
	} {
		if in := ToInfo(p); in.State != -1 || in.File != "" {
			t.Errorf("%v: %+v, want nothing playing", p, in)
		}
	}
	if in := ToInfo(map[string]any{"PlaybackStatus": "Playing", "Rate": 0.0, "Metadata": meta("xesam:title", "x")}); in.Rate != 1 {
		t.Errorf("rate 0 should read as 1, got %v", in.Rate)
	}
}

func same(a, b *core.Info) bool {
	return a.File == b.File && a.FullDir == b.FullDir && a.State == b.State && a.Position == b.Position &&
		a.Duration == b.Duration && a.Rate == b.Rate && strings.Join(a.Dir, "|") == strings.Join(b.Dir, "|")
}

func playerReply(props map[string]any) dbustest.Handler {
	return func(m *dbus.Message) dbustest.Reply {
		if m.Interface != propsIface || m.Member != "GetAll" || m.Path != objectPath || len(m.Body) != 1 || m.Body[0] != playerIface {
			return dbustest.Reply{Error: "org.freedesktop.DBus.Error.UnknownMethod"}
		}
		out := map[string]any{}
		for k, v := range props {
			out[k] = v
		}
		return dbustest.Reply{Sig: "a{sv}", Body: []any{out}}
	}
}

func v(sig string, val any) dbus.Variant { return dbus.Variant{Sig: sig, Value: val} }

func TestQuery(t *testing.T) {
	bus := dbustest.New(t)
	vlc := map[string]any{
		"PlaybackStatus": v("s", "Playing"), "Rate": v("d", 1.0), "Position": v("x", int64(5_000_000)),
		"Metadata": v("a{sv}", map[string]any{
			"xesam:url":     v("s", "file:///videos/Sample%20Show%20S01E02.mkv"),
			"mpris:length":  v("x", int64(1_200_000_000)),
			"mpris:trackid": v("o", "/org/videolan/vlc/playlist/1"),
		}),
	}
	bus.Own("org.mpris.MediaPlayer2.vlc", playerReply(vlc))
	bus.Own("org.mpris.MediaPlayer2.spotify", playerReply(vlc))              // a music player: never asked
	bus.Own("org.mpris.MediaPlayer2.firefox.instance_1_9", playerReply(vlc)) // a browser: never asked
	bus.Own("org.mpris.MediaPlayer2.mpv.instance12", playerReply(map[string]any{"PlaybackStatus": v("s", "Stopped")}))
	bus.Own("org.mpris.MediaPlayer2.haruna", func(*dbus.Message) dbustest.Reply { // broken: left out
		return dbustest.Reply{Error: "org.freedesktop.DBus.Error.Failed"}
	})

	got := Query(bus.Address, time.Second)
	if len(got) != 2 {
		t.Fatalf("got %d players: %+v", len(got), got)
	}
	// bus-name order: mpv.instance12 before vlc
	if got[0].Player != "mpv" || got[0].State != -1 {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].Player != "VLC" || got[1].File != "Sample Show S01E02.mkv" || got[1].State != 2 || got[1].Position != 5000 || got[1].Duration != 1_200_000 {
		t.Errorf("second: %+v", got[1])
	}
	for _, c := range bus.Calls() {
		if strings.Contains(c, "spotify") || strings.Contains(c, "firefox") {
			t.Errorf("a non-video player was asked: %s", c)
		}
	}
}

func TestQueryWithoutBus(t *testing.T) {
	if got := Query("", time.Second); got != nil {
		t.Errorf("no address: %v", got)
	}
	if got := Query("unix:path=/nonexistent/bus", 200*time.Millisecond); got != nil {
		t.Errorf("no bus: %v", got)
	}
}

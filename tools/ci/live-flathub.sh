#!/usr/bin/env bash
# The live engine test (internal/engine/live_test.go) against Linux players as their makers release them, from
# Flathub, rather than the older ones Ubuntu packages (live-players.sh): VLC and Celluloid through MPRIS, each with
# the test's own fake Discord, and a pause that must show at once. The weekly canary runs it, inside a session bus
# of its own:   dbus-run-session -- tools/ci/live-flathub.sh
set -euo pipefail
videos="$HOME/Videos" # VLC may read it; Celluloid is given it below (its sandbox opens only what it is given)
mkdir -p "$videos"
video="$videos/Sample Show S01E02.mkv"
ffmpeg -loglevel error -f lavfi -i "testsrc=duration=600:size=320x240:rate=5" -f lavfi -i "sine=duration=600" \
  -c:v libx264 -preset ultrafast -c:a aac -shortest "$video"
app="" # the player started last
cleanup() { [ -z "$app" ] || flatpak kill "$app" 2> /dev/null || true; }
trap cleanup EXIT

live() {
  local name="$1"; shift
  echo "== $name ($(flatpak info "$app" | sed -n 's/^ *Version: //p'))"
  bash "$(dirname "$0")/must-pass.sh" TestLivePlayer env MPCRPC_LIVE_PLAYER="$name" MPCRPC_LIVE_TITLE="Sample Show" \
    MPCRPC_LIVE_MPV_PIPE=/nonexistent/none MPCRPC_LIVE_EVENTS=mpris ${1+"$@"} go test -count=1 -timeout 5m -run TestLivePlayer -v ./internal/engine
}
stop_last() { flatpak kill "$app" 2> /dev/null || true; app=""; sleep 2; }
wait_bus() { for _ in $(seq 1 240); do dbus-send --session --print-reply --dest=org.freedesktop.DBus / org.freedesktop.DBus.ListNames | grep -qE "\"$1" && return 0; sleep 0.25; done; echo "$1 is not on the bus"; return 1; }

# wait_playing NAME-PATTERN: the player's MPRIS PlaybackStatus must become Playing (else it shows what it says)
wait_playing() {
  local name status=""
  name="$(dbus-send --session --print-reply --dest=org.freedesktop.DBus / org.freedesktop.DBus.ListNames | grep -oE "\"$1[^\"]*" | head -1 | tr -d '"')"
  for _ in $(seq 1 80); do
    status="$(dbus-send --session --print-reply --dest="$name" /org/mpris/MediaPlayer2 org.freedesktop.DBus.Properties.Get string:org.mpris.MediaPlayer2.Player string:PlaybackStatus 2>&1 | grep -oE '"[A-Za-z]+"' | tr -d '"' || true)"
    [ "$status" = Playing ] && return 0
    sleep 0.25
  done
  echo "$name does not play (PlaybackStatus: ${status:-none})"
  return 1
}

app=org.videolan.VLC
xvfb-run -a flatpak run --command=vlc "$app" --intf dummy --extraintf dbus --no-video --aout dummy --loop "$video" > /dev/null 2>&1 &
wait_bus org.mpris.MediaPlayer2.vlc
live VLC
stop_last

app=io.github.celluloid_player.Celluloid
xvfb-run -a flatpak run --filesystem="$videos:ro" "$app" --mpv-options="--ao=null --loop-file=inf" "$video" > /dev/null 2>&1 &
wait_bus "org.mpris.MediaPlayer2.*[Cc]elluloid"
wait_playing "org.mpris.MediaPlayer2.*[Cc]elluloid"
live Celluloid
stop_last

echo "live Flathub player tests passed"
[ -z "${PASSED_FILE:-}" ] || touch "$PASSED_FILE" # proof for CI that the script got to the end

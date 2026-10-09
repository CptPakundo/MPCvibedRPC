#!/usr/bin/env bash
# Runs the live engine test (internal/engine/live_test.go) against real players on a CI machine, each with the test's
# own fake Discord. Linux: mpv over its IPC socket, VLC and mpv (mpv-mpris) through MPRIS, Celluloid through MPRIS.
# macOS: mpv over its IPC socket, then IINA after the real program has set it up ("Set up the player connection").
# Linux runs it inside its own session bus:   dbus-run-session -- tools/ci/live-players.sh
set -euo pipefail
os="$(uname -s)"
work="$(mktemp -d)"
video="$work/Sample Show S01E02.mkv"
ffmpeg -loglevel error -f lavfi -i "testsrc=duration=600:size=320x240:rate=5" -f lavfi -i "sine=duration=600" \
  -c:v libx264 -preset ultrafast -c:a aac -shortest "$video"
tmp="${TMPDIR:-/tmp}"; tmp="${tmp%/}"
pid="" # the player started last (one at a time; macOS has bash 3.2, so no arrays)
cleanup() { [ -z "$pid" ] || kill "$pid" 2> /dev/null || true; }
trap cleanup EXIT

# live NAME [VAR=value...]: the engine must find the player under NAME and show the sample title
live() {
  local name="$1"; shift
  echo "== $name"
  env MPCRPC_LIVE_PLAYER="$name" MPCRPC_LIVE_TITLE="Sample Show" ${1+"$@"} go test -count=1 -run TestLivePlayer -v ./internal/engine
}
stop_last() { kill "$pid" 2> /dev/null || true; wait "$pid" 2> /dev/null || true; pid=""; sleep 1; }
wait_socket() { for _ in $(seq 1 80); do [ -S "$1" ] && return 0; sleep 0.25; done; echo "no socket at $1"; return 1; }
wait_bus() { for _ in $(seq 1 80); do dbus-send --session --print-reply --dest=org.freedesktop.DBus / org.freedesktop.DBus.ListNames | grep -qE "\"$1" && return 0; sleep 0.25; done; echo "$1 is not on the bus"; return 1; }

# mpv over its IPC socket (both systems)
mpv --no-config --vo=null --ao=null --loop-file=inf --input-ipc-server="$tmp/mpvsocket" "$video" > /dev/null 2>&1 &
pid=$!
wait_socket "$tmp/mpvsocket"
live mpv
stop_last

if [ "$os" = Linux ]; then
  none="MPCRPC_LIVE_MPV_PIPE=/nonexistent/none" # so that mpv can only be found through MPRIS below

  cvlc --intf dummy --extraintf dbus --no-video --aout dummy --loop "$video" > /dev/null 2>&1 &
  pid=$!
  wait_bus org.mpris.MediaPlayer2.vlc
  live VLC "$none"
  stop_last

  mpv --vo=null --ao=null --loop-file=inf "$video" > /dev/null 2>&1 & # the mpv-mpris script is loaded from /etc/mpv/scripts
  pid=$!
  wait_bus org.mpris.MediaPlayer2.mpv
  live mpv "$none"
  stop_last

  xvfb-run -a celluloid --mpv-options="--ao=null --loop-file=inf" "$video" > /dev/null 2>&1 &
  pid=$!
  wait_bus "org.mpris.MediaPlayer2.*[Cc]elluloid"
  live Celluloid "$none"
  stop_last
  pkill -x celluloid || true # xvfb-run does not pass the signal on
else
  echo "== IINA: the real program sets it up"
  go build -o "$work/MPCvibedRPC" ./cmd/mpcvibedrpc
  export MPCRPC_HOME="$work/home" MPCRPC_BROWSER=none MPCRPC_FOREGROUND=1
  mkdir -p "$MPCRPC_HOME"
  printf '%s' '{"app":{"openWindow":false,"startPresence":false,"checkUpdates":false,"welcomeSeen":true},"port":1}' > "$MPCRPC_HOME/config.json"
  "$work/MPCvibedRPC" &
  app=$!
  for _ in $(seq 1 60); do [ -s "$MPCRPC_HOME/ipc.json" ] && break; sleep 0.25; done
  port="$(jq -r .port "$MPCRPC_HOME/ipc.json")"; token="$(jq -r .token "$MPCRPC_HOME/ipc.json")"
  curl -sf -H "X-Token: $token" -H 'Content-Type: application/json' -X POST --data '{"closeMpc":false}' "http://127.0.0.1:$port/api/mpc-web" | tee "$work/setup.json"
  echo
  curl -sf -H "X-Token: $token" -H 'Content-Type: application/json' -X POST --data '{}' "http://127.0.0.1:$port/api/quit" > /dev/null
  wait "$app" || true
  defaults read com.colliderli.iina enableAdvancedSettings | grep -qx 1 || { echo "IINA's advanced settings are off"; exit 1; }
  defaults read com.colliderli.iina userOptions | tee /dev/stderr | grep -q "input-ipc-server" || { echo "IINA has no input-ipc-server"; exit 1; }
  # a second setup keeps what is there
  "$work/MPCvibedRPC" &
  app=$!
  for _ in $(seq 1 60); do [ -s "$MPCRPC_HOME/ipc.json" ] && break; sleep 0.25; done
  port="$(jq -r .port "$MPCRPC_HOME/ipc.json")"; token="$(jq -r .token "$MPCRPC_HOME/ipc.json")"
  curl -sf -H "X-Token: $token" -H 'Content-Type: application/json' -X POST --data '{"closeMpc":false}' "http://127.0.0.1:$port/api/mpc-web" | grep -q "IINA was already set up" || { echo "the second setup should find IINA set up"; exit 1; }
  curl -sf -H "X-Token: $token" -H 'Content-Type: application/json' -X POST --data '{}' "http://127.0.0.1:$port/api/quit" > /dev/null
  wait "$app" || true
  unset MPCRPC_HOME MPCRPC_BROWSER MPCRPC_FOREGROUND

  open -a IINA "$video"
  wait_socket "$tmp/iina-mpvsocket" || { ls -la "$tmp"; exit 1; }
  live IINA
  osascript -e 'tell application "IINA" to quit' || true
fi
echo "live player tests passed"
[ -z "${PASSED_FILE:-}" ] || touch "$PASSED_FILE" # proof for CI that the script got to the end

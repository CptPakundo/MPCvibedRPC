#!/usr/bin/env bash
# Smoke test of the real program on macOS or Linux, in throwaway folders: start, API, run at login on and off, player
# setup, diagnostics, a second start, quit. On macOS, given the .app, it also opens the app the way the Finder does
# and checks that the program keeps running and that opening it again does not start a second copy.
#   usage: smoke-unix.sh PROGRAM VERSION [APP]
set -euo pipefail
exe="$1"; version="$2"; app="${3:-}"
os="$(uname -s)"
work="$(mktemp -d)"
export MPCRPC_HOME="$work/home" MPCRPC_BROWSER=none MPCRPC_FOREGROUND=1
export XDG_CONFIG_HOME="$work/config" # mpv.conf and the Linux autostart entry are written here, not in the real home
mkdir -p "$MPCRPC_HOME" "$XDG_CONFIG_HOME/mpv" # an mpv folder: mpv counts as present for the setup
quiet='{"app":{"openWindow":false,"startPresence":false,"checkUpdates":false,"welcomeSeen":true},"port":1}'
printf '%s' "$quiet" > "$MPCRPC_HOME/config.json"

fail() { echo "::error title=smoke-unix::$*"; [ -f "$MPCRPC_HOME/mpcvibedrpc.log" ] && tail -n 40 "$MPCRPC_HOME/mpcvibedrpc.log"; exit 1; }
wait_for() { # wait_for "what" command...
  local what="$1"; shift
  for _ in $(seq 1 60); do "$@" && return 0; sleep 0.25; done
  fail "timed out waiting for $what"
}
have_ipc() { [ -s "$MPCRPC_HOME/ipc.json" ] && curl -sf "http://127.0.0.1:$(jq -r .port "$MPCRPC_HOME/ipc.json")/api/ping" > /dev/null; }
port=""; token=""
read_ipc() { port="$(jq -r .port "$MPCRPC_HOME/ipc.json")"; token="$(jq -r .token "$MPCRPC_HOME/ipc.json")"; }
get() { curl -sf -m 60 -H "X-Token: $token" "http://127.0.0.1:$port/api/$1"; }
post() { curl -sf -m 60 -H "X-Token: $token" -H 'Content-Type: application/json' -X POST --data "${2:-"{}"}" "http://127.0.0.1:$port/api/$1"; }
gone() { ! kill -0 "$1" 2> /dev/null; }

echo "== start"
"$exe" --version | grep -qx "$version" || fail "--version is not $version"
"$exe" &
pid=$!
wait_for "the program to answer" have_ipc
read_ipc
[ "$(curl -sf "http://127.0.0.1:$port/api/ping" | jq -r .version)" = "$version" ] || fail "ping: wrong version"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'X-Token: wrong' "http://127.0.0.1:$port/api/state")" = 401 ] || fail "a wrong token must be refused"

state="$(get state)"
want_os=linux; [ "$os" = Darwin ] && want_os=darwin
[ "$(jq -r .os <<< "$state")" = "$want_os" ] || fail "state.os"
[ "$(jq -r .canAutoStart <<< "$state")" = true ] || fail "run at login should be available"
keys="$(jq -r '[.schema[].fields[].key] | join(" ")' <<< "$state")"
if [ "$want_os" = darwin ]; then
  [[ " $keys " == *" iinaPipe "* && " $keys " != *" mpris "* ]] || fail "macOS shows the IINA setting only: $keys"
else
  [[ " $keys " == *" mpris "* && " $keys " != *" iinaPipe "* ]] || fail "Linux shows the MPRIS setting only: $keys"
fi
echo "state ok ($want_os)"

echo "== run at login"
if [ "$want_os" = darwin ]; then item_dir="$HOME/Library/LaunchAgents"; item_glob="io.github.cptpakundo.mpcvibedrpc-*.plist"
else item_dir="$XDG_CONFIG_HOME/autostart"; item_glob="mpcvibedrpc-*.desktop"; fi
[ "$(post save '{"app":{"autoStart":true}}' | jq -r .autoStart)" = true ] || fail "autoStart did not switch on"
item="$(find "$item_dir" -name "$item_glob" 2> /dev/null | head -n 1)"
[ -n "$item" ] || fail "no login item in $item_dir"
grep -qF -- "--background" "$item" || fail "the login item does not start in the background"
grep -qF -- "$MPCRPC_HOME" "$item" || fail "the login item does not keep the data folder"
if [ "$want_os" = darwin ]; then plutil -lint "$item"; else command -v desktop-file-validate > /dev/null && desktop-file-validate "$item"; fi
cat "$item"
[ "$(post save '{"app":{"autoStart":false}}' | jq -r .autoStart)" = false ] || fail "autoStart did not switch off"
[ ! -e "$item" ] || fail "the login item was not removed"
echo "login item ok"

echo "== player setup"
r="$(post mpc-web '{"closeMpc":false}')"
echo "$r"
if [ "$want_os" = darwin ] && [ ! -d /Applications/IINA.app ]; then
  ! grep -q IINA <<< "$r" || fail "IINA is not installed here, yet the setup touched it: $r"
  ! defaults read com.colliderli.iina userOptions > /dev/null 2>&1 || fail "settings were written for an IINA that is not installed"
fi
grep -qE "^input-ipc-server=/.+/mpvsocket$" "$XDG_CONFIG_HOME/mpv/mpv.conf" || fail "mpv.conf has no absolute input-ipc-server: $(cat "$XDG_CONFIG_HOME/mpv/mpv.conf")"
r="$(post mpc-web '{"closeMpc":false}')"
jq -e '.message | contains("already set up")' <<< "$r" > /dev/null || fail "a second setup should find mpv set up: $r"

echo "== diagnostics"
diag="$(get diagnostics | jq -r .text)"
grep -q "MPCvibedRPC diagnostics" <<< "$diag" || fail "diagnostics"
! grep -qF "$work" <<< "$diag" || fail "diagnostics leak a path"
grep -m1 -i "system" <<< "$diag" || true

echo "== second start hands over"
"$exe" || fail "the second start failed"
kill -0 "$pid" || fail "the first copy died"

echo "== presence on and off (nothing plays: Discord stays on standby)"
[ "$(post start | jq -r .running)" = true ] || fail "start"
[ "$(post stop | jq -r .running)" = false ] || fail "stop"

echo "== quit"
post quit > /dev/null
wait_for "the program to quit" gone "$pid"
[ ! -e "$MPCRPC_HOME/ipc.json" ] || fail "ipc.json left behind"

if [ -n "$app" ]; then
  echo "== macOS: open the app like the Finder does"
  unset MPCRPC_FOREGROUND MPCRPC_BROWSER # the app's own window this time
  swift tools/ci/windows.swift > /dev/null # compiled once, so the checks below are quick
  windows() { swift tools/ci/windows.swift | awk -F'\t' -v p="$1" -v l="$2" '$1 == p && $2 == l' | wc -l | tr -d ' '; }
  icons() { swift tools/ci/windows.swift | awk -F'	' '$2 == 25' | wc -l | tr -d ' '; } # menu bar icons, whoever draws them (macOS 26: Control Center)
  shot="${SCREENSHOT:-$work/screen.png}"
  icons_before="$(icons)"
  open --env "MPCRPC_HOME=$MPCRPC_HOME" "$app"
  wait_for "the app to answer" have_ipc
  read_ipc
  server="$(jq -r .pid "$MPCRPC_HOME/ipc.json")"
  sleep 5
  kill -0 "$server" || fail "the program did not keep running after the launcher left"
  ps -o args= -p "$server" | grep -q -- "--serve" || fail "the running copy should be the --serve one"
  [ "$(get state | jq -r .tray)" = true ] || fail "the program says it has no menu bar icon"
  screencapture -x "${shot%.png}-menubar.png" 2> /dev/null || true
  icons_after="$(icons)"
  echo "menu bar icons: $icons_before before, $icons_after after"
  [ "$(windows "$server" 25)" -ge 1 ] || [ "$icons_after" -gt "$icons_before" ] || { swift tools/ci/windows.swift; fail "no menu bar icon on screen"; }
  echo "menu bar icon ok"
  [ "$(windows "$server" 0)" = 0 ] || fail "a window is open although openWindow is off"

  echo "== macOS: the settings window"
  open "$app" # opening the app again: macOS tells the running app, which shows its window
  sleep 5
  [ "$(jq -r .pid "$MPCRPC_HOME/ipc.json")" = "$server" ] || fail "opening the app again started another copy"
  n="$(pgrep -f "MPCvibedRPC.app/Contents/MacOS/MPCvibedRPC" | wc -l | tr -d ' ')"
  [ "$n" = 1 ] || fail "$n copies are running"
  [ "$(windows "$server" 0)" = 1 ] || { swift tools/ci/windows.swift; fail "the app should show exactly one window of its own"; }
  ! pgrep -f -- "--user-data-dir=$MPCRPC_HOME/window" > /dev/null || fail "a browser was started for the window"
  post open > /dev/null # once more: the same window comes forward, no second one
  sleep 2
  [ "$(windows "$server" 0)" = 1 ] || fail "a second window was opened"
  if screencapture -x "$shot" 2> /dev/null; then echo "screenshot: $shot"; fi
  echo "window ok"
  grep -c "starting" "$MPCRPC_HOME/mpcvibedrpc.log" || true
  post quit > /dev/null
  wait_for "the app to quit" gone "$server"
fi
echo "smoke test passed"
[ -z "${PASSED_FILE:-}" ] || touch "$PASSED_FILE" # proof for CI that the script got to the end

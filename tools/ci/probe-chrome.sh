#!/usr/bin/env bash
# Temporary probe: how many windows does Chrome on macOS open for --app with a fresh profile, per launch variant?
set -uo pipefail
chrome="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
[ -x "$chrome" ] || { echo "no Chrome"; exit 0; }
"$chrome" --version
url='data:text/html,<title>probe</title><h1>probe</h1>'
base=(--window-size=620,700 --no-first-run --no-default-browser-check --disable-background-mode --disable-features=msStartupBoost)
swift tools/ci/windows.swift > /dev/null 2>&1 # compile once

count() { # count PROFILE: the windows of the browser that uses this profile
  local pids; pids="$(pgrep -f -- "--user-data-dir=$1" | tr '\n' '|' | sed 's/|$//')"
  [ -n "$pids" ] || { echo "  (no process)"; return; }
  swift tools/ci/windows.swift | awk -F'\t' -v p="^($pids)\$" '$1 ~ p && $2 == 0 { n++; print "  window " $4 } END { print "  layer-0 windows: " n+0 }'
}
stop() { pkill -f -- "--user-data-dir=$1"; sleep 2; }

run() { # run NAME PROFILE command...
  local name="$1" prof="$2"; shift 2
  echo "== $name"
  "$@" > /dev/null 2>&1 &
  sleep 8
  count "$prof"
  stop "$prof"
}

p="$(mktemp -d)/a"; run "A: as the program does it (fresh profile)" "$p" "$chrome" --app="$url" "${base[@]}" --user-data-dir="$p"
run "A2: same profile again (after SIGTERM)" "$p" "$chrome" --app="$url" "${base[@]}" --user-data-dir="$p"
p="$(mktemp -d)/b"; run "B: + --no-startup-window" "$p" "$chrome" --app="$url" "${base[@]}" --user-data-dir="$p" --no-startup-window
p="$(mktemp -d)/c"; run "C: through open -na" "$p" open -na "Google Chrome" --args --app="$url" "${base[@]}" --user-data-dir="$p"
p="$(mktemp -d)/d"; run "D: + --disable-session-crashed-bubble --hide-crash-restore-bubble" "$p" "$chrome" --app="$url" "${base[@]}" --user-data-dir="$p" --hide-crash-restore-bubble --disable-session-crashed-bubble
p="$(mktemp -d)/e"; run "E: + --new-window" "$p" "$chrome" --new-window --app="$url" "${base[@]}" --user-data-dir="$p"
p="$(mktemp -d)/f"; run "F: --app first, no other flags" "$p" "$chrome" --app="$url" --user-data-dir="$p"
echo "probe done"

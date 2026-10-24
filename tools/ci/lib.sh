#!/usr/bin/env bash
# Shared pieces of the CI scripts. Source it:   . "$(dirname "$0")/lib.sh"
# macOS has bash 3.2: no arrays, no negative indexes, nothing newer than that in here.

# ci_video PATH [SECONDS]: writes a generated test video with sound at PATH (its folder is made; the generic name is
# the caller's job). The default length is ten minutes.
ci_video() {
  local secs="${2:-600}"
  mkdir -p "$(dirname "$1")"
  ffmpeg -loglevel error -f lavfi -i "testsrc=duration=$secs:size=320x240:rate=5" -f lavfi -i "sine=duration=$secs" \
    -c:v libx264 -preset ultrafast -c:a aac -shortest "$1"
}

# wait_socket PATH [TRIES]: waits, a quarter of a second per try, until the Unix socket at PATH exists.
wait_socket() {
  local i
  for i in $(seq 1 "${2:-240}"); do [ -S "$1" ] && return 0; sleep 0.25; done
  echo "no socket at $1"; return 1
}

# wait_bus NAME-PATTERN [TRIES]: waits until a name matching the pattern is on the session bus.
wait_bus() {
  local i
  for i in $(seq 1 "${2:-80}"); do
    dbus-send --session --print-reply --dest=org.freedesktop.DBus / org.freedesktop.DBus.ListNames | grep -qE "\"$1" && return 0
    sleep 0.25
  done
  echo "$1 is not on the bus"; return 1
}

# done_file: the proof for CI that the script got to its very end (run-annotated.sh fails the step without it when
# PASSED_FILE is set). Call it as the last line of a script, never earlier.
done_file() { [ -z "${PASSED_FILE:-}" ] || touch "$PASSED_FILE"; }

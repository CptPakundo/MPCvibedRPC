#!/usr/bin/env bash
# Runs the live engine test (internal/engine/live_test.go) against a real Plex Media Server on a Linux CI machine, with
# the test's own fake Discord: the official server image, unclaimed (no Plex account), that lets this machine in
# without signing in; a generated sample episode; and a stand-in player that reports playback to the server the way
# Plex apps do (/:/timeline). The engine follows the server's play notifications at a fixed address, without plex.tv.
set -euo pipefail
work="$(mktemp -d)"
P=http://127.0.0.1:32400
player=(-H "Accept: application/json" -H "X-Plex-Client-Identifier: ci-sample-player" -H "X-Plex-Product: CI Player"
  -H "X-Plex-Device-Name: CI" -H "X-Plex-Platform: Linux" -H "X-Plex-Version: 1.0")
cleanup() { [ -z "${feed:-}" ] || kill "$feed" 2> /dev/null || true; docker rm -f pms > /dev/null 2>&1 || true; }
trap cleanup EXIT

mkdir -p "$work/media/TV/Sample Show/Season 01" "$work/config"
ffmpeg -loglevel error -f lavfi -i "testsrc=duration=300:size=320x240:rate=5" -f lavfi -i "sine=duration=300" \
  -c:v libx264 -preset ultrafast -c:a aac -shortest "$work/media/TV/Sample Show/Season 01/Sample Show - S01E02.mkv"
docker run -d --name pms --network host -e TZ=UTC -e "ALLOWED_NETWORKS=127.0.0.1/255.255.255.255" \
  -v "$work/config:/config" -v "$work/media:/data" plexinc/pms-docker:latest > /dev/null

# up once /identity has a machine identifier and no start-up state
for i in $(seq 1 150); do
  s="$(curl -s "$P/identity" || true)"
  if echo "$s" | grep -q machineIdentifier && ! echo "$s" | grep -q startState; then break; fi
  [ "$i" = 150 ] && { echo "the Plex server did not start"; docker logs pms 2>&1 | tail -n 30; exit 1; }
  sleep 2
done
echo "Plex Media Server $(echo "$s" | sed -E 's/.*"version":"([^"]*)".*/\1/')"

curl -sf -o /dev/null -X POST "${player[@]}" \
  "$P/library/sections?name=TV&type=show&agent=tv.plex.agents.none&scanner=Plex%20TV%20Series&language=xn&location=%2Fdata%2FTV"
curl -sf -o /dev/null "${player[@]}" "$P/library/sections/1/refresh"
rk=""
for i in $(seq 1 90); do
  rk="$(curl -s "${player[@]}" "$P/library/all?type=4" | sed -nE 's/.*"ratingKey":"([0-9]+)".*/\1/p')"
  [ -n "$rk" ] && break
  [ "$i" = 90 ] && { echo "the sample episode was not added"; exit 1; }
  sleep 2
done
echo "sample episode: $rk"

# the stand-in player: reports playback every 3 seconds, like a Plex app
(
  t=0
  while :; do
    curl -s -o /dev/null "${player[@]}" "$P/:/timeline?ratingKey=$rk&key=%2Flibrary%2Fmetadata%2F$rk&state=playing&time=$t&duration=300000"
    t=$((t + 3000)); sleep 3
  done
) &
feed=$!

MPCRPC_LIVE_PLAYER=Plex MPCRPC_LIVE_TITLE="Sample Show - S01E02" MPCRPC_LIVE_PLEX_ADDRESS="$P" \
  go test -count=1 -timeout 3m -run TestLivePlayer -v ./internal/engine

[ -z "${PASSED_FILE:-}" ] || touch "$PASSED_FILE"

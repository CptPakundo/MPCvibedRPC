#!/usr/bin/env bash
# Runs the live tests (internal/jellyfin/live_test.go and the engine's TestLivePlayer) against a real Jellyfin or Emby
# server on a Linux CI machine, with the engine test's own fake Discord: the official server image, set up through its
# first-run wizard with a made-up user; a generated sample episode; and a stand-in player that reports playback the way
# Jellyfin and Emby apps do (/Sessions/Playing, then /Sessions/Playing/Progress every few seconds).
#   usage: live-jellyfin.sh jellyfin|emby
set -euo pipefail
kind="${1:?jellyfin or emby}"
case "$kind" in
  jellyfin) image=jellyfin/jellyfin:latest; product=Jellyfin; port=8096 ;;
  emby) image=emby/embyserver:latest; product=Emby; port=8097 ;;
  *) echo "unknown server: $kind"; exit 2 ;;
esac
work="$(mktemp -d)"
S="http://127.0.0.1:$port"
user=sample
pass="ci-$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n')"
hdr() { echo "MediaBrowser Client=\"CI Player\", Device=\"CI\", DeviceId=\"ci-player-$kind\", Version=\"1.0\"${1:+, Token=\"$1\"}"; }
call() { # call METHOD PATH [JSON] [TOKEN]
  local h; h="$(hdr "${4:-}")"
  curl -sS --fail-with-body -X "$1" -H "Authorization: $h" -H "X-Emby-Authorization: ${h/MediaBrowser/Emby}" ${4:+-H "X-Emby-Token: $4"} \
    -H "Content-Type: application/json" -H "Accept: application/json" ${3:+--data "$3"} "$S$2"
}
cleanup() { [ -z "${feed:-}" ] || kill "$feed" 2> /dev/null || true; docker rm -f "ci-$kind" > /dev/null 2>&1 || true; }
trap cleanup EXIT

# "Example", not "Sample": Emby skips any file with "sample" in its name (release groups use it for preview clips)
mkdir -p "$work/media/TV/Example Show/Season 01" "$work/config" "$work/cache"
ffmpeg -loglevel error -f lavfi -i "testsrc=duration=300:size=320x240:rate=5" -f lavfi -i "sine=duration=300" \
  -c:v libx264 -preset ultrafast -c:a aac -shortest "$work/media/TV/Example Show/Season 01/Example Show - S01E02.mkv"
chmod -R a+rwX "$work"
if [ "$kind" = jellyfin ]; then
  docker run -d --name "ci-$kind" -p "$port:8096" -v "$work/config:/config" -v "$work/cache:/cache" -v "$work/media:/media" "$image" > /dev/null
else
  docker run -d --name "ci-$kind" -p "$port:8096" -e UID="$(id -u)" -e GID="$(id -g)" -v "$work/config:/config" -v "$work/media:/media" "$image" > /dev/null
fi

for i in $(seq 1 120); do
  info="$(curl -s "$S/System/Info/Public" || true)"
  echo "$info" | grep -q '"Id"' && break
  [ "$i" = 120 ] && { echo "the $product server did not start"; docker logs "ci-$kind" 2>&1 | tail -n 40; exit 1; }
  sleep 2
done
echo "$product server: $info"

# the first-run wizard, as the web page does it
call GET /Startup/Configuration > /dev/null || true
call POST /Startup/Configuration '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}' > /dev/null
call GET /Startup/User > /dev/null || true
call POST /Startup/User "{\"Name\":\"$user\",\"Password\":\"$pass\"}" > /dev/null
call POST /Startup/RemoteAccess '{"EnableRemoteAccess":true,"EnableAutomaticPortMapping":false}' > /dev/null || true
call POST /Startup/Complete > /dev/null
sleep 3

auth="$(call POST /Users/AuthenticateByName "{\"Username\":\"$user\",\"Pw\":\"$pass\"}")"
token="$(echo "$auth" | jq -r '.AccessToken // empty')"
uid="$(echo "$auth" | jq -r '.User.Id // empty')"
[ -n "$token" ] && [ -n "$uid" ] || { echo "the stand-in player could not sign in: $auth"; exit 1; }
echo "stand-in player signed in (user $uid)"

call POST "/Library/VirtualFolders?name=Shows&collectionType=tvshows&paths=%2Fmedia%2FTV&refreshLibrary=true" \
  '{"LibraryOptions":{"PathInfos":[{"Path":"/media/TV"}]}}' "$token" > /dev/null || { echo "adding the library failed"; exit 1; }
echo "libraries: $(call GET /Library/VirtualFolders "" "$token" | jq -c '[.[] | {Name, Locations}]' || true)"
item=""
for i in $(seq 1 90); do
  # the video itself (not the show or season folders); Emby does not answer IncludeItemTypes=Episode here
  items="$(call GET "/Users/$uid/Items?Recursive=true&Fields=MediaType" "" "$token" || true)"
  item="$(echo "$items" | jq -r '[.Items[] | select(.IsFolder != true)][0].Id // empty' 2> /dev/null || true)"
  [ -n "$item" ] && break
  if [ "$i" = 5 ] || [ "$i" = 45 ]; then
    # Emby only checks the folders when a library is added; its "Scan media library" task adds the videos
    scan="$(call GET /ScheduledTasks "" "$token" | jq -r '.[] | select(.Key == "RefreshLibrary") | .Id' | head -n 1 || true)"
    if [ -n "$scan" ]; then call POST "/ScheduledTasks/Running/$scan" "" "$token" > /dev/null || true; fi
  fi
  if [ "$i" = 90 ]; then
    echo "the sample episode was not added: $items"
    call GET "/Users/$uid/Items?Recursive=true" "" "$token" | jq -c '[.Items[] | {Name, Type, IsFolder, Id}]' || true
    ls -laR "$work/media"
    docker exec "ci-$kind" ls -la "/media/TV/Example Show/Season 01" || true
    for p in $(call GET "/Users/$uid/Items?Recursive=true" "" "$token" | jq -r '.Items[] | select(.Type == "Season") | .Id'); do
      echo "season $p: $(call GET "/Users/$uid/Items?ParentId=$p" "" "$token" | head -c 800)"
    done
    docker logs "ci-$kind" 2>&1 | tail -n 20
    for l in "$work"/config/logs/*.txt; do echo "== $l"; grep -v -E "PRAGMA|SQLITE_DBCONFIG|Opening sqlite|journal_mode" "$l" | tail -n 120; done
    exit 1
  fi
  sleep 2
done
echo "sample episode: $item"
echo "$items" | head -c 600; echo

# the stand-in player: starts playing, then reports its position every 3 seconds, like a Jellyfin or Emby app
call POST /Sessions/Playing "{\"ItemId\":\"$item\",\"MediaSourceId\":\"$item\",\"PlaySessionId\":\"ci-play-1\",\"PositionTicks\":0,\"IsPaused\":false,\"CanSeek\":true,\"PlayMethod\":\"DirectPlay\"}" "$token" || {
  echo "the server refused the start of playback"
  for l in "$work"/config/logs/*.txt; do grep -i -E -B2 -A12 "Sessions/Playing|Exception" "$l" | tail -n 60; done
  exit 1
}
(
  t=0
  while :; do
    sleep 3; t=$((t + 30000000))
    call POST /Sessions/Playing/Progress "{\"ItemId\":\"$item\",\"MediaSourceId\":\"$item\",\"PlaySessionId\":\"ci-play-1\",\"PositionTicks\":$t,\"IsPaused\":false,\"EventName\":\"timeupdate\",\"CanSeek\":true,\"PlayMethod\":\"DirectPlay\"}" "$token" > /dev/null || true
  done
) &
feed=$!
sleep 4

export MPCRPC_LIVE_JELLYFIN_ADDRESS="$S" MPCRPC_LIVE_JELLYFIN_USER="$user" MPCRPC_LIVE_JELLYFIN_PASSWORD="$pass"
export MPCRPC_LIVE_JELLYFIN_PRODUCT="$product" MPCRPC_LIVE_TITLE="Example Show"
go test -count=1 -timeout 3m -run TestLiveJellyfin -v ./internal/jellyfin
MPCRPC_LIVE_PLAYER="$product" go test -count=1 -timeout 3m -run TestLivePlayer -v ./internal/engine

[ -z "${PASSED_FILE:-}" ] || touch "$PASSED_FILE"

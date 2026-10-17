#!/usr/bin/env bash
# The live engine test against real PotPlayer and Kodi on a GitHub Windows runner, with the test's own fake Discord.
# Both are installed from Chocolatey; a generated test video (a generic name) is played; nothing else is changed.
# Writes $PASSED_FILE at the very end, so a step that dies early can never pass.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
work="$RUNNER_TEMP/live-windows"
mkdir -p "$work"

echo "::group::install"
choco install -y --no-progress kodi ffmpeg > "$work/choco.log" || { cat "$work/choco.log"; exit 1; }
# PotPlayer replaces the installer at its fixed download address, so the package's recorded checksum goes stale; on
# this throwaway runner it is installed without that check.
choco install -y --no-progress --ignore-checksums potplayer >> "$work/choco.log" || { cat "$work/choco.log"; exit 1; }
tail -5 "$work/choco.log"
echo "::endgroup::"

video="$work/Sample Movie (2020).mkv"
ffmpeg -loglevel error -y -f lavfi -i testsrc=duration=900:size=320x240:rate=10 -c:v libx264 -preset ultrafast "$video"
win_video="$(cygpath -w "$video")"

live() { # live NAME [VAR=value ...]: the live engine test, which must really run and pass
  local name="$1"; shift
  env MPCRPC_LIVE_PLAYER="$name" MPCRPC_LIVE_TITLE="Sample Movie" "$@" \
    "$here/must-pass.sh" TestLivePlayer go test -count=1 -v -run TestLivePlayer ./internal/engine
}

echo "::group::PotPlayer"
pot="$(ls /c/Program\ Files/DAUM/PotPlayer/PotPlayerMini64.exe /c/Program\ Files*/DAUM/PotPlayer/PotPlayerMini*.exe 2>/dev/null | head -1)"
[ -n "$pot" ] || { echo "PotPlayer was not installed where expected"; ls "/c/Program Files" "/c/Program Files (x86)"; exit 1; }
"$pot" "$win_video" &
sleep 10
live PotPlayer
taskkill //F //IM "$(basename "$pot")" > /dev/null || true
echo "::endgroup::"

echo "::group::Kodi"
userdata="$APPDATA/Kodi/userdata"
mkdir -p "$userdata"
cat > "$userdata/guisettings.xml" <<'XML'
<settings version="2">
    <setting id="services.webserver">true</setting>
    <setting id="services.webserverport">8080</setting>
    <setting id="services.webserverauthentication">true</setting>
    <setting id="services.webserverusername">kodi</setting>
    <setting id="services.webserverpassword">ci-secret</setting>
</settings>
XML
kodi="$(ls /c/Program\ Files/Kodi/kodi.exe /c/Program\ Files*/Kodi/kodi.exe 2>/dev/null | head -1)"
[ -n "$kodi" ] || { echo "Kodi was not installed where expected"; exit 1; }
"$kodi" &
rpc() { curl -s -m 5 -u kodi:ci-secret -H 'Content-Type: application/json' -d "$1" http://127.0.0.1:8080/jsonrpc; }
for i in $(seq 1 60); do
  if rpc '{"jsonrpc":"2.0","id":1,"method":"JSONRPC.Ping"}' | grep -q pong; then break; fi
  sleep 2
done
rpc '{"jsonrpc":"2.0","id":1,"method":"JSONRPC.Ping"}' | grep -q pong || { echo "Kodi's web server did not answer"; cat "$APPDATA/Kodi/kodi.log" 2>/dev/null | tail -40; exit 1; }
file_json="$(printf '%s' "$win_video" | sed 's/\\/\\\\/g')"
rpc "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"Player.Open\",\"params\":{\"item\":{\"file\":\"$file_json\"}}}"
echo
sleep 8
live Kodi MPCRPC_LIVE_KODI_PASSWORD=ci-secret
taskkill //F //IM kodi.exe > /dev/null || true
echo "::endgroup::"

touch "$PASSED_FILE"

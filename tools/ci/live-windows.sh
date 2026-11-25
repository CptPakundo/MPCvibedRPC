#!/usr/bin/env bash
# The live engine test against real PotPlayer and Kodi on a GitHub Windows runner, with the test's own fake Discord.
# Both are installed from Chocolatey; a generated test video (a generic name) is played; nothing else is changed.
# Writes $PASSED_FILE at the very end, so a step that dies early can never pass.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
work="$RUNNER_TEMP/live-windows"
mkdir -p "$work"

# choco_install CHECK PACKAGE... [OPTION...]: choco install, tried up to three times, 30 seconds apart (a download
# from Chocolatey's mirrors fails now and then, for example with a TLS error for Kodi). CHECK is a shell test that must
# hold afterwards: choco has answered success with the program missing, so its exit code alone is not trusted.
choco_install() {
  local check="$1" try; shift
  for try in 1 2 3; do
    if choco install -y --no-progress "$@" >> "$work/choco.log" 2>&1 && eval "$check"; then return 0; fi
    echo "choco install $* did not leave what it should ($check), try $try of 3"
    [ "$try" = 3 ] || sleep 30
  done
  grep -iE "install|fail|error|warn|not " "$work/choco.log" | grep -viE "proceeds|community|chocolatey.org/compare|awesome" | tail -60
  return 1
}

# ffmpeg: Chocolatey puts its shim in its bin folder, which this shell may not have on PATH yet
export PATH="$PATH:/c/ProgramData/chocolatey/bin"
pot_exe() { ls /c/Program\ Files/DAUM/PotPlayer/PotPlayerMini64.exe /c/Program\ Files*/DAUM/PotPlayer/PotPlayerMini*.exe 2>/dev/null | head -1; }

echo "::group::install"
: > "$work/choco.log"
choco_install 'command -v ffmpeg >/dev/null && ls /c/Program\ Files*/Kodi/kodi.exe >/dev/null 2>&1' kodi ffmpeg
# PotPlayer replaces the installer at its fixed download address, so the package's recorded checksum goes stale; on
# this throwaway runner it is installed without that check.
choco_install '[ -n "$(pot_exe)" ]' potplayer --ignore-checksums
tail -5 "$work/choco.log"
echo "::endgroup::"

video="$work/Sample Movie (2020).mkv"
ffmpeg -loglevel error -y -f lavfi -i testsrc=duration=900:size=320x240:rate=10 -c:v libx264 -preset ultrafast "$video"
win_video="$(cygpath -w "$video")"

live() { # live NAME [VAR=value ...]: the live engine test, which must really run and pass
  local name="$1"; shift
  env MPRP_LIVE_PLAYER="$name" MPRP_LIVE_TITLE="Sample Movie" "$@" \
    "$here/must-pass.sh" TestLivePlayer go test -count=1 -v -run TestLivePlayer ./internal/engine
}

echo "::group::PotPlayer"
pot="$(pot_exe || true)"
[ -n "$pot" ] || { echo "PotPlayer was not installed where expected"; ls "/c/Program Files" "/c/Program Files (x86)"; exit 1; }
"$pot" "$win_video" &
sleep 10
live PotPlayer
MPRP_LIVE_POTPLAYER=1 "$here/must-pass.sh" TestLivePotPlayer go test -count=1 -v -run TestLivePotPlayer ./internal/potplayer
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
kodi="$(ls /c/Program\ Files/Kodi/kodi.exe /c/Program\ Files*/Kodi/kodi.exe 2>/dev/null | head -1 || true)"
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
live Kodi MPRP_LIVE_KODI_PASSWORD=ci-secret MPRP_LIVE_EVENTS=kodi # also: a pause shows at once, from Kodi's notification
taskkill //F //IM kodi.exe > /dev/null || true
echo "::endgroup::"

touch "$PASSED_FILE"

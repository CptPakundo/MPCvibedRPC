#!/usr/bin/env bash
# TEMPORARY probe (removed before merge): what a real, unclaimed Plex Media Server sends, to build the reader on.
set -u
T="${RUNNER_TEMP:-/tmp}"
P=http://127.0.0.1:32400
H=(-H "Accept: application/json" -H "X-Plex-Client-Identifier: ci-probe-player" -H "X-Plex-Product: CI Player" -H "X-Plex-Device-Name: CI" -H "X-Plex-Platform: Linux" -H "X-Plex-Version: 1.0")
show() { echo "=== $1"; }

: show "plex.tv pin" # recorded in the first run
if false; then
pin=$(curl -s -X POST -H "Accept: application/json" -H "X-Plex-Product: MPCvibedRPC" -H "X-Plex-Client-Identifier: ci-probe-0001" "https://plex.tv/api/v2/pins?strong=true")
echo "$pin" | sed -E 's/"code":"[^"]*"/"code":"(code)"/'
id=$(echo "$pin" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])' 2>/dev/null || echo "")
if [ -n "$id" ]; then
  show "plex.tv pin check"
  curl -s -H "Accept: application/json" -H "X-Plex-Client-Identifier: ci-probe-0001" "https://plex.tv/api/v2/pins/$id" | sed -E 's/"code":"[^"]*"/"code":"(code)"/'
  echo
  show "plex.tv user without token"
  curl -s -o /dev/null -w "%{http_code}\n" -H "Accept: application/json" -H "X-Plex-Client-Identifier: ci-probe-0001" "https://plex.tv/api/v2/user"
fi
fi

mkdir -p "$T/plexmedia/Movies/Sample Movie (2020)" "$T/plexmedia/TV/Sample Show/Season 01" "$T/plexcfg"
ffmpeg -loglevel error -y -f lavfi -i testsrc=duration=300:size=320x240:rate=25 -f lavfi -i sine=duration=300 -c:v libx264 -preset ultrafast -c:a aac -shortest "$T/plexmedia/Movies/Sample Movie (2020)/Sample Movie (2020).mkv"
cp "$T/plexmedia/Movies/Sample Movie (2020)/Sample Movie (2020).mkv" "$T/plexmedia/TV/Sample Show/Season 01/Sample Show - S01E02.mkv"
docker run -d --name pms --network host -e TZ=UTC -e "ALLOWED_NETWORKS=127.0.0.1/255.255.255.255" \
  -v "$T/plexcfg:/config" -v "$T/plexmedia:/data" plexinc/pms-docker:latest > /dev/null
for i in $(seq 1 150); do curl -s "$P/identity" | grep -q machineIdentifier && break; sleep 2; done
echo "ready after $i tries"
show "identity"; curl -s "${H[@]}" "$P/identity"; echo
show "root"; curl -s "${H[@]}" "$P/" | head -c 1500; echo
show "sessions (none yet)"; curl -s -w " [%{http_code}]" "${H[@]}" "$P/status/sessions"; echo

show "create sections"
curl -s -w " [%{http_code}]\n" -X POST "${H[@]}" "$P/library/sections?name=Movies&type=movie&agent=tv.plex.agents.none&scanner=Plex%20Video%20Files%20Scanner&language=xn&location=%2Fdata%2FMovies"
curl -s -w " [%{http_code}]\n" -X POST "${H[@]}" "$P/library/sections?name=TV&type=show&agent=tv.plex.agents.none&scanner=Plex%20Series%20Scanner&language=xn&location=%2Fdata%2FTV"
rk=""; ek=""
for i in $(seq 1 60); do
  rk=$(curl -s "${H[@]}" "$P/library/sections/1/all" | python3 -c 'import json,sys; m=json.load(sys.stdin)["MediaContainer"].get("Metadata",[]); print(m[0]["ratingKey"] if m else "")' 2>/dev/null)
  ek=$(curl -s "${H[@]}" "$P/library/sections/2/allLeaves" | python3 -c 'import json,sys; m=json.load(sys.stdin)["MediaContainer"].get("Metadata",[]); print(m[0]["ratingKey"] if m else "")' 2>/dev/null)
  [ -n "$rk" ] && [ -n "$ek" ] && break; sleep 2
done
echo "movie ratingKey=$rk episode ratingKey=$ek"
show "movie metadata"; curl -s "${H[@]}" "$P/library/metadata/$rk"; echo
show "episode metadata"; curl -s "${H[@]}" "$P/library/metadata/$ek"; echo
part=$(curl -s "${H[@]}" "$P/library/metadata/$ek" | python3 -c 'import json,sys; print(json.load(sys.stdin)["MediaContainer"]["Metadata"][0]["Media"][0]["Part"][0]["key"])' 2>/dev/null)
echo "part=$part"

show "listeners"
( timeout 70 curl -sN -H "Accept: text/event-stream" "$P/:/eventsource/notifications?filters=playing" > "$T/sse.txt" 2> "$T/sse.err"; echo "sse exit $?" >> "$T/sse.err" ) &
( timeout 70 curl -sN -i "$P/:/eventsource/notifications" > "$T/sse-all.txt" 2>&1 ) &
pip install -q websocket-client > /dev/null 2>&1
cat > "$T/ws.py" <<'EOF'
import websocket, sys, time
out = open(sys.argv[1], "w")
ws = websocket.create_connection("ws://127.0.0.1:32400/:/websockets/notifications", timeout=70)
end = time.time() + 65
while time.time() < end:
    try:
        out.write(ws.recv() + "\n"); out.flush()
    except Exception as e:
        out.write("ERR %r\n" % e); break
EOF
( timeout 70 python3 "$T/ws.py" "$T/ws.txt" 2> "$T/ws.err" ) &
sleep 3

show "play: fetch part + timelines"
curl -s -o /dev/null -w "part %{http_code} %{size_download}\n" -r 0-200000 "${H[@]}" "$P$part"
for t in 0 4000 8000; do
  curl -s -w " [%{http_code}]\n" "${H[@]}" "$P/:/timeline?ratingKey=$ek&key=%2Flibrary%2Fmetadata%2F$ek&state=playing&time=$t&duration=300000&playbackTime=$t"
  sleep 4
done
show "sessions while playing"; curl -s -w " [%{http_code}]" "${H[@]}" "$P/status/sessions"; echo
show "sessions while playing (xml)"; curl -s "$P/status/sessions" | head -c 3000; echo
curl -s -w " [%{http_code}]\n" "${H[@]}" "$P/:/timeline?ratingKey=$ek&key=%2Flibrary%2Fmetadata%2F$ek&state=paused&time=9000&duration=300000"
sleep 4
show "sessions while paused"; curl -s "${H[@]}" "$P/status/sessions"; echo
sleep 12
show "sessions paused 16s later"; curl -s "${H[@]}" "$P/status/sessions" | head -c 400; echo
curl -s -w " [%{http_code}]\n" "${H[@]}" "$P/:/timeline?ratingKey=$ek&key=%2Flibrary%2Fmetadata%2F$ek&state=stopped&time=9000&duration=300000"
sleep 3
show "sessions after stop"; curl -s "${H[@]}" "$P/status/sessions"; echo
show "movie as a second player"
H2=(-H "Accept: application/json" -H "X-Plex-Client-Identifier: ci-probe-player-2" -H "X-Plex-Product: CI Player" -H "X-Plex-Device-Name: CI2" -H "X-Plex-Platform: Linux")
curl -s -w " [%{http_code}]\n" "${H2[@]}" "$P/:/timeline?ratingKey=$rk&key=%2Flibrary%2Fmetadata%2F$rk&state=playing&time=1000&duration=300000"
sleep 3
show "sessions movie"; curl -s "${H[@]}" "$P/status/sessions"; echo
wait
show "sse (filters=playing)"; cat "$T/sse.txt"; cat "$T/sse.err"
show "sse (all, first 4000)"; head -c 4000 "$T/sse-all.txt"; echo
show "websocket"; head -c 6000 "$T/ws.txt"; cat "$T/ws.err"
show "pms log tail"; docker logs pms 2>&1 | tail -n 20
docker rm -f pms > /dev/null
echo "probe done"

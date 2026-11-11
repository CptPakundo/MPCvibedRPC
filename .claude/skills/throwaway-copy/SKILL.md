---
name: throwaway-copy
description: How to run the real MPCvibedRPC program, a live engine test or a real player safely - throwaway MPCRPC_HOME copies, a stand-in update server, portable players, the Plex, Stremio and Jellyfin/Emby fakes. Use before starting any copy of the program or any real player.
---
# Running the real program safely
- Test copies keep `checkUpdates` off, which also keeps them from installing anything: with it
  on, a copy without a window installs what its update server offers ten seconds after its start and shows a real
  notification. To try the dialog, start with it off, open the page, then switch it on there (that looks at once).
- `MPCRPC_HOME=<scratch folder>` gives a copy its own data folder **and its own autostart value name**, so it can never
  touch an installed copy. A config such as
  `{"app":{"openWindow":false,"startPresence":false,"checkUpdates":false,"welcomeSeen":true}}` keeps it quiet.
- The API port and token are in `<MPCRPC_HOME>/ipc.json`; the page is `http://127.0.0.1:<port>/?t=<token>` (the
  program itself opens its window with a one-time code, `?c=`, so the token is not on the browser's command line;
  the page keeps the token in `sessionStorage` and takes it out of its address). To test window
  states without Discord, override `window.fetch` for `/api/status` in the page.
- `api/start` with nothing playing is safe: Discord is only contacted once something plays ("standby"). To keep a real
  player from being picked up, point `port` at an unused port and leave `mpvPipe`/`vlcPassword` empty.
- Update checks: `MPCRPC_UPDATE_API=http://127.0.0.1:<port>` with `tools/ci/fake-http.ps1` (on CI; on this kind of Windows PC its
  HttpListener may be refused without admin rights, so a small Go server in a scratch folder serves the same three
  answers there).
- `MPCRPC_BROWSER=none` opens no window (tests, CI); `MPCRPC_FOREGROUND=1` keeps a macOS copy from handing over to a
  detached one.
- Live engine test against a real player, with the test's own fake Discord (opt-in):
  `MPCRPC_LIVE_PLAYER=mpv MPCRPC_LIVE_TITLE="Sample Movie" go test -count=1 -run TestLivePlayer -v ./internal/engine`
  (also `MPCRPC_LIVE_PORT`, `MPCRPC_LIVE_VLC_PASSWORD`, `MPCRPC_LIVE_VLC_PORT`, `MPCRPC_LIVE_MPV_PIPE`,
  `MPCRPC_LIVE_IINA_PIPE`, `MPCRPC_LIVE_PLEX_ADDRESS`, `MPCRPC_LIVE_PLEX_TOKEN`, `MPCRPC_LIVE_JELLYFIN_ADDRESS`, `_USER`, `_PASSWORD`; `MPCRPC_LIVE_PLAYER` is the name shown: mpv, IINA, VLC, Celluloid, Plex, ...). Use portable player builds in a scratch
  folder (MPC-QT: `portable.txt`; MPC-BE: an ini next to the exe; mpv: `portable_config\`; VLC: a `portable\` folder
  next to `vlc.exe`), verify their published checksums, and check `%APPDATA%`, `%LOCALAPPDATA%` and `HKCU\Software`
  afterwards. Test videos can be made with mpv's encoder from `lavfi` test sources, with generic names.
- Plex: never sign in to a real Plex account, and never call plex.tv's sign-in, from tests or manual checks without the
  owner's OK. The sign-in, the server list and the notifications are tested against fakes (`internal/plex`,
  `cmd/mpcvibedrpc/plex_test.go`, `internal/engine/plex_test.go`); the window's Plex states can be shown by overriding
  `window.fetch` for `/api/plex*`. The same for Stremio: no real account or call to link.stremio.com / api.strem.io
  without the owner's OK; fakes in `internal/stremio`, `cmd/mpcvibedrpc/stremio_test.go`, `internal/engine/rest_test.go`,
  and `/api/stremio*` for the window.
- Jellyfin / Emby: never sign in to a real server from tests or manual checks without the owner's OK. Fakes are in
  `internal/jellyfin`, `cmd/mpcvibedrpc/jellyfin_test.go` and `internal/engine/jellyfin_test.go`; a small fake server on
  127.0.0.1 can drive the window's sign-in in a throwaway copy (keep presence off, so Discord is never contacted).
- Resting: a throwaway copy rests whenever Discord is closed and its window is closed (no player is asked then). To
  test players with a closed Discord, keep its window open.
- Leave nothing running: quit test copies, and only stop processes you started.

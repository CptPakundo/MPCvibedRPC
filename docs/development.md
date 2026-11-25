# Development

How MPCvibedRPC is built, tested and kept working. For using it, see the [README](../README.md); for the rules that
apply to every change, [CONTRIBUTING.md](../CONTRIBUTING.md) (and [AGENTS.md](../AGENTS.md) for AI tools); for
releasing and screenshots, [maintaining.md](maintaining.md).

## Glossary
Terms these docs use:

| Term | Meaning |
|---|---|
| check, tick | One round of the engine's loop: ask the players, then update Discord if something changed (every `pollInterval`, 5 s by default). |
| nudge | A player event (pause, seek, new file) that brings the next check forward. |
| standby | "Discord on standby": showing is on, but nothing plays yet, so Discord has not been contacted. |
| resting | Discord is closed and no settings window is open: a check only looks whether Discord has started. |
| golden data | Recorded reference outputs in `internal/*/testdata/*.gz` that the parsers and lookups are checked against. Frozen: never regenerate it. |
| throwaway copy | A copy of the program run with a data folder of its own (`MPCRPC_HOME`), so it cannot touch an installed copy. See [Running the program safely](#running-the-program-safely). |
| smoke test | CI starts the real built program on its own system and checks it from outside (`tools/ci/smoke.ps1`, `smoke-unix.sh`). |
| live jobs | CI jobs that install real players and media servers on a throwaway runner and run the engine against them. |
| canary | The weekly CI run against the newest players, server images and catalog services; it opens an issue when something outside changed. |

## Building and running
The program is written in Go and uses only the standard library: one file under 10 MB (about 17 MB of memory while
running, about 3 MB while resting), no runtime, nothing to install. Building needs [Go](https://go.dev/dl/) at the
version in `go.mod` (the `go` line) and no internet beyond that.

Windows (the resource step adds the icon and version details; it is optional):

    go run ./tools/mkrsrc -version X.Y.Z -out cmd/mpcvibedrpc/rsrc_windows_amd64.syso
    go build -trimpath -buildvcs=false -ldflags "-H=windowsgui -s -w -X main.version=X.Y.Z" -o MPCvibedRPC.exe ./cmd/mpcvibedrpc

`-H=windowsgui` makes it a windowed program, so no console flashes up. You can build it from any OS
(`GOOS=windows`).

Linux:

    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "-s -w -X main.version=X.Y.Z" -o MPCvibedRPC-linux-amd64 ./cmd/mpcvibedrpc

macOS: the app is built on a Mac **with cgo** by `tools/ci/package-macos.sh VERSION OUTDIR` (`CGO_ENABLED=1`; both
processor types in one program, the app icon, an ad-hoc signature, a zip). The menu bar icon and the settings window
(WebKit) are in `internal/winsys/macapp_darwin.go`, which is only built with cgo. A build without cgo still runs, but
shows the settings in the browser and has no menu bar icon.

Flags of the program:

| Flag | What it does |
|---|---|
| `--background` | Start in the tray. |
| `--home <folder>` | A data folder of its own, like `MPCRPC_HOME`. |
| `--after-update` | Set on the copy an update started. |
| `--update-failed` | Set on the old copy that an update put back (see [Updates](#updates)). |
| `--version` | Print the version and exit. |
| `--serve` | macOS: the copy that keeps running after the app is opened (`launch_darwin.go`). |

## Running the program safely
Never touch a real setup while testing ([the rule](../CONTRIBUTING.md#ground-rules)). Use throwaway copies and the
fakes below.

- **A throwaway copy.** `MPCRPC_HOME=<scratch folder>` (or `--home <folder>`) gives a copy its own data folder **and
  its own autostart value name**, so it never touches an installed copy. A config such as
  `{"app":{"openWindow":false,"startPresence":false,"checkUpdates":false,"welcomeSeen":true}}` keeps it quiet.
- **Keep `checkUpdates` off in test copies.** With it on, a copy without a window installs what its update server
  offers ten seconds after its start and shows a real operating-system notification. To try the update dialog, start
  with it off, open the page, then switch it on there (that checks at once), or trigger a check through
  `api/update-check`.
- **The page.** The API port and token are in `<MPCRPC_HOME>/ipc.json`; the page is
  `http://127.0.0.1:<port>/?t=<token>`. (The program itself opens its window with a one-time code, `?c=`, so the token
  is not on the browser's command line; the page keeps the token in `sessionStorage` and takes it out of its address.)
  To test window states without Discord, override `window.fetch` for `/api/status` (or `/api/plex*`,
  `/api/stremio*`) in the page.
- **Showing on Discord.** `api/start` with nothing playing is safe: Discord is only contacted once something plays
  (standby). To keep a real player from being picked up, point `port` at an unused port and leave `mpvPipe` and
  `vlcPassword` empty.
- **Resting.** A throwaway copy rests whenever Discord is closed and its window is closed (no player is asked then).
  To test players with Discord closed, keep the window open.
- **Update checks.** `MPCRPC_UPDATE_API=http://127.0.0.1:<port>` with `tools/ci/fake-http.ps1` (on CI). On a Windows
  PC its HttpListener may be refused without admin rights; a small Go server in a scratch folder can serve the same
  three answers.
- **Other variables.** `MPCRPC_BROWSER=none` opens no window (tests, CI); any other value names the browser to show
  the window in (one that understands `--app`). `MPCRPC_FOREGROUND=1` keeps a macOS copy from handing over to a
  detached one.
- **Portable players.** Use portable builds in a scratch folder and verify their published checksums: MPC-QT
  (`portable.txt`), MPC-BE (an ini next to the exe), mpv (`portable_config\`), VLC (a `portable\` folder next to
  `vlc.exe`; start it with `--no-qt-privacy-ask`, or a first-run dialog keeps it from closing normally). Check
  `%APPDATA%`, `%LOCALAPPDATA%` and `HKCU\Software` afterwards. Make test videos with mpv's encoder from `lavfi` test
  sources, with generic names.
- **Accounts.** Never sign in to a real account ([the rule](../CONTRIBUTING.md#ground-rules)). Every reader has fakes:
  Plex in `internal/plex`, `cmd/mpcvibedrpc/plex_test.go` and `internal/engine/plex_test.go`; Stremio in
  `internal/stremio`, `cmd/mpcvibedrpc/stremio_test.go` and `internal/engine/rest_test.go`; Jellyfin / Emby in
  `internal/jellyfin`, `cmd/mpcvibedrpc/jellyfin_test.go` and `internal/engine/jellyfin_test.go`. A small fake server on
  127.0.0.1 can drive the window's sign-in in a throwaway copy (keep showing on Discord off).
- **Leave nothing running.** Quit test copies, stop only processes you started, delete scratch tests before a full
  test run, and check the real data folders afterwards.

### Opt-in live tests
These skip unless their variable is set. CI sets them for its throwaway players and servers.

| Test | Variables | What it does |
|---|---|---|
| `TestLivePlayer` (`internal/engine`) | `MPCRPC_LIVE_PLAYER` (the name shown: mpv, IINA, VLC, Celluloid, Plex, ...), `MPCRPC_LIVE_TITLE` | Runs the engine against a real player, with the test's own fake Discord. |
| | `MPCRPC_LIVE_PORT`, `MPCRPC_LIVE_MPV_PIPE`, `MPCRPC_LIVE_IINA_PIPE` | The MPC port and the mpv / IINA pipe to ask. |
| | `MPCRPC_LIVE_VLC_PASSWORD`, `MPCRPC_LIVE_VLC_PORT` | VLC's web interface. |
| | `MPCRPC_LIVE_KODI_PASSWORD`, `MPCRPC_LIVE_KODI_ADDRESS` | Kodi's web server (the address is optional). |
| | `MPCRPC_LIVE_PLEX_ADDRESS`, `MPCRPC_LIVE_PLEX_TOKEN` | A Plex server. |
| | `MPCRPC_LIVE_JELLYFIN_ADDRESS`, `_USER`, `_PASSWORD` | Signs in to a Jellyfin or Emby server with the password. |
| | `MPCRPC_LIVE_EVENTS` (`pipe`, `kodi` or `mpris`) | **Changes the player's state:** pauses the real player and wants the pause shown at once from the player's own event, then resumes it. |
| `TestLiveJellyfin` (`internal/jellyfin`) | `MPCRPC_LIVE_JELLYFIN_ADDRESS`, `_USER`, `_PASSWORD`, `_PRODUCT` (`Jellyfin` or `Emby`), `MPCRPC_LIVE_TITLE` | Signs in to a real server (a wrong password first, then the password, and Quick Connect on Jellyfin), follows what the user plays, signs out. |
| `TestLivePotPlayer` (`internal/potplayer`, Windows) | `MPCRPC_LIVE_POTPLAYER=1` | Asks a real PotPlayer that plays a local file and checks that it sends the file's path. |
| `TestLiveCatalogs` (`internal/artwork`) | `MPCRPC_LIVE_CATALOGS=1` | One long-known title per keyless catalog, through the program's own search and episode code (the canary; the real titles are kept in that file). |
| `TestAutostartRegistryRoundTrip` (`internal/winsys`, Windows) | `MPCRPC_TEST_REGISTRY=1` | **Touches the real setup:** writes a real `HKCU\...\Run` value (for a throwaway data folder's value name, never the plain `MPCvibedRPC` one) and removes it again. |

Example: `MPCRPC_LIVE_PLAYER=mpv MPCRPC_LIVE_TITLE="Sample Movie" go test -count=1 -run TestLivePlayer -v ./internal/engine`

## Where things are

### The program: `cmd/mpcvibedrpc`
| File | Job |
|---|---|
| `main.go` | flags, single instance, `var version` |
| `app.go` | the `app` type: start-up and quit order |
| `api.go` | the local API handlers (`state`, `status`, `save`, `reset`, `start`, `stop`, `mpc-web`, `diagnostics`, `update-*`, `quit`, ...) |
| `update.go` | update checks, install, automatic installs, the hand-over (`handOverTo`) |
| `selfcheck.go` | **Help > Run a check** (its lines never hold a title) |
| `window.go` | the window's X quits; minimizing sends it to the tray (`minimizeToTray`) |
| `tray.go` | the tray / menu bar icon, tooltip and menu |
| `accounts.go` | the `approval` helper the sign-ins share, and their common endpoints |
| `plex.go`, `stremio.go`, `jellyfin.go` | the sign-ins |
| `logger.go` | the log |
| `launch_darwin.go` | macOS: started from the Finder, hands over to a detached `--serve` copy and leaves; gives a copy started at login a TMPDIR |
| `launch_other.go` | the same hooks as no-ops on other systems |

### Packages: `internal/`
| Package | Job |
|---|---|
| `engine` | the check loop: asks the players, builds and sends the activity, pause clearing, hide rules, preview, status for the window |
| | `engine.go` types, Start/Stop, Status · `players.go` `fetchPlayer` and its `picker` · `tick.go` · `discord.go` · `events.go` player events · `remote.go` the Plex, Stremio and Jellyfin watchers, resting · `folder.go` the folder listing for `folderEpisodeNumbers` (read once per file, again when the folder changed) · `disc.go` a disc's own name at a drive's root (`bdmt_*.xml`, the volume label) · `stats.go` `stats.json`, counts only (taken under `e.mu`, written after it is released) |
| `core` | settings and defaults (`config.go`), file-name parsing (`parse.go`), the activity (`activity.go`), preview, privacy rules, catalog services (`sources.go`), every other outside address (`hosts.go`), player logos (`icons.go`), titles written like file names (`names.go`) |
| `mpvipc`, `pipe` | mpv JSON IPC (mpv, MPC-QT, IINA); named pipes on Windows, Unix sockets elsewhere (shared with `discord`) |
| `mpris`, `dbus` | Linux players through MPRIS (an allowlist of video players) and their signals (`watch.go`); a minimal D-Bus client; `dbus/dbustest` is a fake bus |
| `vlchttp` | VLC's web interface, read-only |
| `potplayer` | PotPlayer (Windows): window messages from its control header `InternalSimpleCmd.h`, read-only |
| `kodi` | Kodi's JSON-RPC and its notifications on TCP 9090 (`watch.go`), read-only |
| `plex` | PIN sign-in and servers (`account.go`), the 7-day key sign-in (`jwt.go`, off by default), play notifications (`watcher.go`), metadata to a file-like name (`meta.go`) |
| `stremio` | link-code sign-in and sign-out (`account.go`), the library as Stremio's player updates it (`watcher.go`) |
| `jellyfin` | Jellyfin and Emby: server lookup, Quick Connect and password sign-in, sign-out (`account.go`), sessions (`watcher.go`), item to a file-like name (`meta.go`) |
| `remote` | what the three account readers share: problem texts, nil-safe log and clock, OS name, network-error wording, the same-origin redirect check |
| `capture` | whether Discord shares the screen (Windows, read-only; see [The stream title](#the-stream-title)) |
| `conns` | Recent connections (Privacy tab): wraps `http.DefaultTransport` and counts requests by host, in memory. A new HTTP client must keep using the default transport. |
| `proc` | starting a helper process hidden (Windows) or detached (`Detach`, `DetachVisible`: the window's browser, the macOS `--serve` copy) |
| `discord` | Discord's local IPC protocol |
| `artwork`, `jsre` | catalog lookups and episode titles; a regex engine with JavaScript semantics that the matching rules rely on |
| `store`, `jsonx` | `config.json`, the window's layout (`schema.go`), validation; a JSON reader/writer that keeps key order and unknown keys and prints numbers like JavaScript |
| `server`, `assets` | the local API on 127.0.0.1 (per-launch token, host check, CSP); the page (`ui.html`, `ui.css`, `ui.js`, embedded; its look: [design/settings-window.md](design/settings-window.md)) and `changelog.json` |
| `winsys` | the operating system; see the next table |
| `diag` | **Copy diagnostics**: keeps titles and file names, leaves out folder paths, URL paths and queries, server names, sign-ins, keys and the Never show list |
| `updater` | GitHub release check, download, checksum, swap and restore of the program file |

`internal/winsys`, by topic. The macOS and Linux parts live in untagged files where possible, so their logic is tested
on every system.

| File | Job |
|---|---|
| `tray.go` (`TrayOptions`), `tray_windows.go` | the tray (Windows) |
| `run.go` | helper commands without a console, the running players, system details for diagnostics |
| `macapp_darwin.go` | macOS (cgo only): the menu bar icon and the settings window (WebKit) |
| `window.go`, `browser*.go` | the window's browser, opening a URL or folder |
| `taskbar_windows.go` | the browser window's taskbar button gets the program's name, icon and start command (set from outside once it appears) |
| `autostart*.go` | start at login: the registry's Run key, a LaunchAgent, XDG autostart (`autostart_unix.go`) |
| `single_*.go` | single instance (mutex, file lock) |
| `mpcsetup.go` (`EnableMpcWebInterface`), `setup_unix.go`, `mpvconf.go`, `vlcconf.go` | **Set up the player connection** |

### Tools and docs
| Path | Job |
|---|---|
| `tools/mkrsrc` | exe resources (version info, icon, DPI manifest) |
| `tools/mkico` | the icon generator (`-icns`: the macOS app icon; `-dot`: the update dot) |
| `tools/relnotes` | the release text, rendered from the changelog |
| `tools/doclinks` | only a test: every relative link and anchor in the Markdown files resolves |
| `tools/screenshots` | `demo.js` for the README pictures ([Screenshots](maintaining.md#screenshots)) |
| `tools/downloads` | the project's own download count (on the `stats` branch; see [CI](#ci)) |
| `tools/ci` | the CI scripts; see the list below |
| `docs/` | user docs (`how-it-works.md`, `config.md`, `troubleshooting.md`), `roadmap.md`, `decisions.md`, `ai.md`, `design/` (design proposals and the window's look), `research/` (reports on Discord's card and on immutable releases), this file, `maintaining.md`, images and player logos (`players/`, named by `core/icons.go`; sources in `THIRD-PARTY-NOTICES.md`). **Every install fetches the logos from `docs/players/<name>.png` on the `master` branch** (`PlayerIconBase`): never move, rename or delete a logo or rename `master`; adding one is fine |

`tools/ci`:
- `smoke.ps1`, `smoke-unix.sh`: the smoke tests (Windows: also DPI awareness, version details and resting).
- `live-players.sh`, `live-windows.sh`, `live-plex.sh`, `live-jellyfin.sh`, `live-flathub.sh` (canary only): the live
  jobs.
- `reproduce.sh`: rebuilds the programs from a tag and compares them byte for byte (`sums` and `same` give the release
  workflow's comparison).
- `verify-release.ps1`: checks a release, first its draft, then the public one.
- `version.sh`: the version from `main.go` (see [maintaining.md](maintaining.md#releasing)).
- `dev-download.sh`: every development download of a release file, recorded in the ledger.
- `canary-report.sh`: the canary's issue. `package-macos.sh`: the .app.
- `test-noskip.sh` with `allowed-skips.sh`, `must-pass.sh`, `run-annotated.sh`, `lib.sh`: see
  [Writing tests](#writing-tests) and [Rules for CI files](#rules-for-ci-files).
- `windows.swift`: lists the windows on screen on macOS (counts the settings window's windows, finds the menu bar
  icon). `fake-discord.ps1`, `fake-http.ps1`: fakes for Discord and HTTP.

## How players are read
`engine.fetchPlayer` asks the players in this order:
1. the MPC web interface (`/variables.html` on `port`; `core.PlayerOf` names MPC-HC, MPC-BE or MPC-QT)
2. MPC-QT's always-on pipe `cmdrkotori.mpc-qt.mpv`
3. mpv's pipe (`mpvPipe`)
4. macOS: IINA's socket (`iinaPipe`, an mpv option in IINA's settings)
5. VLC's web interface (`vlcPort`, only when `vlcPassword` is set)
6. Linux: MPRIS players on the session bus (`mpris`; `engine.HasMPRIS`)
7. Windows: PotPlayer's windows (its documented WM_USER messages; the path comes back as WM_COPYDATA)
8. Kodi's JSON-RPC web server (`kodiAddress`, only when `kodiPassword` is set)
9. Plex, Stremio and last Jellyfin / Emby, each only while signed in (`plexToken`, `stremioKey`, `jellyfinToken`)

The first player that plays a video wins, then the first that plays music, then the first that is paused, then the
first that answers ([users' view](how-it-works.md#players-in-detail)). A new player is asked after the existing ones,
and MPC-HC behaves as before ([decision](decisions.md#behaviour)). A song (`core.IsAudioFile`: `.mp3`, `.flac`, ...) is
shown as "Listening to" (`core.Listening`) with its file name (`core.MusicMedia`), never looked up and not counted in
the stats. MPRIS only reads players on the allowlist in `internal/mpris`: music players and browsers offer MPRIS too and
must never show as "Watching". MPC-QT's pipe is only asked, never watched.

On macOS and Linux a bare pipe name is a socket in `os.TempDir()` (`pipe.Path`; `~/` is expanded). A copy started at
login on macOS gets TMPDIR from `getconf DARWIN_USER_TEMP_DIR`, because launchd sets none and that folder is where
Discord, IINA and MPC-QT put their sockets.

**Player events** (`engine/events.go`) wake the loop early (a nudge: 250 ms later, at most every 4 s); the check still
reads what plays.
- mpv and IINA: `mpvipc.Watch`, a second IPC connection that observes `pause` and listens for seek and file events. On
  Windows `pipe.Dial` waits up to a second for a pipe whose instances are all taken (the check and the watch can
  connect at the same moment).
- MPRIS players: `mpris.Watch`, D-Bus `PropertiesChanged` and `Seeked` signals.
- Kodi: `kodi.Watch`, its JSON-RPC over TCP on port 9090 (on by default in Kodi).
- Tests set `Options.KodiEvents` and `Options.MPRISBus`, so no real Kodi or bus is reached.

### Plex
- **Sign-in** (`cmd/mpcvibedrpc/plex.go`): a token through a plex.tv PIN the user approves in their browser; the
  window never receives the token.
- **Server:** the watcher looks the chosen server up on plex.tv (`clients.plex.tv/api/v2/resources`, or uses
  `plexAddress`) and checks `/identity` without the token. The token goes only to the server that answered with the
  chosen id; a shared server gets its own `accessToken`, never the account's.
- **Playback:** Server-Sent Events from `/:/eventsource/notifications?filters=playing` (a
  `PlaySessionStateNotification` per player report, every few seconds). Titles come from `/library/metadata/<ratingKey>`
  and are written like a file name ("Sample Show - S01E02 - Title", "Sample Movie (2020)") so the usual parsing and
  lookups apply. On a server the user owns, only their own sessions count (`/status/sessions`: user 1 or the account
  id); music and photos are skipped.
- **`plexJwt`** (off by default, `config.json` only): Plex's newer 7-day tokens (`jwt.go`, as developer.plex.tv's
  "Authenticating with Plex" describes). An Ed25519 key per install (`plexKey`, a secret); its JWK is sent with the PIN
  to `clients.plex.tv/api/v2/pins`; the token is collected with a signed device JWT and renewed (`/api/v2/auth/nonce`,
  then `/api/v2/auth/token`) two days before it runs out (`plexAccount.keepFresh`, hourly). Only tested against a
  fake plex.tv; turn it on by default only after a real sign-in has been tried with the maintainer's OK.

### Stremio
- **Sign-in:** the account's key through a link code (`link.stremio.com/api/v2/create` and `read`, the TV apps'
  sign-in; only an https stremio.com page is ever opened). Sign-out and Restore defaults call
  `api.strem.io/api/logout`.
- **Playback:** Stremio has no "now playing" API. The watcher polls `datastoreMeta` (every library entry's mtime) every
  20 s and fetches changed entries with `datastoreGet`.
- Stremio's player (stremio-core) pushes the entry on load, pause, resume and stop, and while it plays once
  `PUSH_TO_LIBRARY_EVERY` has passed since the last regular push (90 s up to core 0.65.0, 180 s from 0.65.1; the extra
  pushes do not move that timer).
- A push with the time moved on before the next regular one is due is a pause or stop (status taken down); one with the
  time unchanged is a start or resume. The watcher learns each player's interval from its pushes (`judge`); no push for
  the interval plus about 55 s (180 s unless 90 s was learned) means it stopped.
- The API was learned from stremio-core's source and unit tests; there is no live test.

### Jellyfin and Emby
- **Server:** the user types its address on the Players tab; `/System/Info/Public` says which it is (Emby may answer
  only under `/emby`, which is then kept in `jellyfinAddress`).
- **Sign-in:** Jellyfin uses Quick Connect (`/QuickConnect/Initiate`, `Connect`, `/Users/AuthenticateWithQuickConnect`;
  the user enters the code in a Jellyfin app). Emby, or Quick Connect off: `/Users/AuthenticateByName` (the password is
  passed on once, never kept). Sign-out and Restore defaults call `/Sessions/Logout`.
- **Headers:** every request carries `Authorization: MediaBrowser ...` (Jellyfin 12 ignores the legacy headers by
  default) and `X-Emby-Authorization` / `X-Emby-Token` (Emby).
- **Playback:** `GET /Sessions` every 5 s; only the signed-in user's sessions count (`UserId`). The position is moved on
  by the server's `Date` minus `LastPlaybackCheckIn` (Emby sends no check-in time: an unchanged report keeps its first
  time base).
- Error messages never carry the server's address (`jellyfin.Reason`).

### Known quirks
- MPC-QT's web page reports a meaningless `playbackRate` (its pipe is preferred) and reports seeking as state 3
  (treated as playing).
- VLC 3 rounds `time` to whole seconds (position x length is used); VLC 4 uses a flat playlist.
- With the default app name, the tooltip names mpv and VLC as themselves, and the default corner icon (MPC's logo) is
  left out for players outside the MPC family.
- The default Discord application (`clientId`) was registered by the upstream project, not by this one: every install
  depends on it staying there. It holds only a few images (`default`, `pause_small`, `play_small`, `stop_small` and
  `mpcbe_logo`, checked 2026-11-21); the player logos, the MPC family's too, are fetched from `docs/players` (below).

### Set up the player connection
`winsys.EnableMpcWebInterface`:
- **MPC-HC, MPC-BE:** web interface on (registry and ini; they are closed and reopened after the user confirms), and
  this-PC-only when the button switches it on.
- **mpv:** `input-ipc-server` in the `mpv.conf` it reads (a name the user set is kept and adopted).
- **VLC:** `extraintf=http`, a password and `http-host=127.0.0.1` in the `vlcrc` it reads (an existing password and
  port are kept and adopted). VLC is closed and reopened only if something must change, because it rewrites `vlcrc` on
  exit.
- **MPC-QT** needs nothing.

On macOS and Linux (`winsys.enableUnix`):
- **mpv:** the full socket path in `~/.config/mpv/mpv.conf`.
- **IINA:** `enableAdvancedSettings` and an `input-ipc-server` row in `userOptions` through
  `defaults write com.colliderli.iina` (an existing row is kept and adopted; IINA is not closed, the user reopens it).
- **VLC on macOS:** as on Windows, in `~/Library/Preferences/org.videolan.vlc/vlcrc` (closed with AppleScript, reopened
  with `open -b`).
- **Linux:** VLC and the other MPRIS players need nothing.

## The engine
**Resting** (`engine.rest`): while Discord's IPC endpoint is absent (`pipe.Exists`: a directory query on Windows, a
stat elsewhere; never a connection) and no settings window holds `/api/alive` (`server.Options.OnWindow` ->
`engine.Options.MayRest`), a check asks no player, the Plex, Stremio and Jellyfin watchers are stopped, and `OnRest`
(`winsys.TrimMemory`) hands memory back. A window opening calls `engine.Wake`. Tests get no resting unless they set
`MayRest`.

**Clearing** (`tick.go`):
- A status counts as shown from the moment it is sent, also when Discord's answer does not come (it may have been
  applied).
- A clear Discord does not take keeps it "shown", so the next check clears again.
- While nothing plays and Discord is connected, it is cleared again every `reclearEvery` (a minute, quietly).
- A file paused within `endSlack` (3 s) of its end has ended.
- Tests: `internal/engine/stale_test.go` (the fake Discord's `slow` and `failClears`).

**The engine's state** is many fields under `e.mu`, changed by the check, the connect goroutine, the connection watch,
the cover finder and the API. The rules that keep them in step:
1. A status is due when the player reports a change (`core.NeedsUpdate`) or when `resend` is set (a new connection, a
   late cover, a send Discord did not take).
2. Only a change by the player restarts the pause limit or brings back a status cleared after a long pause.
3. Every place that takes `e.mu` again after asking Discord checks that its run is still the current one
   (`e.cur == r`) before it books anything.
4. The clear after a long pause is booked when Discord took it, not before.
5. A send Discord did not take waits `retryAfter` (5 s, doubling to a minute) before the next try.
6. `basicMode` and `noLinks` hold for the file they were found out for (`modeKey`).
7. A check in which no player answers while a status is up changes nothing once, and asks again after `missWait` (a
   second).
8. With `streamTitle` "auto", the check asks `capture` whether Discord shares the screen and builds the stream form
   ("always") or the usual one ("off"); a change of that sends the same file again.

Tests: `internal/engine/state_test.go`. `TestStatusConverges` runs random orders of player, Discord and settings events
against the fakes and checks that Discord ends up showing exactly what the player's last state calls for. After a
change to the check loop, run it with more orders:

    MPCRPC_CONVERGE_SEEDS=500 go test -count=1 -run TestStatusConverges ./internal/engine

## The stream title
`streamTitle` ("auto", "always", "off"): Discord names a stream after a Playing activity only, so the engine builds the
Playing form while Discord shares the screen ([users' view](how-it-works.md#on-your-streams),
[decision](decisions.md#behaviour)).
- `internal/capture` tells a share from the thread Discord's app runs only then (`shareThreads`: `RsVideoSource`; the
  names and start times of Discord's threads, walked with `NtGetNextThread` and `GetThreadDescription`, about 15 ms a
  look). Seen with Discord 1.0.9267 for a screen share and a window share, not for the share picker or a camera.
- A second sign is Windows' record of screen captures per program
  (`HKCU\...\ConsentStore\graphicsCaptureProgrammatic\NonPackaged`; a stop time of zero means a capture runs). Windows
  11 25H2 and later no longer write it there.
- A watch reports changes of both. Nothing on other systems.
- If Discord renames the thread, "auto" sees nothing: the fix is the name in `shareThreads`, found by listing Discord's
  thread names while a share starts and stops.
- Engine tests never look at this computer (`Options.Sharing`; the engine tests' `init` replaces the default). The
  capture package's own tests read this computer's Discord, read-only. It cannot be tried against a real Discord in
  tests: it is checked on a real stream.

## Updates
[Users' view](troubleshooting.md#updates).
- **When:** as the program starts, then every six hours by the wall clock (`scheduleUpdateChecks`).
- **Window open:** a found version is offered at once by a dialog (`askUpdate` in `ui.js`: Update now or Not now, once
  per version; the notice at the top brings it back).
- **Automatic install** (`autoUpdate`, on by default; Windows and Linux): at a quiet moment (`quietMoment`: nothing
  shown on Discord, no settings window; never in the first ten seconds after the start or after a window was asked for,
  `settle`). The new copy starts in the tray (`auto-updated` marker in the data folder) and says so once.
- **Hand-over** (`handOverTo` in `cmd/mpcvibedrpc/update.go`): the old copy frees the window, tray, port and
  single-instance lock and waits for the new copy's `ipc.json`. If it does not come, it puts the old program file back,
  writes `update-failed` (the version, which is not installed by itself again) and starts itself again with
  `--update-failed`.
- **Red dot:** while an update waits, the tray and menu bar icon show a dot (`assets.IconUpdate`, made by
  `tools/mkico -dot`; macOS: `winsys.menuBarIcon`).

## Adding things

### A new player
1. A reader package with unit tests on recorded-style responses.
2. Wire it into `fetchPlayer` *after* the existing players; engine tests including player order.
3. Setup support in `winsys` if the player needs switching on.
4. A live run in `tools/ci/live-players.sh` if it exists on Linux or macOS. A Linux MPRIS player only needs its
   bus-name part in `mpris.videoPlayers`, its process name in `winsys.unixPlayers`, and the lists below.
5. Every place that lists players: `ui.html` and `ui.js` (welcome card, "No player is answering" card, "Nothing
   playing" hint, Privacy "What else leaves this PC"; the macOS and Linux texts are in `OS_TEXT`), the README (tagline,
   What you get, Getting started, the Players table, Help), `docs/how-it-works.md` (Players in detail),
   `docs/troubleshooting.md`, `docs/development.md` (Where things are), `docs/config.md`, the help texts in
   `store/schema.go` (`port`, `mpris`, `iinaPipe`), `SECURITY.md`, `.github/ISSUE_TEMPLATE/bug_report.yml`, the exe
   description in `tools/mkrsrc/main.go`, the header comment in `main.go`, the engine's "Looking for a player on ..."
   log line, `diag`, and the repository description and topics. `TestPlayersAreNamedEverywhere` catches a missed text.

### A new setting
A field with a `json` tag in `core.Config` and its default in `DefaultConfig()`; a `Field` in `store/schema.go` if it
belongs in the window (int fields need Min/Max; `secret` for passwords and keys); a row in `docs/config.md` with the
default in JSON form (`TestEverySettingIsDocumented`); `diag.summarise` if the value is private (shown as "(set)").
Defaults that change behaviour for existing users need the maintainer's decision and a line in the release notes.

### A new catalog service
An entry in `core.Sources` (id, label, use, host: it gets its switch on the Privacy tab); a `<Name>Base` field with its
default in `core.Config` (tests point it at a fake); its id in the default `artworkSources` and/or `episodeSources` (a
default change needs the maintainer); the `Provider` in `artwork/providers.go` and/or the `epProvider` in
`artwork/episodes.go`; its line in `blockedHostFilter` (`artwork/disabled.go`); a subtest in
`internal/artwork/live_test.go` (the canary); its row in `SECURITY.md`; the credit its terms ask for (Help tab credits,
`THIRD-PARTY-NOTICES.md`); and `docs/config.md`.

### A new outside address
Any host the code names (a service, a link, an image) goes in `core.Hosts` (or `core.Sources`) and in `SECURITY.md` >
Where it connects; `TestEveryHostIsListed` scans the code and the page and fails otherwise.

## Testing
`go test -count=1 ./...` runs nearly everything:
- the parsers, artwork matching and activity builder against the golden data (tens of thousands of cases);
- the engine against fakes: MPC-HC, mpv, VLC, Plex, Stremio, Jellyfin / Emby and Discord;
- the whole program, built and driven end to end (not on Windows: `TestEndToEnd` and `TestEndToEndUpdateRollback`
  skip there, and the Windows smoke test in CI covers them; they run on Linux and macOS, the rollback test on Linux only).

What CI adds (the race detector, the lint gates, the smoke tests, the live jobs and the canary) is in [CI](#ci). How to
run a copy or a real player without touching an installed setup: [Running the program safely](#running-the-program-safely).

**Tests that tie texts to the code** (keep them passing; they fail when a doc falls behind):
- `TestEverySettingIsDocumented` (`internal/store`): every setting has its row and default in [config.md](config.md).
- `TestPlayersAreNamedEverywhere` (`cmd/mpcvibedrpc`): every text that lists the players names them all.
- `TestEveryHostIsListed` and `TestSecurityListsEveryHost` (`internal/core`): every outside address the code and the
  page name is in `core.Hosts` or `core.Sources`, and `SECURITY.md` names each.
- `TestPageScriptParses` (`internal/assets`): `node --check` on `ui.js` (skips without Node.js).
- `TestChangelog` (`cmd/mpcvibedrpc`): the newest changelog entry is the version in `main.go`.
- `tools/relnotes`'s test: the release text renders from `internal/assets/changelog.json`.
- `TestLinksResolve` (`tools/doclinks`): every relative link and `#anchor` in every Markdown file resolves (GitHub's
  anchor rules; links to other websites are not followed).

### Writing tests
- `tools/ci/test-noskip.sh` fails CI when a test skips that should run on that OS. A new test that is Linux-only,
  Windows-only or opt-in must be added to `tools/ci/allowed-skips.sh` (`ci.yml` and `release.yml` both use it). The
  live test scripts run through `must-pass.sh`, so a live test that skips fails its CI job.
- Engine tests use a fake Discord (Unix socket on Linux, named pipe on Windows) and fake players (`players_test.go`,
  `vlc_test.go`). Wait for state with the `eventually()` helper rather than reading it once.
- Engine tests run in parallel and poll every 25 ms (`newTestEngine` sets `Options.MinPoll`; `ReclearEvery`,
  `RecheckAfter` and `RetryAfter` shorten the other waits). So a test must never change a package variable; the one
  that has to (it shortens `discord.RequestTimeout`) stays sequential.
- Pure timer logic is tested on `testing/synctest`'s clock (`TestNudgesArePaced`).
- `Options.Pipes` keeps a real local MPC-QT/mpv/IINA out of the tests, and `Options.MPRIS` the real session bus; VLC is
  only asked when a test sets a password. On macOS a socket path may only be 104 bytes, so test sockets go in a short
  folder under /tmp there (`sockDir`).
- Run `go test -race` somewhere that supports it (Linux CI) after concurrency changes.
- **Window script changes need the real page opened** ([how](#running-the-program-safely)): unit tests cover the
  server side only, and `TestPageScriptParses` only catches a script that does not parse.

## CI
The workflows are in `.github/workflows/`. **CI is the only place macOS and Linux run for real**, so read those logs
after every change that touches them. A Linux or cloud environment can run almost everything else; the tray, the
window, the registry and the Windows players need Windows.

`ci.yml` runs on every push to a branch other than `master` that changes more than documentation (top-level `.md`
files, `docs/`, issue templates); `docs.yml` covers those changes (below). A pull request from this repository is tested by its branch's push run; only pull
requests from forks are tested through the `pull_request` event. `master` is not tested on push (its commits come from
a tested branch, and the tag run tests again). A newer push cancels a run still going on the same branch.

| Job | When | What it does |
|---|---|---|
| `test` | every run | `go vet` and the tests on Linux (with `-race`), Windows and macOS |
| `lint` | every run | Linux: the gofmt gate, `govulncheck` and `staticcheck` (for Linux and `GOOS=windows`; staticcheck also `GOOS=darwin`; without ST1005, because the error texts are sentences the window shows), and `deadcode -test` (every function must be reachable from the program or a test). No job needs it, but a failing gate fails the run and the canary. |
| `windows` | after `test` | builds the exe (LF checkout, `CGO_ENABLED=0`) and runs `smoke.ps1` |
| `reproduce` | after `windows` | rebuilds the exe on Linux (`reproduce.sh`); fails unless it is the same file byte for byte |
| `unix` | after `test` | builds the Linux programs and the macOS app and runs `smoke-unix.sh` (API, login item, player setup, second start; on macOS opening the app through LaunchServices) |
| `players` | live | `live-players.sh`: real mpv, VLC (MPRIS), mpv-mpris and Celluloid on Linux; mpv and IINA (set up by the real program first) on macOS |
| `players-windows` | live | `live-windows.sh`: real PotPlayer and Kodi from Chocolatey, a generated video in each, the live engine test and `TestLivePotPlayer` |
| `plex` | live | `live-plex.sh`: the official Plex Media Server image (unclaimed, no account), a stand-in player reporting through `/:/timeline` |
| `jellyfin` | live | `live-jellyfin.sh jellyfin\|emby`: the official images, the first-run wizard with a made-up user, a stand-in player (`/Sessions/Playing`), the real sign-in (password and Quick Connect) and the live engine test |

"Live" jobs run **once per pull request** (when it is opened, reopened or marked ready), by hand (`workflow_dispatch`)
and in the canary, not on every push ([decision](decisions.md#releases)). The build-and-smoke steps are the composite
actions `.github/actions/build-windows` and `build-unix`, shared with `release.yml`.

Other workflows:
- **`docs.yml`** (every push to a branch other than `master` and every pull request, with no paths filter): runs only
  the tests that read the documentation (`TestEverySettingIsDocumented`, `TestPlayersAreNamedEverywhere`,
  `TestEveryHostIsListed`, `TestSecurityListsEveryHost`, `tools/relnotes`' tests and `tools/doclinks`), because `ci.yml` skips a change
  that touches only documentation.
- **`release.yml`** (on a tag): tests, builds, smoke-tests and publishes; it does not run the lint gates (they gate the
  pull request before the merge). See [maintaining.md](maintaining.md#releasing).
- **`canary.yml`** (Mondays, and by hand): runs all of `ci.yml` (`workflow_call`) against the newest players and
  server images, mpv, VLC and Celluloid from Flathub (`live-flathub.sh`), and `TestLiveCatalogs`.
  `canary-report.sh` opens an issue labelled `canary` on a failure and closes it once a run passes (only on the
  schedule or a by-hand run with `report` ticked; a pull request that changes the canary does a dry run). Treat a
  canary issue like a bug report.
- **`downloads.yml`** (every three hours, and ten minutes after each release): the project's own download count
  (people downloading the program files, not checksums or notices, without the project's own downloads), written to
  the `stats` branch only; `master` is never written by a workflow. The README's badge shows GitHub's own count ([decision](decisions.md#releases)); the exact one is
  `summary.json` on `stats`. It counts from `baseline.json` (run the workflow by hand with `baseline` ticked to start it again from the counts now) minus the ledger of development downloads
  ([dev-download.sh](maintaining.md#downloading-release-files)). `notCountedYet` in its summary is a recorded download
  GitHub has not counted yet (it lags by minutes); if it stays above 0, a recorded download never happened.

A Go security fix means bumping the patch version in `go.mod` alone: every `setup-go` reads `go-version-file`.

### Rules for CI files
- **Actions are pinned by commit** (`uses: owner/action@<sha> # vX.Y.Z`); Dependabot updates the pins. A new action
  gets the same form, and a job gets only the `permissions` it needs (workflows are read-only by default).
- The CI scripts share `tools/ci/lib.sh` (the test video, `wait_socket`, `wait_bus`, `done_file`): a shared script
  must source it. `run-annotated.sh` fails a step whose script did not reach `done_file` when `PASSED_FILE` is set: CI
  scripts write `$PASSED_FILE` at their end and steps check it.
- The concurrency group must include the event action.

### Known flaky checks
These have failed without a code change; read the log, then re-run once:
- The **Chocolatey Kodi download** in `players-windows` (TLS errors from its mirror). The canary uses the same job.
- The macOS **IINA pause check** in `players` timed out once (5 s) and passed on a re-run.
- `TestEpisodeWithoutTitleIsAskedAgainWhilePlaying` (it counts sends) and two timing tests each failed once in a
  full-suite run that came right after heavy work. Run the single test again before suspecting the change.

## Not yet tried for real
Everything below has only run against fakes, or not at all. Anyone with the setup can try it and report the result
(then move the line to [What has been tried](how-it-works.md#what-has-been-tried)):
- A **pinned taskbar button** of the settings window: it shows the program's icon and starts the program.
- A viewer seeing "Streaming <title>" with no other Playing program running.
- The **update dialog** in the real settings window, the tray's update notice, and the tray icon's sharpness at 200 %
  display scaling.
- **Copy diagnostics** in a normal browser window (not the `--app=` window).
- **Start with Windows** after it was switched off in Task Manager's Startup list.
- **Set up the player connection** end to end (it writes MPC-HC's registry values).
- **MPC-QT's multi-window mode** with a portable build and the live engine test.
- **mpv on a real Linux desktop.**
- The settings window in **Chrome, Brave or Vivaldi** instead of Edge.
- **Plex sign-out** against plex.tv.
- A **Blu-ray's volume label** read from a real drive. (A real disc was recognised once in a real test.)

## Lessons
Short rules learned on this project.

Testing:
- **Fakes only answer what you thought of.** For a parse or lookup change, compare old and new on the real catalogs
  before release: a `git clone --branch vX` copy of the last release, the same scratch test in both, a diff of the
  outputs. Real titles stay in the scratch file; delete it after (and before any full test run).
- **A hung test run:** if a test panics while it holds a fake's lock, the fake's deferred cleanup waits for that same
  lock and never finishes, so the helper `eventually()` (which polls until a condition holds) waits forever. Kill the
  run and run the new tests one at a time with `-timeout 40s -v` to find the one that panics.
- A property test (one that checks a rule over many generated inputs) often fails first because the rule is stated
  wrongly, not because the code is wrong: check the rule first.
- Check the state a timing stands for, not the timing: wait with `eventually()` until the condition holds, instead of
  reading the fake Discord and the engine one after the other (they are updated at slightly different moments, so the
  second read can be stale).
- One run's memory reading misleads (the garbage collector runs when it likes): compare several runs side by side.
- `filepath.Base` does not split Windows paths (with backslashes) on Linux, so code that reads a player's file path
  must not rely on it when the path came from a Windows player.
- An apostrophe inside a single-quoted JavaScript string in `ui.js` ends the string early and breaks the whole
  settings page (`TestPageScriptParses` catches it).

Code:
- When a delay or grace period is removed (for example the wait before the first update check), grep for what other
  code relied on it: the first poll of the players, a page that is still loading, tests that expect "no update yet".
- A feature that reads an operating-system record (for example the Windows record of which program captures the
  screen) must first check that the record is still written today: look at the newest entry's date.
- Before deciding something cannot be done, look one layer down: Discord does not say which window is being shared,
  but the operating system knew. Ask what the system records about it.
- An asset key the Discord application does not hold is dropped silently. The application's assets are public at
  `https://discord.com/api/v10/oauth2/applications/<id>/assets`: check there before relying on a key.

Catalogs and episodes:
- Check an episode-title bug against a source that gives explicit episode numbers (Wikipedia's episode lists). When
  counting entries in a recorded response, parse the JSON lines; counting them with `grep` miscounts.
- The golden season data (the recorded season lists of the catalogs) pins two rules: the earliest entry wins, and a
  "Part" counts as its number. It also pins that a file name with `[ii part]` (the roman numeral "ii" and "part", as
  anime releases write a season) gets no season mark. New season rules must be narrow additions in `pickSeasonEntry`
  (the function that picks a catalog's season entry), checked against that data.

CI:
- The CI scripts run on macOS, which has bash 3.2: no negative array indexes, and no `${arr[@]}` of an empty array
  under `set -u`. `defaults export <domain> -` (reading an app's settings) succeeds for apps that were never installed,
  so it proves nothing about an install.
- Chocolatey's package for PotPlayer (the live Windows job installs it from there) has a checksum that goes stale
  whenever PotPlayer replaces its installer: CI installs it with `--ignore-checksums`.
- When the live Emby job sets up its test server: Emby ignores files with "sample" in the name (the test videos are called "Example Show"); adding a library only
  checks its folders (run the `RefreshLibrary` task); `/Sessions/Playing` needs a `PlaySessionId`; its logs go to
  `/config/logs`.
- Jellyfin 12 (the server the live job runs) ignores the legacy `X-Emby-*` headers unless `EnableLegacyAuthorization` is on: send
  `Authorization: MediaBrowser ...`.
- To learn a third-party server's API without an account, run its official image on a CI branch with a probe script.

Working practice:
- **Before building a sketched feature, check that it can do something for the user, and say its limits first.** For
  example, Discord's status only goes through the local Discord app, so a "keep following Plex while Discord is
  closed" switch would change nothing on any profile.
- **A narrow test someone sanctioned** ("read-only, no setting changes") stays inside it, and its unavoidable side
  effects (recent-files lists, titles visible to friends on Discord) are named before it starts.
- **Commit after every coherent step.** A power loss once zero-filled the one file that had uncommitted edits; git's
  objects were fine. `git fsck --full` plus a scan for zero-filled text files is the damage check.
- **In a verify-then-commit command chain, never put `;` before the commit**: a failing check must stop it.
- **`git config` run inside a `git worktree` writes the shared `.git/config`** (it once switched the main checkout off
  `autocrlf`). Use `git -c key=value` per command, or a separate clone.
- **Two people or agents on one checkout commit only their own paths** (`git commit -- <path>`), never `git add -A`.

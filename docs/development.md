# Development

How MPCvibedRPC is built, tested and released. For using it, see the [README](../README.md).

## Building
The program is written in Go and uses only the standard library: one small file (about 8 MB, about 17 MB of memory while
running and about 3 MB while resting), no runtime, nothing to install. Building needs [Go 1.27+](https://go.dev/dl/) and no internet beyond that.

    go run ./tools/mkrsrc -version X.Y.Z -out cmd/mpcvibedrpc/rsrc_windows_amd64.syso    (icon + version details, optional)
    go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=X.Y.Z" -o MPCvibedRPC.exe ./cmd/mpcvibedrpc

`-H=windowsgui` makes it a windowed program, so no console flashes up. You can build from any OS (`GOOS=windows`).
Discord's local protocol is implemented in `internal/discord`.

The Linux program is built the same way (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w"
-o MPCvibedRPC-linux-amd64 ./cmd/mpcvibedrpc`). The macOS app is put together on a Mac by
`tools/ci/package-macos.sh VERSION OUTDIR` (both processor types in one program, the app icon, an ad-hoc signature, a zip).

## Testing
`go test ./...` runs everything: the parsers, artwork matching and activity builder are checked against recorded reference output
(tens of thousands of cases, stored as test data in each package's `testdata/`), the engine against a fake MPC-HC, a fake mpv, a fake VLC, a fake Plex, a fake Stremio, a fake Jellyfin / Emby and a fake Discord, and the
whole program is built and driven end to end. On GitHub, Linux runs the tests with the race detector, Windows and macOS run them too, each system's
program gets a smoke test, and the engine is run against real players on Linux (mpv, VLC, mpv-mpris, Celluloid, a real Plex Media Server, and the official Jellyfin and Emby server images: `tools/ci/live-jellyfin.sh jellyfin|emby`), macOS (mpv, and IINA after the program has set it up) and Windows (PotPlayer and Kodi). Once a week the canary (`.github/workflows/canary.yml`) runs all of it against the newest players, server images and catalog services and opens an issue when something outside changed. Linux also runs `gofmt`, `govulncheck`, `staticcheck` and `deadcode`. A few tests tie the texts to the code: every setting has its row and default in [config.md](config.md), every text that lists the players names them all, the release notes are for the version being built, and the settings page's script parses.

Running a copy that cannot touch an installed one: `MPCRPC_HOME` gives it its own data folder (and its own
run-at-login entry), and `MPCRPC_BROWSER` names the browser to show the window in (one that understands `--app`), or
`none` for no window at all.

## Releasing
To publish a version:

1. In the change that makes the release: the version in `cmd/mpcvibedrpc/main.go` (the workflows and smoke tests read
   it from there), the version's lines at the top of `internal/assets/changelog.json` (the window shows them once
   after the update; a test checks that the newest entry is the version being built). That entry is also the release
   text: the workflow renders it with `tools/relnotes`, adding `.github/release-advisory.md` when that file exists (for
   what one-liners cannot say) and the "Get it" footer.
2. The change goes through a pull request whose CI is green and is merged; then
   `git tag -a vX.Y.Z -m "MPCvibedRPC X.Y.Z" && git push origin vX.Y.Z` - the workflow in
   `.github/workflows/release.yml` checks that the tag matches the version, runs the tests, builds the programs on
   Windows, Linux and macOS runners, smoke-tests each of them (Windows: tray, Discord pipe, resting while Discord is
   closed, DPI awareness, version details, autostart, self-update; macOS and Linux: the API, login item, player setup, a
   second start, and on macOS opening the app like the Finder does) and attaches them with their `.sha256` files and
   build records to a release that is still a draft.
3. A second runner rebuilds the Windows and Linux programs from the tag (`tools/ci/reproduce.sh`); each must match
   the release's file byte for byte. Then the draft is checked as a user would get it (`tools/ci/verify-release.ps1`):
   the ten files, each program's checksum and build record, the exe's version. Only then is the release made public,
   and a real self-update from the release before it is tried.

## Screenshots
The README's pictures (`docs/images/window-*.jpg`) are taken again when the part of the window they show changes:

1. Build the program and start it with a throwaway data folder (`MPCRPC_HOME`), with presence off and
   `{"app": {"seenVersion": "<version>"}}` in its `config.json` so the What's new box stays closed.
2. Open its settings page (the address and token are in `ipc.json` in that folder) in a browser at 620 x 700 pixels,
   dark mode (a high-DPI screen gives sharper pictures).
3. For `window-main.jpg` paste `tools/screenshots/demo.js` into the page's console: it shows a made-up
   "Sample Show" playing and sample stats, and nothing is sent anywhere. Take the General tab. For
   `window-privacy.jpg` (Privacy tab, scrolled to "Services that may see your titles") and `window-plex.jpg`
   (Accounts tab, scrolled to Plex) reload the page first, so presence shows as off.
4. Save them 800 pixels wide (800 x 903). Only made-up titles; nothing from a real library or account.

## Where things are
One row per package; the rules that go with them are in the repository's CLAUDE.md.

| Path | Job |
|---|---|
| `cmd/mpcvibedrpc` | `main.go` (flags, single instance, `var version`), `app.go` (the `app` type: start-up and quit order), `api.go` (the local API handlers: `state`, `status`, `save`, `reset`, `start`, `stop`, `mpc-web`, `diagnostics`, `update-*`, `quit`, ...), `update.go` (checks, install, automatic installs), `tray.go` (icon, tooltip), `accounts.go` (the `approval` helper the Plex, Stremio and Jellyfin sign-ins share, and their common endpoints) with `plex.go`, `stremio.go`, `jellyfin.go`, `logger.go` |
| `internal/engine` | the tick loop: asks the players, builds and sends the activity, pause clearing, hide rules, preview, status for the window, the local stats (`stats.go`, `stats.json`: counts only, never titles); split by job: `engine.go` (types, Start/Stop, Status), `players.go` (`fetchPlayer` and its `picker`), `remote.go` (the Plex, Stremio and Jellyfin watchers as one list, resting), `discord.go`, `tick.go` |
| `internal/core` | settings and defaults (`config.go`), filename parsing (`parse.go`), the activity (`activity.go`), preview, privacy rules, the online-service catalog (`sources.go`), every other outside address (`hosts.go`) |
| `internal/mpvipc`, `internal/pipe` | mpv JSON IPC (mpv, MPC-QT, IINA); named pipes on Windows, Unix sockets elsewhere (shared with `internal/discord`) |
| `internal/mpris`, `internal/dbus` | Linux players through MPRIS (an allowlist of video players); their change signals (`watch.go`); a minimal D-Bus client (calls and `Watch` for signals); `dbus/dbustest` is a fake bus for tests |
| `internal/vlchttp` | VLC's web interface, read-only (`status.json`, `playlist.json`) |
| `internal/potplayer`, `internal/kodi` | PotPlayer (Windows: window messages from its control header `InternalSimpleCmd.h`); Kodi (JSON-RPC: `Player.GetActivePlayers`, `GetItem`, `GetProperties`) and its notifications on TCP 9090 (`watch.go`); both read-only. Player logos for Discord are files in `docs/players`, named by `core/icons.go` (sources in `THIRD-PARTY-NOTICES.md`) |
| `internal/plex` | Plex: the plex.tv PIN sign-in, the account's servers (`account.go`), the 7-day key sign-in, off by default (`jwt.go`), one server's play notifications (`watcher.go`), metadata to a file-like name (`meta.go`); read-only |
| `internal/stremio` | Stremio: the link-code sign-in and sign-out (`account.go`), the account's library as Stremio's player updates it (`watcher.go`); read-only |
| `internal/jellyfin` | Jellyfin and Emby: the server lookup, Quick Connect and password sign-in, sign-out (`account.go`), the session list (`watcher.go`), item to a file-like name (`meta.go`); read-only |
| `internal/remote` | what the Plex, Stremio and Jellyfin readers share: the problem text, nil-safe log and clock types, the OS name, network-error wording, the same-origin redirect check (their titles are written like file names by `core/names.go`) |
| `internal/conns` | Recent connections (Privacy tab): `main` wraps `http.DefaultTransport`, so every web request to another computer is counted by host (count, last time, in memory; never a path or a title). A new HTTP client must keep using the default transport |
| `internal/proc` | starting a helper process hidden (Windows: no console) or detached from this one (`Detach`, `DetachVisible`: the window's browser, the macOS `--serve` copy) |
| `internal/discord` | Discord's local IPC protocol |
| `internal/artwork`, `internal/jsre` | catalog lookups, episode titles; a regex engine with JavaScript semantics that the matching rules rely on |
| `internal/store`, `internal/jsonx` | `config.json`, the settings window layout (`schema.go`), validation; a JSON reader/writer that keeps the file's key order and unknown keys and prints numbers like JavaScript |
| `internal/server`, `internal/assets` | the local API on 127.0.0.1 (per-launch token, host check, CSP) and the whole page (`ui.html`, `ui.css`, `ui.js`, each embedded), the changelog (`changelog.json`) |
| `internal/winsys` | the operating system: tray (Windows), autostart (registry, LaunchAgent, XDG autostart: `autostart_unix.go`), single instance (mutex, file lock), the window's browser, "Set up the player connection" (MPC registry/ini, `mpv.conf`, `vlcrc`, IINA's settings: `setup_unix.go`); by topic: `run.go`, `mpcsetup.go` (`EnableMpcWebInterface`), `window.go` (browser, open URL/folder), `autostart.go`, `tray.go` (`TrayOptions`). The macOS/Linux parts live in untagged files where possible so their logic is tested on every system |
| `internal/diag` | "Copy diagnostics": titles and file names kept (a wrong title is traced back to its file name); folder paths, URL paths and queries, server names, sign-ins, keys and the Never show list left out |
| `internal/updater` | GitHub release check, download, checksum, swap, restart |
| `tools/mkrsrc`, `tools/mkico` | exe resources (version info, icon, DPI manifest); the icon generator (`-icns` writes the macOS app icon) |
| `tools/downloads` | the README's download counter: GitHub's counts of the program files since `baseline.json`, minus the ledger of development downloads (both on the `stats` branch, kept by `.github/workflows/downloads.yml`, which writes `downloads.json` for the badge and `summary.json`) |
| `cmd/mpcvibedrpc/launch_darwin.go` | macOS: started from the Finder, the program hands over to a detached `--serve` copy and leaves (a build without the Cocoa code then still finds the running copy when opened again; with it, macOS sends the running app a reopen event, which shows the window); also gives a login-started copy a TMPDIR |
| `tools/ci` | `smoke.ps1` (Windows smoke test of the real exe, including DPI awareness, version details and resting), `smoke-unix.sh` (macOS/Linux), `verify-release.ps1` (the release: its draft, then the public one; its downloads go into the ledger), `dev-download.sh` (every development download of a release file, recorded in the ledger), `version.sh`, `live-players.sh` (real players on CI), `live-flathub.sh` (mpv, VLC and Celluloid from Flathub, canary only), `live-plex.sh` (a real Plex Media Server on CI), `live-jellyfin.sh` (real Jellyfin and Emby servers on CI), `canary-report.sh` (the weekly canary's issue), `package-macos.sh` (the .app), `test-noskip.sh` with the per-system list of tests that may skip (`allowed-skips.sh`), `must-pass.sh` (a live test must really run and pass), `run-annotated.sh`, fakes for Discord and HTTP |
| `docs/config.md` | every setting with its default |
| `docs/` | the settings reference (`config.md`), how it works (`how-it-works.md`), troubleshooting (`troubleshooting.md`), the roadmap (`roadmap.md`), the decisions (`decisions.md`), how AI is used (`ai.md`), this file, the README's images (`images/`) and the player logos (`players/`) |

Flags of the program: `--background` (start in the tray), `--after-update` (the copy an update started), `--home <folder>` (a data folder of its own, like `MPCRPC_HOME`), `--version`; on macOS `--serve` (the copy that keeps running after the app is opened, see `launch_darwin.go`).

## How players are read
`engine.fetchPlayer` asks, in this order: the MPC web interface (`/variables.html` on `port`, named by `core.PlayerOf`:
MPC-HC, MPC-BE or MPC-QT), MPC-QT's always-on pipe `cmdrkotori.mpc-qt.mpv`, mpv's pipe (`mpvPipe`), on macOS IINA's
socket (`iinaPipe`, an mpv option in IINA's settings), then VLC's web interface (`vlcPort`, only when `vlcPassword` is
set), on Linux the MPRIS players on the session bus (`mpris`; `engine.HasMPRIS`), on Windows PotPlayer's windows
(`internal/potplayer`: its documented WM_USER messages, the path back as WM_COPYDATA), Kodi's JSON-RPC web server
(`kodiAddress`, only when `kodiPassword` is set), then Plex, Stremio and last
Jellyfin / Emby, each only while signed in (`plexToken`, `stremioKey`, `jellyfinToken`; see below). **The first player
that plays a video wins; otherwise the first that plays music; otherwise the first that is paused; otherwise the first
that answers is reported.** A song (`core.IsAudioFile`: `.mp3`, `.flac`, ...) is shown as "Listening to" (`core.Listening`)
with its file name (`core.MusicMedia`), never looked up and not counted in the stats. MPC-HC must keep behaving exactly as before whenever
anything is added. On macOS and Linux a bare pipe name is a socket in `os.TempDir()` (`pipe.Path`; `~/` is expanded);
a copy started at login on macOS gets TMPDIR from `getconf DARWIN_USER_TEMP_DIR`, because launchd sets none and that
folder is where Discord, IINA and MPC-QT put their sockets. Players that report changes are also watched
(`engine/events.go`): mpv and IINA (`mpvipc.Watch`: one more IPC connection that observes `pause` and listens for seek
and file events), MPRIS players (`mpris.Watch`: D-Bus `PropertiesChanged` and `Seeked` signals) and Kodi
(`kodi.Watch`: its JSON-RPC over TCP on port 9090, on by default in Kodi). An event wakes the loop early (`nudge`:
250 ms later, at most every 4 s), and the check still reads what plays; tests set `Options.KodiEvents` and
`Options.MPRISBus` so no real Kodi or bus is reached. MPC-QT's pipe is only asked. MPRIS only reads players on the allowlist in
`internal/mpris` (music players and browsers offer MPRIS too and must never show as "Watching").

Plex (`internal/plex`): "Sign in to Plex" gets a token through a plex.tv PIN the user approves in their browser
(`cmd/mpcvibedrpc/plex.go`); the window never receives the token. While presence runs, a watcher looks the chosen
server up on plex.tv (`clients.plex.tv/api/v2/resources`, or uses `plexAddress`), checks `/identity` without the token (the token goes only to
the server that answered with the chosen id; a shared server gets its own `accessToken`, never the account's), and reads
Server-Sent Events from `/:/eventsource/notifications?filters=playing` (a `PlaySessionStateNotification` per player
report, every few seconds). Titles come from `/library/metadata/<ratingKey>` and are written like a file name
("Sample Show - S01E02 - Title", "Sample Movie (2020)") so the usual parsing and lookups apply. On a server the user owns
only their own sessions count (`/status/sessions`: user 1 or the account id); music and photos are skipped.
With `plexJwt` (off by default, `config.json` only) the sign-in uses Plex's newer 7-day tokens instead (`jwt.go`, as
developer.plex.tv's "Authenticating with Plex" describes): an Ed25519 key per install (`plexKey`, a secret), its JWK
sent with the PIN to `clients.plex.tv/api/v2/pins`, the token collected with a signed device JWT, and renewed
(`/api/v2/auth/nonce`, then `/api/v2/auth/token`) two days before it runs out (`plexAccount.keepFresh`, hourly).
Only tested against a fake plex.tv; turn it on by default only once a real sign-in has been tried with the owner's OK.

Stremio (`internal/stremio`): "Sign in to Stremio" gets the account's key through a link code (`link.stremio.com/api/v2/
create` and `read`, the TV apps' sign-in; only an https stremio.com page is ever opened); sign-out and Restore defaults
call `api.strem.io/api/logout`. Stremio has no "now playing" API: the watcher polls `datastoreMeta` (every library
entry's mtime) every 20 s and fetches changed entries with `datastoreGet`. Stremio's player (stremio-core) pushes the
entry on load, pause, resume and stop, and while it plays once `PUSH_TO_LIBRARY_EVERY` has passed since the last
regular push (90 s up to core 0.65.0, 180 s since 0.65.1; the extra pushes do not move that timer). A push with the
time moved on before the next regular one is due is a pause or stop (status taken down), one with the time unchanged
is a start or resume; the watcher learns each player's interval from its pushes (`judge`), and no push for the
interval plus ~55 s (180 s unless 90 s was learned) means it stopped. The API was learned from stremio-core's source and unit tests; there is no live test.

Jellyfin and Emby (`internal/jellyfin`, `cmd/mpcvibedrpc/jellyfin.go`): the user types the server's address on the
Accounts tab; `/System/Info/Public` says which it is (Emby may answer only under `/emby`, which is then kept in `jellyfinAddress`). Jellyfin signs in with Quick
Connect (`/QuickConnect/Initiate`, `Connect`, `/Users/AuthenticateWithQuickConnect`; the user enters the code in a
Jellyfin app); Emby, or Quick Connect off, with `/Users/AuthenticateByName` (the password is passed on once, never kept).
Every request carries `Authorization: MediaBrowser ...` (Jellyfin 12 ignores the legacy headers by default) and
`X-Emby-Authorization` / `X-Emby-Token` (Emby). The watcher polls `GET /Sessions` every 5 s; only the signed-in user's
sessions count (`UserId`), the position is moved on by the server's `Date` minus `LastPlaybackCheckIn` (Emby sends no check-in time: an unchanged report keeps its first time base). Sign-out and
Restore defaults call `/Sessions/Logout`. Error messages never carry the server's address (`jellyfin.Reason`).

**Resting** (`engine.rest`): while Discord's IPC endpoint is absent (`pipe.Exists`: a directory query on Windows,
a stat elsewhere; never a connection) and no settings window holds `/api/alive` (`server.Options.OnWindow` ->
`engine.Options.MayRest`), a tick asks no player, the Plex, Stremio and Jellyfin watchers are stopped, and `OnRest`
(`winsys.TrimMemory`) hands memory back. A window opening calls `engine.Wake`. Tests get no resting unless they set
`MayRest`.

**Clearing** (`tick.go`): a status counts as shown from the moment it is sent, also when Discord's answer does not
come (it may have been applied: a late answer once left statuses up for good); a clear Discord does not take keeps it
"shown", so the next check clears again; while nothing plays and Discord is connected it is cleared again every
`reclearEvery` (a minute, quietly); a file paused within `endSlack` (3 s) of its end has ended. Tests:
`internal/engine/stale_test.go` (the fake Discord's `slow` and `failClears`).

Known quirks: MPC-QT's web page reports a meaningless `playbackRate` (its pipe is preferred) and reports seeking as
state 3 (treated as playing). VLC 3 rounds `time` to whole seconds (position x length is used), VLC 4 uses a flat
playlist. With the default app name, the tooltip names mpv and VLC as themselves, and the default corner icon (MPC's
logo) is left out for players outside the MPC family. The default Discord application (`clientId`, which holds the
`mpc-hc` images) was registered by the upstream project, not by this one: every install depends on it staying there.

"Set up the player connection" (`winsys.EnableMpcWebInterface`): MPC-HC/MPC-BE get their web interface on (registry
and ini; they are closed and reopened, after the user confirms) and **this-PC-only when the button switches it on**;
mpv gets `input-ipc-server` in the `mpv.conf` it reads (a name the user set is kept and adopted); VLC gets
`extraintf=http`, a password and `http-host=127.0.0.1` in the `vlcrc` it reads (existing password/port kept and
adopted; VLC is closed/reopened only if something must change, because it rewrites `vlcrc` on exit). MPC-QT needs nothing.
On macOS and Linux (`winsys.enableUnix`): mpv gets the full socket path in `~/.config/mpv/mpv.conf`; IINA gets
`enableAdvancedSettings` and an `input-ipc-server` row in `userOptions` through `defaults write com.colliderli.iina`
(an existing row is kept and adopted; IINA is not closed, the user reopens it); VLC on macOS as on Windows, in
`~/Library/Preferences/org.videolan.vlc/vlcrc` (closed with AppleScript and reopened with `open -b`); on Linux VLC and
the other MPRIS players need nothing.

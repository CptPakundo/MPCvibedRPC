# CLAUDE.md

Guidance for Claude Code sessions working on this repository. Keep it current: update it in the same pull request
whenever a change makes something here wrong (a new player, setting, CI step, release step or rule).

## What this is
MPCvibedRPC is a small Windows tray app (Go, standard library only, one ~8 MB exe, nothing to install) that shows what
a video player is playing as Discord Rich Presence: "Watching <title>" with a progress bar, cover art, episode titles,
ratings, and each player's logo. Supported players: **MPC-HC, MPC-BE, MPC-QT, mpv, VLC and PotPlayer**, **Kodi**, and **Plex**, **Stremio**, **Jellyfin** and
**Emby** on every system (any device the user plays on, read from their Plex server after "Sign in to Plex", from
their Stremio library after "Sign in to Stremio", or from their own Jellyfin or Emby server; all sign-ins are on the
Accounts tab). It talks to the Discord desktop app over its local IPC pipe; the settings window is a local web page
shown in an Edge/Chrome `--app=` window (tabs General, Accounts, Look, Titles & art, Privacy, Advanced, Help; each group of settings is one card; updates are under General).
The tray (Windows) and menu bar (macOS) menus show a status line, then Open settings, Show on Discord / Stop showing
on Discord, Hide what I'm watching (`hideTitle`, ticked) and Quit. The window says "Show on Discord" / "status", not
"presence".

It also runs on **macOS** (a universal `.app` with no Dock icon; IINA, mpv, VLC, MPC-QT) and **Linux** (a plain binary;
MPRIS players such as VLC, Celluloid, Haruna, plus mpv). macOS has a menu bar icon and shows the settings in a WebKit
window of its own (`internal/winsys/macapp_darwin.go`: Cocoa through cgo, so the macOS build is made on a Mac with
cgo; without cgo it falls back to the browser and no icon). Linux has no tray icon: running the program again shows
the window, and Quit is in the window. Windows is the main system and its behaviour must not change for the others.
The owner has no Mac or Linux machine: those systems are tested on GitHub's runners (see CI below), and once by a
friend of the owner on a real Mac.

- The program is **AI-generated ("vibe coded")**. The README says so once, near the top, and that is enough: the owner
  chose a light touch. A short "Made with AI" (exe properties, the macOS app, the repo description) is fine; no heavy
  disclaimers elsewhere. Never present the code as written by the repository owner.
- The idea comes from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC) (MIT). That credit
  stays in `README.md`, `LICENSE` and `THIRD-PARTY-NOTICES.md`. Otherwise, docs present the program as it is now:
  no history of earlier versions or former names outside `.github/release-notes.md` and the in-app changelog
  (`internal/assets/changelog.json`: short lines, one emoji each, newest version first).
- Versions are `0.9.x` (pre-1.0, three-part, needed by the updater). The current version is in `cmd/mpcvibedrpc/main.go`.
- **Roadmap:** `docs/roadmap.md` holds the long-term goals, the non-goals, the owner's open decisions and a watch list of
  outside facts (Discord, Go and OS floors, Plex, Jellyfin, Stremio, catalogs, distribution). Read it before proposing
  or planning a feature, a new service or a distribution change, and don't propose a listed non-goal without saying so.
  Update it in the same pull request when a change finishes a step, moves a goal or settles a decision, and refresh the
  watch list when you learn an outside fact changed (with the date checked).

## Ground rules
- **Privacy first.** No personal data in commits, code, docs, tests, screenshots or release files: no email addresses,
  local paths or user names, machine details, tokens. Commit as `git -c user.name="Claude" -c user.email="noreply@anthropic.com"`
  and never with anyone's personal email. The diff is scanned for personal data before the first push (see Releases).
- **Generic names.** UI text, docs, code comments and new tests use made-up titles ("Sample Show S01E02", "Sample Movie
  (2020)"), never real shows, films or episode numbers. Exempt: the recorded golden data in `internal/*/testdata/*.gz`,
  and real behaviour that is about a specific source (the Bulbapedia episode lists).
- **Golden test data is frozen.** `internal/*/testdata/*.gz` was recorded once and its generators no longer exist. Never
  regenerate or "fix" it; make code match it.
- **Never touch a real setup while testing.** Tests and manual checks must not contact the real Discord, start presence
  for a real player, or change a real player's settings, the registry or `%APPDATA%`/`%LOCALAPPDATA%` without the
  owner's explicit OK. Use the fakes and throwaway data folders described below.
- **No regressions, no churn.** Weigh pros and cons; don't change working behaviour for its own sake. New behaviour that
  affects existing users stays off or unchanged by default unless the owner decides otherwise.
- **Be exact about what was tested.** Say what ran for real, what ran against fakes, and what was not tested.

## Layout
| Path | Job |
|---|---|
| `cmd/mpcvibedrpc` | `main.go` (flags, single instance, `var version`), `app.go` (the `app` type: start-up and quit order), `api.go` (the local API handlers: `state`, `status`, `save`, `reset`, `start`, `stop`, `mpc-web`, `diagnostics`, `update-*`, `quit`, ...), `update.go` (checks, install), `tray.go` (icon, tooltip), `accounts.go` (the `approval` helper the Plex, Stremio and Jellyfin sign-ins share, and their common endpoints) with `plex.go`, `stremio.go`, `jellyfin.go`, `logger.go` |
| `internal/engine` | the tick loop: asks the players, builds and sends the activity, pause clearing, hide rules, preview, status for the window, the local stats (`stats.go`, `stats.json`: counts only, never titles); split by job: `engine.go` (types, Start/Stop, Status), `players.go` (`fetchPlayer` and its `picker`), `remote.go` (the Plex, Stremio and Jellyfin watchers as one list, resting), `discord.go`, `tick.go` |
| `internal/core` | settings and defaults (`config.go`), filename parsing (`parse.go`), the activity (`activity.go`), preview, privacy rules, the online-service catalog (`sources.go`) |
| `internal/mpvipc`, `internal/pipe` | mpv JSON IPC (mpv, MPC-QT, IINA); named pipes on Windows, Unix sockets elsewhere (shared with `internal/discord`) |
| `internal/mpris`, `internal/dbus` | Linux players through MPRIS (an allowlist of video players); a minimal D-Bus client; `dbus/dbustest` is a fake bus for tests |
| `internal/vlchttp` | VLC's web interface, read-only (`status.json`, `playlist.json`) |
| `internal/potplayer`, `internal/kodi` | PotPlayer (Windows: window messages from its control header `InternalSimpleCmd.h`); Kodi (JSON-RPC: `Player.GetActivePlayers`, `GetItem`, `GetProperties`); both read-only. Player logos for Discord are files in `docs/players`, named by `core/icons.go` (sources in `THIRD-PARTY-NOTICES.md`) |
| `internal/plex` | Plex: the plex.tv PIN sign-in, the account's servers (`account.go`), one server's play notifications (`watcher.go`), metadata to a file-like name (`meta.go`); read-only |
| `internal/stremio` | Stremio: the link-code sign-in and sign-out (`account.go`), the account's library as Stremio's player updates it (`watcher.go`); read-only |
| `internal/jellyfin` | Jellyfin and Emby: the server lookup, Quick Connect and password sign-in, sign-out (`account.go`), the session list (`watcher.go`), item to a file-like name (`meta.go`); read-only |
| `internal/remote` | what the Plex, Stremio and Jellyfin readers share: the problem text, nil-safe log and clock types, the OS name, network-error wording, the same-origin redirect check (their titles are written like file names by `core/names.go`) |
| `internal/discord` | Discord's local IPC protocol |
| `internal/artwork`, `internal/jsre` | catalog lookups, episode titles; a regex engine with JavaScript semantics that the matching rules rely on |
| `internal/store`, `internal/jsonx` | `config.json`, the settings window layout (`schema.go`), validation; a JSON reader/writer that keeps the file's key order and unknown keys and prints numbers like JavaScript |
| `internal/server`, `internal/assets` | the local API on 127.0.0.1 (per-launch token, host check, CSP) and the whole page (`ui.html`), the changelog (`changelog.json`) |
| `internal/winsys` | the operating system: tray (Windows), autostart (registry, LaunchAgent, XDG autostart: `autostart_unix.go`), single instance (mutex, file lock), the window's browser, "Set up the player connection" (MPC registry/ini, `mpv.conf`, `vlcrc`, IINA's settings: `setup_unix.go`); by topic: `run.go`, `mpcsetup.go` (`EnableMpcWebInterface`), `window.go` (browser, open URL/folder), `autostart.go`, `tray.go` (`TrayOptions`). The macOS/Linux parts live in untagged files where possible so their logic is tested on every system |
| `internal/diag` | "Copy diagnostics": titles and file names kept (a wrong title is traced back to its file name); folder paths, URL paths and queries, server names, sign-ins, keys and the Never show list left out |
| `internal/updater` | GitHub release check, download, checksum, swap, restart |
| `tools/mkrsrc`, `tools/mkico` | exe resources (version info, icon, DPI manifest); the icon generator (`-icns` writes the macOS app icon) |
| `cmd/mpcvibedrpc/launch_darwin.go` | macOS: started from the Finder, the program hands over to a detached `--serve` copy and leaves (a build without the Cocoa code then still finds the running copy when opened again; with it, macOS sends the running app a reopen event, which shows the window); also gives a login-started copy a TMPDIR |
| `tools/ci` | `smoke.ps1` (Windows smoke test of the real exe, including DPI awareness, version details and resting), `smoke-unix.sh` (macOS/Linux), `verify-release.ps1` (the release: its draft, then the public one), `version.sh`, `live-players.sh` (real players on CI), `live-plex.sh` (a real Plex Media Server on CI), `live-jellyfin.sh` (real Jellyfin and Emby servers on CI), `package-macos.sh` (the .app), `test-noskip.sh` with the per-system list of tests that may skip (`allowed-skips.sh`), `must-pass.sh` (a live test must really run and pass), `run-annotated.sh`, fakes for Discord and HTTP |
| `docs/config.md` | every setting with its default |

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
folder is where Discord, IINA and MPC-QT put their sockets. MPRIS only reads players on the allowlist in
`internal/mpris` (music players and browsers offer MPRIS too and must never show as "Watching").

Plex (`internal/plex`): "Sign in to Plex" gets a token through a plex.tv PIN the user approves in their browser
(`cmd/mpcvibedrpc/plex.go`); the window never receives the token. While presence runs, a watcher looks the chosen
server up on plex.tv (`clients.plex.tv/api/v2/resources`, or uses `plexAddress`), checks `/identity`, and reads
Server-Sent Events from `/:/eventsource/notifications?filters=playing` (a `PlaySessionStateNotification` per player
report, every few seconds). Titles come from `/library/metadata/<ratingKey>` and are written like a file name
("Sample Show - S01E02 - Title", "Sample Movie (2020)") so the usual parsing and lookups apply. On a server the user owns
only their own sessions count (`/status/sessions`: user 1 or the account id); music and photos are skipped.

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

## Build and test
```bash
go vet ./...
go test -count=1 ./...
go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=X.Y.Z" -o dist/MPCvibedRPC.exe ./cmd/mpcvibedrpc
```
- The workflows carry no version: `tools/ci/version.sh` reads it from `cmd/mpcvibedrpc/main.go` (VERSION, and NEXT for
  the smoke test's update) and, on a tag, fails when the tag does not match.
- CI (`.github/workflows/ci.yml`): Linux runs vet, a **gofmt gate**, `govulncheck` (pinned; a Go security fix
  means bumping the exact Go patch in every `setup-go`), and the tests with `-race`; Windows and macOS run
  the tests. A push or PR that changes only documentation (top-level `.md` files, `docs/`, issue templates,
  `.github/release-notes.md`) is not tested (`paths-ignore`); anything else runs everything. A pull request from this repository
  is tested by its branch's push run (the `pull_request` run skips itself; only forks use it); `master` itself is not tested on push (its commits come from a tested branch, and the tag run tests again), and a newer push cancels
  a run still going on the same branch. Then: Windows builds the exe and runs `tools/ci/smoke.ps1`; `unix` builds the Linux binaries and the macOS
  app and runs `tools/ci/smoke-unix.sh` (API, login item, player setup, second start, and on macOS opening the app
  through LaunchServices); `players` runs `tools/ci/live-players.sh`: the live engine test against real mpv, VLC (MPRIS),
  mpv-mpris and Celluloid on Linux, and mpv and IINA (set up by the real program first) on macOS. `plex` runs
  `tools/ci/live-plex.sh`: the live engine test against the official Plex Media Server image (unclaimed, no account)
  with a stand-in player that reports playback through `/:/timeline`. `jellyfin` runs `tools/ci/live-jellyfin.sh` for
  the official Jellyfin and Emby images (a matrix): the first-run wizard with a made-up user, a stand-in player (`/Sessions/Playing`),
  the real sign-in (password and Quick Connect) and the live engine test. `players-windows` runs `tools/ci/live-windows.sh`: real
  PotPlayer and Kodi from Chocolatey (PotPlayer without its stale package checksum), a generated video in each, the live
  engine test, and `TestLivePotPlayer` (the real PotPlayer sends the file's path). **This is the only
  place macOS and Linux run for real**, so read those logs after every change that touches them. A Linux or cloud
  environment can run almost everything else; the tray, the window, the registry and the Windows players need Windows.
- `tools/ci/test-noskip.sh` fails CI when a test skips that should run on that OS. A new test that is Linux-only,
  Windows-only or opt-in must be added to `tools/ci/allowed-skips.sh` (ci.yml and release.yml both use it). The live
  test scripts run through `must-pass.sh`, so a live test that skips fails its CI job.
- Engine tests use a fake Discord (Unix socket on Linux, named pipe on Windows) and fake players (`players_test.go`,
  `vlc_test.go`). Timing matters: wait for state with the `eventually()` helper rather than reading it once.
  `Options.Pipes` keeps a real local MPC-QT/mpv/IINA out of the tests and `Options.MPRIS` the real session bus; VLC is
  only asked when a test sets a password. On macOS a socket path may only be 104 bytes, so test sockets go in a short
  folder under /tmp there (`sockDir`).
- Run `go test -race` somewhere that supports it (Linux CI) after concurrency changes.

### Working on Windows
- Working-copy files are CRLF (git `autocrlf`; the index is LF), except `internal/jsre/*_table.go`. Some editing tools
  silently rewrite a CRLF file as LF; convert it back before committing so diffs stay small.
- `gofmt -l` flags every CRLF file on Windows. Check formatting on LF copies (or rely on the Linux CI gate), and never
  `gofmt -w` a CRLF file in place.
- Shell scripts that rewrite many files can damage them silently; read the diff before committing.

### Running the real program safely
- `MPCRPC_HOME=<scratch folder>` gives a copy its own data folder **and its own autostart value name**, so it can never
  touch an installed copy. A config such as
  `{"app":{"openWindow":false,"startPresence":false,"checkUpdates":false,"welcomeSeen":true}}` keeps it quiet.
- The API port and token are in `<MPCRPC_HOME>/ipc.json`; the page is `http://127.0.0.1:<port>/?t=<token>`. To test window
  states without Discord, override `window.fetch` for `/api/status` in the page.
- `api/start` with nothing playing is safe: Discord is only contacted once something plays ("standby"). To keep a real
  player from being picked up, point `port` at an unused port and leave `mpvPipe`/`vlcPassword` empty.
- Update checks: `MPCRPC_UPDATE_API=http://127.0.0.1:<port>` with `tools/ci/fake-http.ps1`.
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

## Recipes
**New setting:** a field with a `json` tag in `core.Config` and its default in `DefaultConfig()`; a `Field` in
`store/schema.go` if it belongs in the window (int fields need Min/Max; `secret` for passwords and keys); a row in
`docs/config.md`; `diag.summarise` if the value is private (shown as "(set)"). Defaults that change behaviour for
existing users need the owner's decision and a line in the release notes.

**New player:** reader package with unit tests on recorded-style responses; wire into `fetchPlayer` *after* the
existing players; engine tests including player order; setup support in `winsys` if the player needs switching on;
a live run in `tools/ci/live-players.sh` if it exists on Linux or macOS (a Linux MPRIS player only needs its bus-name
part in `mpris.videoPlayers`, its process name in `winsys.unixPlayers`, and the lists below);
then every place that lists players: `ui.html` (welcome card, "No player is answering" card, "Nothing playing" hint,
Privacy "What else leaves this PC"; the macOS and Linux texts are in `OS_TEXT`), README (tagline, What you get, Getting started, the Players table, Help), `docs/how-it-works.md` (Players in detail), `docs/troubleshooting.md`, `docs/development.md` (Where things are),
`docs/config.md`, the help texts in `store/schema.go` (`port`, `mpris`, `iinaPipe`), `SECURITY.md`, `.github/ISSUE_TEMPLATE/bug_report.yml`, the exe description in `tools/mkrsrc/main.go`,
the header comment in `main.go`, the engine's "Looking for a player on ..." log line, `diag` and the repo description/topics.

**UI text:** a recorded store test pins the label "MPC-HC web interface port"; change only its help text. The window
follows "less is more": short help texts, one term per thing, rarely used actions under Advanced > Maintenance. Keep the README,
`docs/how-it-works.md`, `docs/config.md`, schema help text and release notes in sync with any behaviour change. The
README is for people: short, scannable, one emoji per section heading; details go in `docs/`.

## Releases
One branch and one PR per release: the change, the version bump and the release text together.
1. Branch; change; in the same branch bump `var version` in `cmd/mpcvibedrpc/main.go` (nothing else carries the
   version: the workflows and smoke tests read it; docs use `X.Y.Z`), add the version's entry at the top of
   `internal/assets/changelog.json` (`TestChangelog` fails until it matches `main.go`) and rewrite
   `.github/release-notes.md` (the release body). Screenshots (`docs/images/window-*.jpg`) are retaken only when the
   part of the window they show changed, as `docs/development.md` > Screenshots describes (made-up titles only).
2. Locally: `go vet ./...` (also `GOOS=linux`, `GOOS=darwin`), gofmt on LF copies, `go test -count=1 ./...`, and a
   scan of the diff and new files for personal data (the owner's name, email, local paths) before the first push.
   Then push, `gh pr create`, wait for CI (`gh run watch`) and read the logs to confirm the expected steps ran.
3. With the owner's go-ahead: `gh pr merge <n> --rebase --delete-branch`, then `git tag -a vX.Y.Z -m "MPCvibedRPC X.Y.Z"`
   with the Claude identity on the merged commit, `git push origin vX.Y.Z`. The release workflow checks the tag
   against `main.go`, tests, builds and smoke-tests every program on its own system (the Windows smoke test covers DPI
   awareness, version details, resting and a self-update against a fake server), writes the `.sha256` files, attests,
   publishes it as a **draft**, and then runs `verify` (`tools/ci/verify-release.ps1`): on the draft the ten assets,
   every checksum and build record and the exe's version; only if they pass is the release made public (and latest),
   followed by the checks that need it public: that it is the latest release, and a real self-update from the previous
   release. A release whose draft checks fail stays a draft (nobody's updater sees it): fix, then delete the draft and
   the tag, or publish it by hand once checked.
4. Read the `verify` job's log (`gh run view --log`) and report. Nothing needs re-running locally after a release.
5. A release with new features also updates the repository to match: its description and topics
   (`gh repo edit --description ... --add-topic ...`), so they name every player and system the program supports.
   The owner gave a standing go-ahead for this.

Merging, tagging and publishing are outward-facing: do them only with the owner's go-ahead.

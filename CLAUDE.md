# CLAUDE.md

Guidance for Claude Code sessions working on this repository. Keep it current: update it in the same pull request
whenever a change makes something here wrong (a new player, setting, CI step, release step or rule).

## What this is
MPCvibedRPC is a small Windows tray app (Go, standard library only, one ~8 MB exe, nothing to install) that shows what
a video player is playing as Discord Rich Presence: "Watching <title>" with a progress bar, cover art, episode titles,
ratings. Supported players: **MPC-HC, MPC-BE, MPC-QT, mpv and VLC**, and **Plex** on every system (any device the user plays on,
read from their Plex server after "Sign in to Plex"). It talks to the Discord desktop app over its
local IPC pipe; the settings window is a local web page shown in an Edge/Chrome `--app=` window.

It also runs on **macOS** (a universal `.app` with no Dock icon; IINA, mpv, VLC, MPC-QT) and **Linux** (a plain binary;
MPRIS players such as VLC, Celluloid, Haruna, plus mpv). macOS has a menu bar icon and shows the settings in a WebKit
window of its own (`internal/winsys/macapp_darwin.go`: Cocoa through cgo, so the macOS build is made on a Mac with
cgo; without cgo it falls back to the browser and no icon). Linux has no tray icon: running the program again shows
the window, and Quit is in the window. Windows is the main system and its behaviour must not change for the others.
The owner has no Mac or Linux machine: those systems are tested on GitHub's runners (see CI below), and once by a
friend of the owner on a real Mac.

- The program is **entirely AI-generated**, and the repo says so (README banner, release notes, exe file properties,
  repo description). Keep those statements; never present the code as written by the repository owner.
- The idea comes from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC) (MIT). That credit
  stays in `README.md`, `LICENSE` and `THIRD-PARTY-NOTICES.md`. Otherwise, docs present the program as it is now:
  no changelog of earlier versions or former names outside `.github/release-notes.md`.
- Versions are `0.9.x` (pre-1.0, three-part, needed by the updater). The current version is in `cmd/mpcvibedrpc/main.go`.

## Ground rules
- **Privacy first.** No personal data in commits, code, docs, tests, screenshots or release files: no email addresses,
  local paths or user names, machine details, tokens. Commit as `git -c user.name="Claude" -c user.email="noreply@anthropic.com"`
  and never with anyone's personal email. Release checks scan the exe for personal strings (see Releases).
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
| `cmd/mpcvibedrpc/main.go` | wiring, single instance, local API handlers (`state`, `status`, `save`, `reset`, `start`, `stop`, `mpc-web`, `diagnostics`, `clear-cache`, `update-*`, `plex*`, `quit`, ...), update checks, tray, quit order |
| `internal/engine` | the tick loop: asks the players, builds and sends the activity, pause clearing, hide rules, preview, status for the window |
| `internal/core` | settings and defaults (`config.go`), filename parsing (`parse.go`), the activity (`activity.go`), preview, privacy rules, the online-service catalog (`sources.go`) |
| `internal/mpvipc`, `internal/pipe` | mpv JSON IPC (mpv, MPC-QT, IINA); named pipes on Windows, Unix sockets elsewhere (shared with `internal/discord`) |
| `internal/mpris`, `internal/dbus` | Linux players through MPRIS (an allowlist of video players); a minimal D-Bus client; `dbus/dbustest` is a fake bus for tests |
| `internal/vlchttp` | VLC's web interface, read-only (`status.json`, `playlist.json`) |
| `internal/plex` | Plex: the plex.tv PIN sign-in, the account's servers (`account.go`), one server's play notifications (`watcher.go`), metadata to a file-like name (`meta.go`); read-only |
| `internal/discord` | Discord's local IPC protocol |
| `internal/artwork`, `internal/jsre` | catalog lookups, episode titles; a regex engine with JavaScript semantics that the matching rules rely on |
| `internal/store` | `config.json`, the settings window layout (`schema.go`), validation |
| `internal/server`, `internal/assets` | the local API on 127.0.0.1 (per-launch token, host check, CSP) and the whole page (`ui.html`) |
| `internal/winsys` | the operating system: tray (Windows), autostart (registry, LaunchAgent, XDG autostart: `autostart_unix.go`), single instance (mutex, file lock), the window's browser, "Set up the player connection" (MPC registry/ini, `mpv.conf`, `vlcrc`, IINA's settings: `setup_unix.go`). The macOS/Linux parts live in untagged files where possible so their logic is tested on every system |
| `internal/diag` | "Copy diagnostics" with redaction (titles, paths, URL queries, secrets) |
| `internal/updater` | GitHub release check, download, checksum, swap, restart |
| `tools/mkrsrc`, `tools/mkico` | exe resources (version info, icon, DPI manifest); the icon generator (`-icns` writes the macOS app icon) |
| `cmd/mpcvibedrpc/launch_darwin.go` | macOS: started from the Finder, the program hands over to a detached `--serve` copy and leaves (a build without the Cocoa code then still finds the running copy when opened again; with it, macOS sends the running app a reopen event, which shows the window); also gives a login-started copy a TMPDIR |
| `tools/ci` | `smoke.ps1` (Windows smoke test of the real exe), `smoke-unix.sh` (macOS/Linux), `live-players.sh` (real players on CI), `live-plex.sh` (a real Plex Media Server on CI), `package-macos.sh` (the .app), `test-noskip.sh`, fakes for Discord and HTTP |
| `docs/config.md` | every setting with its default |

## How players are read
`engine.fetchPlayer` asks, in this order: the MPC web interface (`/variables.html` on `port`, named by `core.PlayerOf`:
MPC-HC, MPC-BE or MPC-QT), MPC-QT's always-on pipe `cmdrkotori.mpc-qt.mpv`, mpv's pipe (`mpvPipe`), on macOS IINA's
socket (`iinaPipe`, an mpv option in IINA's settings), then VLC's web interface (`vlcPort`, only when `vlcPassword` is
set), on Linux the MPRIS players on the session bus (`mpris`; `engine.HasMPRIS`), and last Plex, only while signed in
(`plexToken`; see below). **The first player that is
playing wins; otherwise the first that answers is reported.** MPC-HC must keep behaving exactly as before whenever
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

Known quirks: MPC-QT's web page reports a meaningless `playbackRate` (its pipe is preferred) and reports seeking as
state 3 (treated as playing). VLC 3 rounds `time` to whole seconds (position x length is used), VLC 4 uses a flat
playlist. With the default app name, the tooltip names mpv and VLC as themselves.

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
go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=0.9.10" -o dist/MPCvibedRPC.exe ./cmd/mpcvibedrpc
```
- CI (`.github/workflows/ci.yml`): Linux runs vet, a **gofmt gate**, and the tests with `-race`; Windows and macOS run
  the tests. Then: Windows builds the exe and runs `tools/ci/smoke.ps1`; `unix` builds the Linux binaries and the macOS
  app and runs `tools/ci/smoke-unix.sh` (API, login item, player setup, second start, and on macOS opening the app
  through LaunchServices); `players` runs `tools/ci/live-players.sh`: the live engine test against real mpv, VLC (MPRIS),
  mpv-mpris and Celluloid on Linux, and mpv and IINA (set up by the real program first) on macOS. `plex` runs
  `tools/ci/live-plex.sh`: the live engine test against the official Plex Media Server image (unclaimed, no account)
  with a stand-in player that reports playback through `/:/timeline`. **This is the only
  place macOS and Linux run for real**, so read those logs after every change that touches them. A Linux or cloud
  environment can run almost everything else; the tray, the window, the registry and the Windows players need Windows.
- `tools/ci/test-noskip.sh` fails CI when a test skips that should run on that OS. A new test that is Linux-only,
  Windows-only or opt-in must be added to the allowlists in `ci.yml`.
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
  `MPCRPC_LIVE_IINA_PIPE`, `MPCRPC_LIVE_PLEX_ADDRESS`, `MPCRPC_LIVE_PLEX_TOKEN`; `MPCRPC_LIVE_PLAYER` is the name shown: mpv, IINA, VLC, Celluloid, Plex, ...). Use portable player builds in a scratch
  folder (MPC-QT: `portable.txt`; MPC-BE: an ini next to the exe; mpv: `portable_config\`; VLC: a `portable\` folder
  next to `vlc.exe`), verify their published checksums, and check `%APPDATA%`, `%LOCALAPPDATA%` and `HKCU\Software`
  afterwards. Test videos can be made with mpv's encoder from `lavfi` test sources, with generic names.
- Plex: never sign in to a real Plex account, and never call plex.tv's sign-in, from tests or manual checks without the
  owner's OK. The sign-in, the server list and the notifications are tested against fakes (`internal/plex`,
  `cmd/mpcvibedrpc/plex_test.go`, `internal/engine/plex_test.go`); the window's Plex states can be shown by overriding
  `window.fetch` for `/api/plex*`.
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
Privacy "What else leaves this PC"; the macOS and Linux texts are in `OS_TEXT`), README (intro, Use it, Requirements, Players table, Troubleshooting, layout table),
`docs/config.md`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/bug_report.yml`, the exe description in `tools/mkrsrc/main.go`,
the header comment in `main.go`, the engine's "Looking for a player on ..." log line, `diag` and the repo description/topics.

**UI text:** a recorded store test pins the label "MPC-HC web interface port"; change only its help text. Keep README
behaviour bullets, `docs/config.md`, schema help text and release notes in sync with any behaviour change.

## Releases
1. Branch, change, `go vet ./...` (also with `GOOS=linux` and `GOOS=darwin`), gofmt, `go test -count=1 ./...`, push,
   open a PR (`gh pr create`), wait for CI and read the logs to confirm the expected steps ran, merge with
   `gh pr merge <n> --rebase --delete-branch`.
2. Version bump: replace the next version with the one after it, then the current with the next, in
   `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `README.md`, `cmd/mpcvibedrpc/main.go`,
   `tools/ci/fake-http.ps1`, `tools/ci/smoke.ps1`, `tools/mkrsrc/main.go` and the placeholder in `bug_report.yml`.
   Rewrite the top of `.github/release-notes.md` (it is the release body; keep the blank line before the
   "Entirely AI-generated" banner).
3. `git tag -a vX.Y.Z -m "MPCvibedRPC X.Y.Z"` with the Claude identity, `git push origin vX.Y.Z`. The release workflow
   builds and smoke-tests the Linux binaries and the macOS app on their own runners, then on Windows tests, builds and
   smoke-tests the exe, writes a `.sha256` for each program, attests them all and publishes.
4. Verify: ten assets (`MPCvibedRPC.exe`, `MPCvibedRPC-linux-amd64`, `MPCvibedRPC-linux-arm64`,
   `MPCvibedRPC-macos.zip`, a `.sha256` for each, `LICENSE`, `THIRD-PARTY-NOTICES.md`); checksums; file version;
   `gh attestation verify <file> --repo CptPakundo/MPCvibedRPC` passes for each program and fails for a one-byte-altered
   copy; a string scan of each program finds no personal data (only the public module path and the bundle identifier);
   the running exe is per-monitor DPI-aware (2); a real self-update from the previous published release in a throwaway
   `MPCRPC_HOME` ends on the new version with a hash equal to the release. (The first release with Linux files cannot
   self-update on Linux from an older one: older releases have no Linux program.)

Merging, tagging and publishing are outward-facing: do them only with the owner's go-ahead.

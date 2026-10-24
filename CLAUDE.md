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
"presence". Updates are looked for 5 s after start, then every six hours; with `autoUpdate` (on by default; Windows
and Linux) a found update installs itself at a quiet moment (`quietMoment`: nothing shown on Discord, no settings
window), and the new copy starts in the tray (`auto-updated` marker in the data folder) and says so once. While an
update waits, the tray and menu bar icon show a red dot (`assets.IconUpdate`, made by `tools/mkico -dot`; macOS:
`winsys.menuBarIcon`). Test copies keep `checkUpdates` off, which also keeps them from installing anything.

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
  no history of earlier versions or former names outside the in-app changelog (`internal/assets/changelog.json`: short
  lines, one emoji each, newest version first; its newest entry is also the GitHub release's text, see Releases).
- Versions are `0.9.x` (pre-1.0, three-part, needed by the updater). The current version is in `cmd/mpcvibedrpc/main.go`.
- **Roadmap:** `docs/roadmap.md` holds the long-term goals, the non-goals, the owner's open decisions and a watch list of
  outside facts (Discord, Go and OS floors, Plex, Jellyfin, Stremio, catalogs, distribution). Read it before proposing
  or planning a feature, a new service or a distribution change, and don't propose a listed non-goal without saying so.
  Update it in the same pull request when a change finishes a step, moves a goal or settles a decision, and refresh the
  watch list when you learn an outside fact changed (with the date checked).
- **Decisions:** `docs/decisions.md` is the public record of the product decisions, each with its reason. Add a line
  when the owner settles one, and don't change listed behaviour without saying which decision it touches.
  `docs/ai.md` says how the program is made and checked; keep it true when the process changes.
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
- **Release files are only downloaded through `tools/ci/dev-download.sh`** (`download BY TAG DIR [PATTERN]`, or
  `record BY TAG/FILE[=N]` for a download made another way, such as a test copy updating itself), never with a bare
  `gh release download`, a browser or curl. It records the download in the ledger on the `stats` branch, so the
  download counter leaves it out. Checking a release without downloading: GitHub lists each file's sha256 as `digest`
  (`gh api repos/OWNER/REPO/releases/tags/vX.Y.Z`).

## Layout and how players are read
The package table ("Where things are") and the full account of how each player is read (the order, the pipes and web
interfaces, Plex, Stremio, Jellyfin/Emby, the event watchers, resting, clearing, the known quirks and "Set up the
player connection") are in `docs/development.md`; keep them current there, in the same pull request as the change.
The rules every change must respect:
- **The first player that plays a video wins; otherwise the first that plays music; otherwise the first that is
  paused; otherwise the first that answers.** A song (`core.IsAudioFile`) is "Listening to" with its file name, never
  looked up and not counted in the stats.
- **MPC-HC must keep behaving exactly as before whenever anything is added.** A new player is asked after the existing
  ones (`engine.fetchPlayer`: the MPC web interface, MPC-QT's pipe, mpv, IINA, VLC, MPRIS, PotPlayer, Kodi, then Plex,
  Stremio and last Jellyfin/Emby, the last three only while signed in).
- MPRIS reads only the allowlist in `internal/mpris`: music players and browsers must never show as "Watching".
- The readers are read-only. Never sign in to a real Plex, Stremio, Jellyfin or Emby account, or call their sign-in
  services, from tests or by hand without the owner's OK; every reader has fakes (see "Running the real program safely").
- Resting (`engine.rest`: Discord's endpoint absent and no window open) asks no player and stops the watchers; tests get
  no resting unless they set `MayRest`. Clearing (`tick.go`): a sent status counts as shown until a clear is confirmed,
  and an idle status is cleared again every minute; `internal/engine/stale_test.go` pins it.
- The default Discord application (`clientId`) was registered by the upstream project; every install depends on it.

## Build and test
```bash
go vet ./...
go test -count=1 ./...
go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=X.Y.Z" -o dist/MPCvibedRPC.exe ./cmd/mpcvibedrpc
```
- The workflows carry no version: `tools/ci/version.sh` reads it from `cmd/mpcvibedrpc/main.go` (VERSION, and NEXT for
  the smoke test's update) and, on a tag, fails when the tag does not match.
- CI (`.github/workflows/ci.yml`): Linux runs vet, a **gofmt gate**, `govulncheck` (pinned; a Go security fix
  means bumping the exact Go patch in every `setup-go`), `staticcheck` (pinned; without ST1005, because the
  program's error texts are sentences the window shows) and `deadcode -test` (every function must be reachable from
  the program or a test: delete what is not, or say in a comment why it stays), and the tests with `-race`; Windows
  and macOS run the tests. The Go patch lives in `go.mod` alone (`go 1.27.3`; every `setup-go` reads `go-version-file`).
  A push or PR that changes only documentation (top-level `.md` files, `docs/`, issue templates) is not tested
  (`paths-ignore`); anything else runs the fast jobs. A pull request from this repository
  is tested by its branch's push run (the `pull_request` run skips the test job; only forks use it); `master` itself is not tested on push (its commits come from a tested branch, and the tag run tests again), and a newer push cancels
  a run still going on the same branch. Then: Windows builds the exe (from an LF checkout, `CGO_ENABLED=0`) and runs `tools/ci/smoke.ps1`; `reproduce` rebuilds that exe on
  Linux with `tools/ci/reproduce.sh` and fails unless it is the same file byte for byte; `unix` builds the Linux binaries and the macOS
  app and runs `tools/ci/smoke-unix.sh` (API, login item, player setup, second start, and on macOS opening the app
  through LaunchServices). The build-and-smoke steps are the composite actions `.github/actions/build-windows` and
  `build-unix`, shared with `release.yml`. **The live jobs run once per pull request** (when it is opened, reopened or
  marked ready; also `workflow_dispatch`, and the canary), not on every push (`docs/decisions.md`): `players` runs `tools/ci/live-players.sh`: the live engine test against real mpv, VLC (MPRIS),
  mpv-mpris and Celluloid on Linux, and mpv and IINA (set up by the real program first) on macOS. `plex` runs
  `tools/ci/live-plex.sh`: the live engine test against the official Plex Media Server image (unclaimed, no account)
  with a stand-in player that reports playback through `/:/timeline`. `jellyfin` runs `tools/ci/live-jellyfin.sh` for
  the official Jellyfin and Emby images (a matrix): the first-run wizard with a made-up user, a stand-in player (`/Sessions/Playing`),
  the real sign-in (password and Quick Connect) and the live engine test. `players-windows` runs `tools/ci/live-windows.sh`: real
  PotPlayer and Kodi from Chocolatey (PotPlayer without its stale package checksum), a generated video in each, the live
  engine test, and `TestLivePotPlayer` (the real PotPlayer sends the file's path). The weekly canary
  (`.github/workflows/canary.yml`, Mondays) runs all of `ci.yml` (`workflow_call`) against the newest players and
  server images, mpv, VLC and Celluloid from Flathub (`live-flathub.sh`: their newest releases), plus `TestLiveCatalogs` (`internal/artwork/live_test.go`, opt-in `MPCRPC_LIVE_CATALOGS=1`: one
  long-known title per keyless catalog, through the program's own search and episode code; it needs real titles, kept
  in that file); `tools/ci/canary-report.sh` opens an issue labelled `canary` on a failure and closes it once a run
  passes (only on the schedule or a by-hand run with `report` ticked; a pull request that changes the canary does a dry
  run). Read and fix a canary issue like a bug report. **This is the only
  place macOS and Linux run for real**, so read those logs after every change that touches them. A Linux or cloud
  environment can run almost everything else; the tray, the window, the registry and the Windows players need Windows.
- The download counter (`.github/workflows/downloads.yml`, every three hours and ten minutes after each release)
  updates the `stats` branch only; `master` is never written by a workflow. GitHub's counts were reset on 2026-10-18
  (every release file replaced by its identical bytes) and counting started then; after another reset, run the
  workflow by hand with `baseline` ticked. Its summary's `notCountedYet` is a download recorded in the ledger that
  GitHub has not counted yet (it lags by minutes); if it stays above 0, a recorded download never happened.
- **Actions are pinned by commit** (`uses: owner/action@<sha> # vX.Y.Z`); Dependabot updates the pins. A new action
  gets the same form, and a job gets only the `permissions` it needs (workflows are read-only by default).
- `tools/ci/test-noskip.sh` fails CI when a test skips that should run on that OS. A new test that is Linux-only,
  Windows-only or opt-in must be added to `tools/ci/allowed-skips.sh` (ci.yml and release.yml both use it). The live
  test scripts run through `must-pass.sh`, so a live test that skips fails its CI job.
- Engine tests use a fake Discord (Unix socket on Linux, named pipe on Windows) and fake players (`players_test.go`,
  `vlc_test.go`). Timing matters: wait for state with the `eventually()` helper rather than reading it once. They run
  in parallel and poll every 25 ms (`newTestEngine` sets `Options.MinPoll`; `ReclearEvery` and `RecheckAfter` shorten
  the other waits), so a test must never change a package variable; the one that has to (it shortens
  `discord.RequestTimeout`) stays sequential. Pure timer logic is tested on `testing/synctest`'s clock
  (`TestNudgesArePaced`).
- Tests tie the texts to the code: `TestEverySettingIsDocumented` (store: every setting has its row and default in
  `docs/config.md`), `TestPlayersAreNamedEverywhere` (cmd: the texts that list players name every one),
  `TestPageScriptParses` (assets: `node --check` on `ui.js`) and `tools/relnotes`'s test (the release text renders from
  the changelog). The CI scripts share `tools/ci/lib.sh` (the test video, `wait_socket`, `wait_bus`, `done_file`);
  `run-annotated.sh` fails a step whose script did not reach `done_file` when `PASSED_FILE` is set.
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

## Recipes
**New setting:** a field with a `json` tag in `core.Config` and its default in `DefaultConfig()`; a `Field` in
`store/schema.go` if it belongs in the window (int fields need Min/Max; `secret` for passwords and keys); a row in
`docs/config.md`; `diag.summarise` if the value is private (shown as "(set)"). Defaults that change behaviour for
existing users need the owner's decision and a line in the release notes.

**New player:** reader package with unit tests on recorded-style responses; wire into `fetchPlayer` *after* the
existing players; engine tests including player order; setup support in `winsys` if the player needs switching on;
a live run in `tools/ci/live-players.sh` if it exists on Linux or macOS (a Linux MPRIS player only needs its bus-name
part in `mpris.videoPlayers`, its process name in `winsys.unixPlayers`, and the lists below);
then every place that lists players: `ui.html` and `ui.js` (welcome card, "No player is answering" card, "Nothing playing" hint,
Privacy "What else leaves this PC"; the macOS and Linux texts are in `OS_TEXT`), README (tagline, What you get, Getting started, the Players table, Help), `docs/how-it-works.md` (Players in detail), `docs/troubleshooting.md`, `docs/development.md` (Where things are),
`docs/config.md`, the help texts in `store/schema.go` (`port`, `mpris`, `iinaPipe`), `SECURITY.md`, `.github/ISSUE_TEMPLATE/bug_report.yml`, the exe description in `tools/mkrsrc/main.go`,
the header comment in `main.go`, the engine's "Looking for a player on ..." log line, `diag` and the repo description/topics.

**New catalog service:** an entry in `core.Sources` (id, label, use, host: it gets its switch on the Privacy tab), a
`<Name>Base` field with its default in `core.Config` (tests point it at a fake), its id in the default
`artworkSources` and/or `episodeSources` (a default change needs the owner), the `Provider` in
`artwork/providers.go` and/or the `epProvider` in `artwork/episodes.go`, its line in `blockedHostFilter`
(`artwork/disabled.go`), a subtest in `internal/artwork/live_test.go` (the canary), its row in `SECURITY.md`, the
credit its terms ask for (Help tab credits, `THIRD-PARTY-NOTICES.md`) and `docs/config.md`.

**New outside address:** any host the code names (a service, a link, an image) goes in `core.Hosts` (or
`core.Sources`) and in `SECURITY.md` > Where it connects; `TestEveryHostIsListed` scans the code and the page and
fails otherwise.

**UI text:** a recorded store test pins the label "MPC-HC web interface port"; change only its help text. The window
follows "less is more": short help texts, one term per thing, rarely used actions under Advanced > Maintenance. Keep the README,
`docs/how-it-works.md`, `docs/config.md`, schema help text and release notes in sync with any behaviour change. The
README is for people: short, scannable, one emoji per section heading; details go in `docs/`.

## Releases
One branch and one PR per release: the change, the version bump and the release text together.
1. Branch; change; in the same branch bump `var version` in `cmd/mpcvibedrpc/main.go` (nothing else carries the
   version: the workflows and smoke tests read it; docs use `X.Y.Z`), add the version's entry at the top of
   `internal/assets/changelog.json` (`TestChangelog` fails until it matches `main.go`). That entry is also the GitHub
   release's text: `release.yml` renders it with `tools/relnotes` (heading, the entry's lines, the "Get it" footer). A
   release that needs more words than one-liners (a security advisory) gets them in `.github/release-advisory.md`,
   which the tool inserts after the lines; delete that file in the pull request after the release that needed it.
   Screenshots (`docs/images/window-*.jpg`) are retaken only when the
   part of the window they show changed, as `docs/development.md` > Screenshots describes (made-up titles only).
2. Locally: `go vet ./...` (also `GOOS=linux`, `GOOS=darwin`), gofmt on LF copies, `go test -count=1 ./...`, and a
   scan of the diff and new files for personal data (the owner's name, email, local paths) before the first push.
   Then push, `gh pr create`, wait for CI (`gh run watch`) and read the logs to confirm the expected steps ran.
3. With the owner's go-ahead: `gh pr merge <n> --rebase --delete-branch` (GitHub refuses to rebase a branch that holds
   merge commits, such as merged agent branches: use `--merge` then, or rebase the branch locally first), then
   `git tag -a vX.Y.Z -m "MPCvibedRPC X.Y.Z"`
   with the Claude identity on the merged commit, `git push origin vX.Y.Z`. The release workflow checks the tag
   against `main.go`, tests, builds and smoke-tests every program on its own system (the Windows smoke test covers DPI
   awareness, version details, resting and a self-update against a fake server), writes the `.sha256` files, attests,
   publishes it as a **draft**, then `reproduce` rebuilds the exe and both Linux programs from the tag on another
   runner (they must match the release byte for byte), and then runs `verify` (`tools/ci/verify-release.ps1`): on the draft the ten assets,
   every checksum and build record and the exe's version; only if they pass is the release made public (and latest),
   followed by the checks that need it public: that it is the latest release, and a real self-update from the previous
   release. A release whose draft checks fail stays a draft (nobody's updater sees it): fix, then delete the draft and
   the tag, or publish it by hand once checked.
4. Read the `verify` job's log (`gh run view --log`) and report. Nothing needs re-running locally after a release.
5. A release with new features also updates the repository to match: its description and topics
   (`gh repo edit --description ... --add-topic ...`), so they name every player and system the program supports.
   The owner gave a standing go-ahead for this.

Merging, tagging and publishing are outward-facing: do them only with the owner's go-ahead.

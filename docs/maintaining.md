# Maintaining

How MPCvibedRPC is released, how its screenshots are taken, and how to run the program and real players without
touching an installed copy. Building and testing are in [development.md](development.md); the rules for everyone are in
[CONTRIBUTING.md](../CONTRIBUTING.md).

## Releasing
One branch and one pull request per release: the change, the version bump and the release text together. Merging,
tagging and publishing are outward-facing: they need the owner's go-ahead. Releases are bundles: a version collects
several changes, and only a fix for something broken, or a security fix, ships alone ([why](decisions.md#releases)).

Steps 1-2 are done where the change was made. Steps 3-4 (from the merge to the verify log) follow a written checklist
and may be handed to the `releaser` agent (Claude Code) with the pull request number, the version and the owner's
go-ahead in their words.

1. **Branch and change.** In the same branch bump `var version` in `cmd/mpcvibedrpc/main.go` (nothing else carries
   the version: the workflows and smoke tests read it through `tools/ci/version.sh`; docs use `X.Y.Z`) and add the
   version's entry at the top of `internal/assets/changelog.json` (the window shows it once after the update;
   `TestChangelog` fails until the newest entry matches `main.go`; a line has at most 130 characters). That entry is
   also the GitHub release's text: `release.yml` renders it with `tools/relnotes` (heading, the entry's lines, the
   "Get it" footer). A release that needs more words than one-liners (a security advisory) gets them in
   `.github/release-advisory.md`, which the tool inserts after the lines; delete that file in the pull request after
   the release that needed it. Screenshots are retaken only when the part of the window they show changed (see
   [Screenshots](#screenshots)).
2. **Check locally.** `go vet ./...` (also with `GOOS=linux` and `GOOS=darwin`), gofmt on LF copies, `go test
   -count=1 ./...`, `govulncheck` (a Go advisory published the same day would fail the pull request), and a scan of
   the diff and new files for personal data before the first push. Then push, `gh pr create --base master`, wait for
   CI (`gh run watch`) and read the logs to confirm the expected steps ran. The live jobs ran when the pull request was
   opened: if later commits touch the engine or a player reader, run `ci.yml` by hand (`workflow_dispatch`) on the
   branch before the release.
3. **Merge and tag** (with the owner's go-ahead). Wait for the pull request's CI first: the repository does not
   require passing checks, so `gh pr merge --auto` merges at once. `gh pr merge <n> --rebase --delete-branch` (GitHub
   refuses to rebase a branch that holds merge commits, such as merged agent branches: use `--merge` then, or rebase
   the branch locally first), then `git tag -a vX.Y.Z -m "MPCvibedRPC X.Y.Z"` on the merged commit and
   `git push origin vX.Y.Z`. The release workflow (`.github/workflows/release.yml`) checks the tag against `main.go`,
   tests, builds the programs on Windows, Linux and macOS runners and smoke-tests each on its own system (Windows:
   tray, Discord pipe, resting while Discord is closed, DPI awareness, version details, autostart, a self-update
   against a fake server; macOS and Linux: the API, login item, player setup, a second start, and on macOS opening
   the app like the Finder does), writes the `.sha256` files, attests, and publishes a **draft**. `reproduce`
   rebuilds the exe and both Linux programs from the tag on another runner at the same time (`tools/ci/reproduce.sh`;
   no job waits for it but `verify`). `verify` first checks that their checksums equal the release's (byte for byte),
   then runs `tools/ci/verify-release.ps1` on the draft: the ten assets, every checksum and build record, the exe's
   version. Only if they pass is the release made public (and latest), followed by the checks that need it public: that
   it is the latest release, and a real self-update from the previous release. A release whose draft checks fail stays
   a draft (nobody's updater sees it): fix, then delete the draft and the tag, or publish it by hand once checked.
4. **Read the `verify` job's log** (`gh run view --log`) and report. Nothing needs re-running locally after a release.
5. **Repository details.** A release with new features also updates the repository description and topics
   (`gh repo edit --description ... --add-topic ...`) so they name every player and system the program supports (the
   owner gave a standing go-ahead for this).

Pitfalls:
- The default branch is `master`, not `main`. Never name a branch like a tag (`v0.9.20`): `git push origin v0.9.20`
  becomes ambiguous; use `release-X.Y.Z`.
- Save a failed job's log before `gh run rerun` (the rerun erases it). A release run whose macOS job is cancelled
  with "not acquired by Runner" built nothing and left no draft: `gh run rerun <id> --failed` is the remedy.
- Versions stay `0.9.x`, three-part: the updater compares three numbers.

### Downloading release files
Release files are only downloaded through `tools/ci/dev-download.sh`, never with a bare `gh release download`, a
browser or curl. It records the download in a ledger on the `stats` branch, so the download counter
(`.github/workflows/downloads.yml`, see [development.md](development.md#ci)) leaves it out:

    tools/ci/dev-download.sh download BY TAG DIR [PATTERN]     # downloads into DIR (new or empty), then records it
    tools/ci/dev-download.sh record BY TAG/FILE[=N]            # for a download made another way, e.g. a test copy updating itself

`BY` says who downloaded ("release check v0.9.39 draft", "Claude agent"); it needs `gh` signed in with write access.
To check a release without downloading anything, GitHub lists each file's sha256 as `digest`
(`gh api repos/OWNER/REPO/releases/tags/vX.Y.Z`). How users check a download: [SECURITY.md](../SECURITY.md#checking-a-download).

## Screenshots
The README's pictures (`docs/images/window-main.jpg`, `window-privacy.jpg`; `window-plex.jpg` is used by
[troubleshooting](troubleshooting.md)) are taken again when the part of the window they show changes:

1. Build the program and start it with a throwaway data folder (`MPCRPC_HOME`), with presence off and
   `{"app": {"seenVersion": "<version>"}}` in its `config.json` so the What's new box stays closed.
2. Open its settings page (the address and token are in `ipc.json` in that folder) in a browser at 660 x 760 pixels,
   dark mode (a high-DPI screen gives sharper pictures). Edge's `--window-size` is the outer window: about 35 px go
   to the title bar and border.
3. For `window-main.jpg` paste `tools/screenshots/demo.js` into the page's console: it shows a made-up
   "Sample Show" playing and sample stats, and nothing is sent anywhere. Take the General tab. For
   `window-privacy.jpg` (Privacy tab, scrolled to "Services that may see your titles") and `window-plex.jpg`
   (Players tab, scrolled to Plex) reload the page first, so presence shows as off.
4. Save them 800 pixels wide (800 x 921). Only made-up titles; nothing from a real library or account.

## Previewing the README
`gh api markdown -f mode=markdown -f text="$(cat README.md)"` renders it the way GitHub does. `mode=gfm` misleads: it
renders differently from the repository page.

## Running the program safely
Never touch a real setup while testing: no contact with the real Discord, no presence for a real player, no change to
a real player's settings, the registry or `%APPDATA%`/`%LOCALAPPDATA%` without the owner's explicit OK. Use throwaway
copies and the fakes below.

- **A throwaway copy.** `MPCRPC_HOME=<scratch folder>` gives a copy its own data folder **and its own autostart value
  name**, so it can never touch an installed copy. A config such as
  `{"app":{"openWindow":false,"startPresence":false,"checkUpdates":false,"welcomeSeen":true}}` keeps it quiet. The
  `--home <folder>` flag does the same.
- **Keep `checkUpdates` off in test copies.** With it on, a copy without a window installs what its update server
  offers ten seconds after its start and shows a real operating-system notification. To try the update dialog, start
  with it off, open the page, then switch it on there (that looks at once; or trigger a check through `api/update-check`).
- **The page.** The API port and token are in `<MPCRPC_HOME>/ipc.json`; the page is
  `http://127.0.0.1:<port>/?t=<token>` (the program itself opens its window with a one-time code, `?c=`, so the token
  is not on the browser's command line; the page keeps the token in `sessionStorage` and takes it out of its address).
  To test window states without Discord, override `window.fetch` for `/api/status` (or `/api/plex*`, `/api/stremio*`)
  in the page. Window script changes need the real page opened: unit tests cover the server side only.
- **Presence.** `api/start` with nothing playing is safe: Discord is only contacted once something plays ("standby").
  To keep a real player from being picked up, point `port` at an unused port and leave `mpvPipe` and `vlcPassword`
  empty.
- **Resting.** A throwaway copy rests whenever Discord is closed and its window is closed (no player is asked then). To
  test players with a closed Discord, keep its window open.
- **Update checks.** `MPCRPC_UPDATE_API=http://127.0.0.1:<port>` with `tools/ci/fake-http.ps1` (on CI; on a Windows PC
  its HttpListener may be refused without admin rights, so a small Go server in a scratch folder serves the same three
  answers there).
- **Other variables.** `MPCRPC_BROWSER=none` opens no window (tests, CI; any other value names the browser to show the
  window in, one that understands `--app`); `MPCRPC_FOREGROUND=1` keeps a macOS copy from handing over to a detached
  one.
- **A live engine test against a real player**, with the test's own fake Discord (opt-in):
  `MPCRPC_LIVE_PLAYER=mpv MPCRPC_LIVE_TITLE="Sample Movie" go test -count=1 -run TestLivePlayer -v ./internal/engine`
  (also `MPCRPC_LIVE_PORT`, `MPCRPC_LIVE_VLC_PASSWORD`, `MPCRPC_LIVE_VLC_PORT`, `MPCRPC_LIVE_MPV_PIPE`,
  `MPCRPC_LIVE_IINA_PIPE`, `MPCRPC_LIVE_PLEX_ADDRESS`, `MPCRPC_LIVE_PLEX_TOKEN`, `MPCRPC_LIVE_JELLYFIN_ADDRESS`,
  `_USER`, `_PASSWORD`; `MPCRPC_LIVE_PLAYER` is the name shown: mpv, IINA, VLC, Celluloid, Plex, ...).
- **Portable players.** Use portable builds in a scratch folder (MPC-QT: `portable.txt`; MPC-BE: an ini next to the exe;
  mpv: `portable_config\`; VLC: a `portable\` folder next to `vlc.exe`, started with `--no-qt-privacy-ask`, otherwise a
  first-run dialog keeps it from closing normally), verify their published checksums, and check `%APPDATA%`,
  `%LOCALAPPDATA%` and `HKCU\Software` afterwards. Test videos can be made with mpv's encoder from `lavfi` test
  sources, with generic names.
- **Accounts.** Never sign in to a real Plex, Stremio, Jellyfin or Emby account, and never call plex.tv's sign-in,
  link.stremio.com / api.strem.io or a real server's sign-in, from tests or manual checks without the owner's OK. Every
  reader has fakes: Plex in `internal/plex`, `cmd/mpcvibedrpc/plex_test.go` and `internal/engine/plex_test.go`; Stremio
  in `internal/stremio`, `cmd/mpcvibedrpc/stremio_test.go` and `internal/engine/rest_test.go`; Jellyfin / Emby in
  `internal/jellyfin`, `cmd/mpcvibedrpc/jellyfin_test.go` and `internal/engine/jellyfin_test.go`. A small fake server on
  127.0.0.1 can drive the window's sign-in in a throwaway copy (keep presence off, so Discord is never contacted).
- **Leave nothing running.** Quit test copies, stop only processes you started, delete scratch tests before a full
  test run, and check the real data folders afterwards.

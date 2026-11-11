# CLAUDE.md

Guidance for Claude Code sessions working on this repository. Keep it current: update it (and the skills in
`.claude/skills/`, the rules in `.claude/rules/`) in the same pull request whenever a change makes something there
wrong (a new player, setting, CI step, release step or rule).

## What this is
MPCvibedRPC is a small Windows tray app (Go, standard library only, one exe of under 10 MB, nothing to install) that shows what
a video player is playing as Discord Rich Presence: "Watching <title>" with a progress bar, cover art, episode titles,
ratings, and each player's logo. Supported players: **MPC-HC, MPC-BE, MPC-QT, mpv, VLC and PotPlayer**, **Kodi**, and **Plex**, **Stremio**, **Jellyfin** and
**Emby** on every system (any device the user plays on, read from their Plex server after "Sign in to Plex", from
their Stremio library after "Sign in to Stremio", or from their own Jellyfin or Emby server; all sign-ins are on the
Accounts tab). It talks to the Discord desktop app over its local IPC pipe; the settings window is a local web page
shown in an Edge/Chrome `--app=` window (tabs General, Accounts, Look, Titles & art, Privacy, Advanced, Help; each group of settings is one card; updates are under General).
The tray (Windows) and menu bar (macOS) menus show a status line, then Open settings, Show on Discord / Stop showing
on Discord, Hide what I'm watching (`hideTitle`, ticked), Show the title on my streams (`streamTitle`, ticked) and Quit. The window says "Show on Discord" / "status", not
"presence". How updates are looked for, offered and installed: `docs/development.md` > Updates.

It also runs on **macOS** (a universal `.app` with no Dock icon; IINA, mpv, VLC, MPC-QT) and **Linux** (a plain binary;
MPRIS players such as VLC, Celluloid, Haruna, plus mpv). macOS has a menu bar icon and shows the settings in a WebKit
window of its own (`internal/winsys/macapp_darwin.go`: Cocoa through cgo, so the macOS build is made on a Mac with
cgo; without cgo it falls back to the browser and no icon). Linux has no tray icon: running the program again shows
the window, and Quit is in the window. Windows is the main system and its behaviour must not change for the others.
The owner has no Mac or Linux machine: those systems are tested on GitHub's runners (see CI), and once by a
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
  owner's explicit OK. Use the fakes and throwaway data folders in the `throwaway-copy` skill.
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
  services, from tests or by hand without the owner's OK; every reader has fakes (see the `throwaway-copy` skill).
- The engine's resting, clearing and state rules are in `.claude/rules/engine.md` (loaded with `internal/engine`).
- The default Discord application (`clientId`) was registered by the upstream project; every install depends on it.

## Build and test
```bash
go vet ./...
go test -count=1 ./...
go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=X.Y.Z" -o dist/MPCvibedRPC.exe ./cmd/mpcvibedrpc
```
- The workflows carry no version: `tools/ci/version.sh` reads it from `cmd/mpcvibedrpc/main.go` (VERSION, and NEXT for
  the smoke test's update) and, on a tag, fails when the tag does not match.
- CI (`.github/workflows/ci.yml`; the jobs, the lint gates, the live jobs, the canary and the download count are
  described in `docs/development.md` > CI): **the live jobs run once per pull request**, and **CI is the only
  place macOS and Linux run for real**, so read those logs after every change that touches them.
- Tests tie the texts to the code: `TestEverySettingIsDocumented` (store: every setting has its row and default in
  `docs/config.md`), `TestPlayersAreNamedEverywhere` (cmd: the texts that list players name every one),
  `TestPageScriptParses` (assets: `node --check` on `ui.js`) and `tools/relnotes`'s test (the release text renders from
  the changelog).

### Working on Windows
- Working-copy files are CRLF (git `autocrlf`; the index is LF), except `internal/jsre/*_table.go`. Some editing tools
  silently rewrite a CRLF file as LF; convert it back before committing so diffs stay small.
- `gofmt -l` flags every CRLF file on Windows. Check formatting on LF copies (or rely on the Linux CI gate), and never
  `gofmt -w` a CRLF file in place.
- Shell scripts that rewrite many files can damage them silently; read the diff before committing.

**UI text:** a recorded store test pins the label "MPC-HC web interface port"; change only its help text. The window
follows "less is more": short help texts, one term per thing, rarely used actions under Advanced > Maintenance. Keep the README,
`docs/how-it-works.md`, `docs/config.md`, schema help text and release notes in sync with any behaviour change. The
README is for people: short, scannable, one emoji per section heading; details go in `docs/`.

## Where the procedures are
Skills (`.claude/skills/<name>/SKILL.md`; they load when used, or open the file by hand):
- `release`: the release steps, from the version bump to the verify job.
- `throwaway-copy`: running the real program, a live engine test or a real player without touching a real setup.
- `new-player`, `new-setting`, `new-catalog`, `new-host`: the checklists for adding one.

Rules (`.claude/rules/`, loaded when a matching file is read or edited): `engine.md` (internal/engine),
`capture.md` (the stream title), `tests.md` (writing tests), `ci.md` (workflows and CI scripts).

## Releases
One branch and one PR per release: the change, the version bump and the release text together.
The steps are in the `release` skill; the part from the merge to the verify job goes to the `releaser` agent.

Merging, tagging and publishing are outward-facing: do them only with the owner's go-ahead.

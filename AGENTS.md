# AGENTS.md

Guidance for any AI coding assistant working on this repository.

## What this is
MPRP is a small Windows tray app (Go, standard library only, one exe under 10 MB, nothing to install) that
shows what a video player is playing as a Discord status: "Watching <title>" with a progress bar, cover art, episode
titles and the player's logo. It reads the players listed in the [README](README.md#-players), talks to the Discord
desktop app over its local IPC pipe, and shows its settings in a local web page. It also runs on macOS (menu bar) and
Linux. Windows is the main system and its behaviour must not change for the others.

## Build and test
```bash
go vet ./...
go test -count=1 ./...
go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=X.Y.Z" -o dist/MPRP.exe ./cmd/mprp
```
CI runs the rest (the race detector, the lint gates, macOS and Linux for real): see
[development.md > CI](docs/development.md#ci).

## Read first
1. [CONTRIBUTING.md](CONTRIBUTING.md): the [ground rules](CONTRIBUTING.md#ground-rules) for everyone and
   [working on Windows](CONTRIBUTING.md#working-on-windows). They apply to you in full.
2. [docs/development.md](docs/development.md): packages, how players are read, the engine's state, testing, CI, and
   the checklists for adding a player, setting, catalog or host. Before you run the program or a real player, read
   [Running the program safely](docs/development.md#running-the-program-safely): a *throwaway copy* is one started
   with a data folder of its own (`MPRP_HOME`), so it cannot touch an installed copy.
3. [docs/maintaining.md](docs/maintaining.md): releasing, screenshots, downloading release files.
4. [docs/roadmap.md](docs/roadmap.md) before proposing a feature, a service or a distribution change (don't propose a
   listed non-goal without saying so), and [docs/decisions.md](docs/decisions.md) before changing listed behaviour.
5. [docs/ai.md > Working with AI agents](docs/ai.md#working-with-ai-agents-on-it) if you orchestrate helper agents:
   what to hand off, how to brief, and how to check what comes back.

## Picking up work
The work to do and how to continue what someone else started are in [CONTRIBUTING.md > Picking up work](CONTRIBUTING.md#picking-up-work): the roadmap's [Next up](docs/roadmap.md#next-up) lists work anyone can take; whatever waits on a decision is under its [Open decisions](docs/roadmap.md#open-decisions-the-maintainers) and needs the maintainer. Nothing the repository needs is kept elsewhere.

## Your first change
Read the named sections, make the change, then run the tests named with it (and always `go vet ./...` and
`go test -count=1 ./...`).
- **A title or file-name parsing fix:** [Titles and covers](docs/how-it-works.md#titles-and-covers) and `core` in
  [Packages](docs/development.md#packages-internal); run `go test ./internal/core` (the golden data must still pass).
- **A window or UI fix:** [The window and the menu](docs/how-it-works.md#the-window-and-the-menu) and the
  `internal/assets` rows in [Packages](docs/development.md#packages-internal) and the window's look in
  [design/settings-window.md](docs/design/settings-window.md); run `go test ./internal/assets ./cmd/mprp`
  (`TestPageScriptParses` and `TestPageBehaviour` need Node.js).
- **A new player:** [A new player](docs/development.md#a-new-player) and
  [How players are read](docs/development.md#how-players-are-read); run `go test ./internal/engine ./cmd/mprp`.
- **A new setting:** [A new setting](docs/development.md#a-new-setting); run `go test ./internal/store ./cmd/mprp`
  (`TestEverySettingIsDocumented` fails until `docs/config.md` has its row).

## Invariants: never break these
- **Player order.** The first player that plays a video wins, then music, then paused, then the first that answers
  ([how it works](docs/how-it-works.md#players-in-detail); code order:
  [development.md](docs/development.md#how-players-are-read)).
- **MPC-HC behaves as before** whenever anything is added: new players are asked after the existing ones
  ([decision](docs/decisions.md#behaviour)).
- **MPRIS reads only the allowlist** in `internal/mpris`: music players and browsers must never show as "Watching".
- **Readers are read-only**, and tests never use real accounts or a real setup ([ground rules](CONTRIBUTING.md#ground-rules)).
- **The default Discord application (`clientId`, `core.DefaultClientID`) is the project's own**, named MPRP; every
  install depends on it. A saved former default (`core.FormerClientID`) reads as it: keep that carry-over.
- **Player logo addresses are fixed.** Every install fetches the logos from `docs/players/<name>.png` on the `master`
  branch (`PlayerIconBase` in `internal/core/icons.go`). Never move, rename or delete those files or rename `master`;
  adding a logo is fine.
- **Docs in the same pull request.** A change that makes the README, `docs/`, schema help text or release notes wrong
  fixes them in that change. New behaviour for existing users is off by default.
- **The tests that tie texts to code** must keep passing ([the list](docs/development.md#testing)).
- **UI text.** The window says "Show on Discord" and "status", never "presence"; use the same words in user docs. A
  recorded store test pins the label "MPC-HC web interface port": change only its help text. The window follows "less
  is more": short help texts, one term per thing, rarely used actions under Advanced > Maintenance. The README is for
  people: short, scannable, one emoji per section heading; details go in `docs/`.

## Other facts
- The program is AI-generated; the README says so once, near the top, and that is enough. Never present the code as
  written by the repository maintainer. The idea comes from angeloanan/MPC-DiscordRPC (MIT), credited in the README,
  `LICENSE` and `THIRD-PARTY-NOTICES.md`.
- Versions are `0.9.x`; the current one is in `cmd/mprp/main.go`. Changelog lines are short, one emoji each.
- Merging, tagging and publishing are outward-facing: do them only with the maintainer's go-ahead
  ([maintaining.md](docs/maintaining.md#rules-for-maintainers)).

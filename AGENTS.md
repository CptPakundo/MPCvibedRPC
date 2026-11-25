# AGENTS.md

Guidance for any AI coding assistant working on this repository.

## What this is
MPCvibedRPC is a small Windows tray app (Go, standard library only, one exe under 10 MB, nothing to install) that
shows what a video player is playing as Discord Rich Presence: "Watching <title>" with a progress bar, cover art,
episode titles and the player's logo. It reads MPC-HC, MPC-BE, MPC-QT, mpv, VLC, PotPlayer, IINA, Kodi, Plex,
Stremio, Jellyfin and Emby, talks to the Discord desktop app over its local IPC pipe, and shows its settings in a
local web page. It also runs on macOS (menu bar) and Linux. Windows is the main system and its behaviour must not
change for the others.

## Read first
1. [CONTRIBUTING.md](CONTRIBUTING.md): the rules for everyone (privacy, generic names, frozen golden data, never touch
   a real setup, tested-for-real reports, Windows line endings).
2. [docs/development.md](docs/development.md): packages, how players are read, the engine's state, building, testing,
   CI, and the checklists for adding a player, setting, catalog or host.
3. [docs/maintaining.md](docs/maintaining.md): releasing, screenshots, throwaway copies and fakes.
4. [docs/roadmap.md](docs/roadmap.md) before proposing a feature, a service or a distribution change (don't propose a
   listed non-goal without saying so), and [docs/decisions.md](docs/decisions.md) before changing listed behaviour.

## Invariants: never break these
- **Player order.** The first player that plays a video wins, then music, then paused, then the first that answers
  ([how it works](docs/how-it-works.md#players-in-detail); code order: [development.md](docs/development.md#how-players-are-read)).
- **MPC-HC behaves as before** whenever anything is added: new players are asked after the existing ones
  ([decision](docs/decisions.md#behaviour)).
- **MPRIS reads only the allowlist** in `internal/mpris`: music players and browsers must never show as "Watching".
- **Readers are read-only.** Never sign in to a real Plex, Stremio, Jellyfin or Emby account, or call their sign-in
  services, from tests or by hand without the owner's OK.
- **The default Discord application (`clientId`) belongs to the upstream project**; every install depends on it.
- **Docs in the same pull request.** A change that makes the README, `docs/`, schema help text or release notes wrong
  fixes them in that change. New behaviour for existing users is off by default.
- **Tests that tie texts to code** must keep passing: `TestEverySettingIsDocumented` (every setting has its row and
  default in `docs/config.md`), `TestPlayersAreNamedEverywhere` (every text that lists players names them all),
  `TestPageScriptParses` (`node --check` on `ui.js`), `TestEveryHostIsListed` (outside addresses are in `core.Hosts`
  and `SECURITY.md`) and the `tools/relnotes` test (the release text renders from `internal/assets/changelog.json`).
- **UI text.** A recorded store test pins the label "MPC-HC web interface port": change only its help text. The window
  follows "less is more": short help texts, one term per thing, rarely used actions under Advanced > Maintenance. The
  README is for people: short, scannable, one emoji per section heading; details go in `docs/`.

## Other facts
- The program is AI-generated; the README says so once, near the top, and that is enough. Never present the code as
  written by the repository owner. The idea comes from angeloanan/MPC-DiscordRPC (MIT), credited in the README,
  `LICENSE` and `THIRD-PARTY-NOTICES.md`.
- Versions are `0.9.x`; the current one is in `cmd/mpcvibedrpc/main.go`. Changelog lines are short, one emoji each.
- Merging, tagging and publishing are outward-facing: do them only with the owner's go-ahead
  ([releasing](docs/maintaining.md#releasing)).
- Commit with a neutral identity, never anyone's personal email.

---
name: new-player
description: Checklist for adding a new video player to MPCvibedRPC - reader package, order in fetchPlayer, engine tests, setup, the CI live run and every place that lists the players.
---
**New player:** reader package with unit tests on recorded-style responses; wire into `fetchPlayer` *after* the
existing players; engine tests including player order; setup support in `winsys` if the player needs switching on;
a live run in `tools/ci/live-players.sh` if it exists on Linux or macOS (a Linux MPRIS player only needs its bus-name
part in `mpris.videoPlayers`, its process name in `winsys.unixPlayers`, and the lists below);
then every place that lists players: `ui.html` and `ui.js` (welcome card, "No player is answering" card, "Nothing playing" hint,
Privacy "What else leaves this PC"; the macOS and Linux texts are in `OS_TEXT`), README (tagline, What you get, Getting started, the Players table, Help), `docs/how-it-works.md` (Players in detail), `docs/troubleshooting.md`, `docs/development.md` (Where things are),
`docs/config.md`, the help texts in `store/schema.go` (`port`, `mpris`, `iinaPipe`), `SECURITY.md`, `.github/ISSUE_TEMPLATE/bug_report.yml`, the exe description in `tools/mkrsrc/main.go`,
the header comment in `main.go`, the engine's "Looking for a player on ..." log line, `diag` and the repo description/topics.

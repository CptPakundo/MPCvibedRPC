# Contributing

The program is AI-generated ("vibe coded"): the code, tests and docs were written with Claude from the owner's prompts
and testing. Bug reports, reviews and pull requests with tests are welcome all the same.

## Quick start
1. Install [Go](https://go.dev/dl/) at the version in `go.mod` (the `go` line) and clone the repository. The default
   branch is `master`.
2. Run the checks: `go vet ./...` and `go test -count=1 ./...`.
3. Build: `go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=0.0.0" -o dist/MPCvibedRPC.exe ./cmd/mpcvibedrpc`
   (other systems: [development.md](docs/development.md#building-and-running)).
4. Before you start the program you built, read [Running the program safely](docs/development.md#running-the-program-safely):
   a copy with its own data folder (`MPCRPC_HOME`) leaves your installed copy, Discord and players alone.

Optional tools:
- **Node.js**: `TestPageScriptParses` checks the settings page's script with `node --check`; without Node.js it skips.
- **The lint tools** CI runs, at the versions pinned in `.github/workflows/ci.yml`:

      go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
      go install honnef.co/go/tools/cmd/staticcheck@v0.8.2
      go install golang.org/x/tools/cmd/deadcode@v0.52.0

  Run them as CI does: `govulncheck ./...`, `staticcheck -checks inherit,-ST1005 ./...` and `deadcode -test ./...`.

## How to help
- **Bugs:** use the [issue templates](https://github.com/CptPakundo/MPCvibedRPC/issues/new/choose). On the **Help** tab,
  **Copy diagnostics** gives a report to attach; what it contains is in
  [Troubleshooting](docs/troubleshooting.md#reporting-a-problem). Look through it before you post it.
- **Security problems:** report them privately, as described in [SECURITY.md](SECURITY.md), not in a public issue.
- **Pull requests:** keep them focused, target `master`, and update the README and `docs/` when behaviour changes.
  Check [docs/decisions.md](docs/decisions.md) and the roadmap's [Not goals](docs/roadmap.md#not-goals) first: a change
  that goes against one should say why.
- **AI-assisted pull requests** are welcome, as the program itself is made that way ([how](docs/ai.md)). Say which
  parts an AI wrote, and what you tested for real (not only against fakes); add tests for the change. A pull request
  you have not run yourself will most likely be closed. AI tools: start with [AGENTS.md](AGENTS.md).

## Ground rules
- **Privacy first.** No personal data in commits, tags, code, docs, tests, screenshots or release files: no email
  addresses, local paths or user names, machine details, tokens. Commit and tag with a neutral identity, never a
  personal email (an annotated tag stores the tagger's email too). Scan the diff before the first push.
- **Generic names.** UI text, docs, code comments and new tests use made-up titles ("Sample Show S01E02", "Sample Movie
  (2020)"), never real shows, films or episode numbers. Exempt: the golden data (below), and real behaviour about a
  specific source (the Bulbapedia episode lists).
- **Golden data is frozen.** The golden data are recorded reference outputs in `internal/*/testdata/*.gz` that the
  parsers and lookups are checked against. They were recorded once and their generators no longer exist. Never
  regenerate or "fix" them; make the code match them.
- **Never touch a real setup while testing.** Tests and manual checks must not contact the real Discord, show a status
  for a real player, or change a real player's settings, the registry or `%APPDATA%`/`%LOCALAPPDATA%`. Use the fakes
  and throwaway data folders in [Running the program safely](docs/development.md#running-the-program-safely).
- **The readers are read-only; real accounts are off-limits.** Never sign in to a real Plex, Stremio, Jellyfin or Emby
  account, or call their sign-in services, from tests or by hand, without the owner's OK. Every reader has fakes.
- **No regressions, no churn.** Weigh pros and cons; don't change working behaviour for its own sake. New behaviour that
  affects existing users stays off or unchanged by default unless the owner decides otherwise.
- **Be exact about what was tested.** Say what ran for real, what ran against fakes, and what was not tested.
- **Docs present the program as it is now**: no history of earlier versions or former names (the upstream credit to
  angeloanan/MPC-DiscordRPC stays in the README, `LICENSE` and `THIRD-PARTY-NOTICES.md`).

Maintainers have a few more rules (outward-facing steps, release-file downloads): [maintaining.md](docs/maintaining.md#rules-for-maintainers).

## Working on Windows
- Working-copy files are CRLF (git `autocrlf`; the index is LF), except `internal/jsre/*_table.go`. Some editing tools
  silently rewrite a CRLF file as LF: convert it back before committing so diffs stay small.
- `gofmt -l` flags every CRLF file on Windows. Check formatting on LF copies (or rely on the Linux CI gate), and never
  `gofmt -w` a CRLF file in place.
- Scripts that rewrite many files can damage them silently: read the diff before committing.

## Where to find what
| You want | Read |
|---|---|
| Rules for AI coding tools | [AGENTS.md](AGENTS.md) |
| Building, running safely, the package map, how players are read, the engine, testing, CI, adding a player, setting, catalog or host | [docs/development.md](docs/development.md) |
| Releasing, screenshots, downloading release files | [docs/maintaining.md](docs/maintaining.md) |
| Why the program behaves as it does; what it leaves out; what is open | [docs/decisions.md](docs/decisions.md), [docs/roadmap.md](docs/roadmap.md) |
| How AI is used and how the work is checked | [docs/ai.md](docs/ai.md) |
| How it works for users; every setting; troubleshooting | [docs/how-it-works.md](docs/how-it-works.md), [docs/config.md](docs/config.md), [docs/troubleshooting.md](docs/troubleshooting.md) |
| Every address the program contacts; checking a download | [SECURITY.md](SECURITY.md) |

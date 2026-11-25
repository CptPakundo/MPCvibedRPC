# Contributing

The program is AI-generated ("vibe coded"): the code, tests and docs were written with an AI assistant (Claude) from the maintainer's prompts
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

## Picking up work
- **Where the work is:** the roadmap's [Next up](docs/roadmap.md#next-up) (work anyone can take without a decision), its
  [Open decisions](docs/roadmap.md#open-decisions-the-maintainers) (they need the maintainer: ask before building),
  [Not yet tried for real](docs/development.md#not-yet-tried-for-real) (checks that need a setup you may have) and the
  open pull requests.
- **Work in progress is a branch and a pull request** (a draft is fine). Its description says what was done, what was
  tested and how (for real, against fakes, not at all), and what is next, in order, so that anyone can continue it
  without asking. Update the description as the work moves.
- Keep to one task per branch. If you stop, say so in the pull request.

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
  account, or call their sign-in services, from tests or by hand, without the maintainer's OK. Every reader has fakes.
- **No regressions, no churn.** Weigh pros and cons; don't change working behaviour for its own sake. New behaviour that
  affects existing users stays off or unchanged by default unless the maintainer decides otherwise.
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
- Git Bash's `sed -i` drops every carriage return of a CRLF file, even when its pattern matches nothing. Use `perl -pi`
  (it keeps line ends), or `tr -d '\r' < f | sed ... | sed 's/$/\r/'`, then check with `file <name>`.
- Never write emoji or other non-ASCII text through perl escape sequences: perl then re-encodes every UTF-8 character
  in the file. Edit such text in an editor (or a tool that writes UTF-8 as it is) and read the diff.
- Multi-pattern perl one-liners misread code: `$(` in JavaScript, `||` alternations and Go backticks have broken files.
  Keep scripted edits to exact strings, and `die` when a string is not found.
- Shell heredocs and quoting can drop backslashes (`\\.\pipe\` became `\.\pipe\`; a `\0` in a commit message became a
  NUL byte). Write text with Windows paths or backslashes to a file with an editor, then grep for it.

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
| Designs for coming work, and the window's look | [docs/design/](docs/design/) |
| Research behind decisions (Discord's status card, immutable releases) | [docs/research/](docs/research/) |

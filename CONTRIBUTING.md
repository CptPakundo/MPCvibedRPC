# Contributing

The program is AI-generated ("vibe coded"): the code, tests and docs were written with Claude from the owner's prompts
and testing. Bug reports, reviews and pull requests with tests are welcome all the same.

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
- **Privacy first.** No personal data in commits, code, docs, tests, screenshots or release files: no email addresses,
  local paths or user names, machine details, tokens. Scan the diff before the first push.
- **Generic names.** UI text, docs, code comments and new tests use made-up titles ("Sample Show S01E02", "Sample Movie
  (2020)"), never real shows, films or episode numbers. Exempt: the recorded golden data in
  `internal/*/testdata/*.gz`, and real behaviour about a specific source (the Bulbapedia episode lists).
- **Golden test data is frozen.** `internal/*/testdata/*.gz` was recorded once and its generators no longer exist.
  Never regenerate or "fix" it; make the code match it.
- **Never touch a real setup while testing.** Tests and manual checks must not contact the real Discord, start
  presence for a real player, sign in to a real Plex, Stremio, Jellyfin or Emby account, or change a real player's
  settings, the registry or `%APPDATA%`/`%LOCALAPPDATA%`. Use the fakes and throwaway data folders in
  [Running the program safely](docs/maintaining.md#running-the-program-safely).
- **No regressions, no churn.** Weigh pros and cons; don't change working behaviour for its own sake. New behaviour that
  affects existing users stays off or unchanged by default unless the owner decides otherwise.
- **Be exact about what was tested.** Say what ran for real, what ran against fakes, and what was not tested.
- **Release files are only downloaded through `tools/ci/dev-download.sh`**, never with a bare `gh release download`, a
  browser or curl, so the download counter can leave development downloads out
  ([how](docs/maintaining.md#downloading-release-files)).
- **Docs present the program as it is now**: no history of earlier versions or former names (the upstream credit to
  angeloanan/MPC-DiscordRPC stays in the README, `LICENSE` and `THIRD-PARTY-NOTICES.md`).

## Working on Windows
- Working-copy files are CRLF (git `autocrlf`; the index is LF), except `internal/jsre/*_table.go`. Some editing tools
  silently rewrite a CRLF file as LF: convert it back before committing so diffs stay small.
- `gofmt -l` flags every CRLF file on Windows. Check formatting on LF copies (or rely on the Linux CI gate), and never
  `gofmt -w` a CRLF file in place.
- Scripts that rewrite many files can damage them silently: read the diff before committing.

## Building
Go is needed at the version in `go.mod` (the `go` line; currently 1.27.x). `go vet ./...` and `go test -count=1 ./...`
are the basics; CI runs the rest. The default branch is `master`.

## Where to find what
| You want | Read |
|---|---|
| Rules for AI coding tools | [AGENTS.md](AGENTS.md) |
| Package map, how players are read, the engine's state, building, testing, CI, adding a player, setting, catalog or host | [docs/development.md](docs/development.md) |
| Releasing, screenshots, running the program and real players safely, downloading release files | [docs/maintaining.md](docs/maintaining.md) |
| Why the program behaves as it does; what it leaves out; what is open | [docs/decisions.md](docs/decisions.md), [docs/roadmap.md](docs/roadmap.md) |
| How AI is used and how the work is checked | [docs/ai.md](docs/ai.md) |
| How it works for users; every setting; troubleshooting | [docs/how-it-works.md](docs/how-it-works.md), [docs/config.md](docs/config.md), [docs/troubleshooting.md](docs/troubleshooting.md) |
| Every address the program contacts; checking a download | [SECURITY.md](SECURITY.md) |

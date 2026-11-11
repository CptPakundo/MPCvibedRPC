---
name: release
description: MPCvibedRPC's release procedure - version bump, changelog entry, local checks, pull request, merge, tag, release run and the verify job. Use when preparing or publishing a release; steps 3-4 (merge to verify) go to the releaser agent.
---
# Releases
Steps 1-2 are done in the session that made the change; steps 3-4 (from the merge to the verify log) go to the
`releaser` agent with the pull request number, the version and the owner's go-ahead in their words.

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
   Then push, `gh pr create`, wait for CI (`gh run watch`) and read the logs to confirm the expected steps ran. The
   live jobs ran when the pull request was opened: if later commits touch the engine or a player reader, run `ci.yml`
   by hand (`workflow_dispatch`) on the branch before the release.
3. With the owner's go-ahead: `gh pr merge <n> --rebase --delete-branch` (GitHub refuses to rebase a branch that holds
   merge commits, such as merged agent branches: use `--merge` then, or rebase the branch locally first), then
   `git tag -a vX.Y.Z -m "MPCvibedRPC X.Y.Z"`
   with the Claude identity on the merged commit, `git push origin vX.Y.Z`. The release workflow checks the tag
   against `main.go`, tests, builds and smoke-tests every program on its own system (the Windows smoke test covers DPI
   awareness, version details, resting and a self-update against a fake server), writes the `.sha256` files, attests,
   publishes it as a **draft**; `reproduce` rebuilds the exe and both Linux programs from the tag on another
   runner at the same time (no job waits for it but `verify`), and `verify` first checks that their checksums equal the
   release's (they must match byte for byte), then runs `tools/ci/verify-release.ps1`: on the draft the ten assets,
   every checksum and build record and the exe's version; only if they pass is the release made public (and latest),
   followed by the checks that need it public: that it is the latest release, and a real self-update from the previous
   release. A release whose draft checks fail stays a draft (nobody's updater sees it): fix, then delete the draft and
   the tag, or publish it by hand once checked.
4. Read the `verify` job's log (`gh run view --log`) and report. Nothing needs re-running locally after a release.
5. A release with new features also updates the repository to match: its description and topics
   (`gh repo edit --description ... --add-topic ...`), so they name every player and system the program supports.
   The owner gave a standing go-ahead for this.

Merging, tagging and publishing are outward-facing: do them only with the owner's go-ahead.

## Pitfalls (lessons)
- The repo does not require passing checks to merge: `gh pr merge --auto` merges at once. Wait for the PR's CI
  (`gh run watch`) before merging.
- The default branch is `master`, not `main` (`gh pr create --base master`). Never name a branch like a tag
  (`v0.9.20`): `git push origin v0.9.20` becomes ambiguous; use `release-X.Y.Z`.
- `gh pr merge --rebase` is refused for a branch with merge commits (agent branches merged in): use --merge or
  rebase locally first.
- Save a failed job's log before `gh run rerun` (the rerun erases it).
- Run `govulncheck` locally before the first push: a Go advisory published the same day would have failed the PR.
- A changelog line may have at most 130 characters (`TestChangelog`).

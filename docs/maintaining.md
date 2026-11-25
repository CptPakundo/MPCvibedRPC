# Maintaining

For maintainers: how MPCvibedRPC is released, how its screenshots are taken, how the README is previewed and how
release files are downloaded. Building, testing and running the program safely are in [development.md](development.md);
the rules for everyone are in [CONTRIBUTING.md](../CONTRIBUTING.md).

## Rules for maintainers
- **Outward-facing steps need the maintainer's go-ahead:** merging, tagging and publishing; deleting branches, releases or
  files; changing repository settings.
- **The maintainer's product decisions** are recorded in [decisions.md](decisions.md); the open ones are in the
  roadmap's [Open decisions](roadmap.md#open-decisions-the-maintainers). Don't change listed behaviour without saying which
  decision it touches.
- **Release files are only downloaded through `tools/ci/dev-download.sh`** ([how](#downloading-release-files)).
- **Dependabot pull requests** are reported (what they change, whether CI is green) and merged only with the
  maintainer's yes.
- **A release with new features** also updates the repository description and topics ([step 5](#releasing)).
- **Code signing is on hold** until a route exists that does not put a personal name in the signature
  ([decision](decisions.md#releases)).

## Releasing
One branch and one pull request per release: the change, the version bump and the release text together. Releases are
bundles: a version collects several changes, and only a fix for something broken, or a security fix, ships alone
([why](decisions.md#releases)).

Steps 1-2 are done where the change was made. Steps 3-4 (from the merge to the verify log) follow a written checklist and need only the pull request number, the
version and the maintainer's go-ahead.

1. **Branch and change.**
   - Bump `var version` in `cmd/mpcvibedrpc/main.go`. Nothing else carries the version: the workflows run
     `tools/ci/version.sh`, which reads it from `main.go`, works out `NEXT` (the next patch version, which the smoke
     test's self-update installs) and, on a tag, fails when the tag does not match. Docs use `X.Y.Z`.
   - Add the version's entry at the top of `internal/assets/changelog.json` (short lines, one emoji each, at most 130
     characters). The window shows it once after the update, and `TestChangelog` fails until the newest entry matches
     `main.go`.
   - That entry is also the GitHub release's text: `release.yml` renders it with `tools/relnotes` (heading, the
     entry's lines, the "Get it" footer). A release that needs more words (a security advisory) gets them in
     `.github/release-advisory.md`, which the tool inserts after the lines; delete that file in the pull request after
     the release that needed it.
   - Retake screenshots only when the part of the window they show changed ([Screenshots](#screenshots)).
2. **Check locally, then open the pull request.**
   - `go vet ./...` (also with `GOOS=linux` and `GOOS=darwin`), gofmt on LF copies, `go test -count=1 ./...` and
     `govulncheck` (a Go advisory published the same day would fail the pull request).
   - Scan the diff and new files for personal data before the first push (the command is in
     [CONTRIBUTING](../CONTRIBUTING.md#ground-rules)), and check that the commits and the tag name no personal email.
   - Push, `gh pr create --base master`, wait for CI (`gh run watch`) and read the logs to confirm the expected steps
     ran.
   - The live jobs ran when the pull request was opened. If later commits touch the engine or a player reader, run
     `ci.yml` by hand (`workflow_dispatch`) on the branch before the release.
3. **Merge, tag and publish** (with the maintainer's go-ahead).
   1. **Wait for the pull request's CI.** The repository does not require passing checks, so `gh pr merge --auto`
      would merge at once.
   2. **Merge:** `gh pr merge <n> --rebase --delete-branch`. GitHub may refuse to rebase a branch that holds merge
      commits (seen once with merged helper branches): use `--merge` then, or rebase the branch locally first.
   3. **Tag** the merged commit with an annotated tag and push it. An annotated tag stores the tagger's name and email,
      so tag with the same neutral identity used for commits, never a personal email:
      `git -c user.name="<name>" -c user.email="<noreply address>" tag -a vX.Y.Z -m "MPCvibedRPC X.Y.Z"`, then
      `git push origin vX.Y.Z`.
   4. **The release workflow** (`.github/workflows/release.yml`) checks the tag against `main.go`, runs the tests,
      builds the programs on Windows, Linux and macOS runners and smoke-tests each on its own system (Windows: tray,
      Discord pipe, resting while Discord is closed, DPI awareness, version details, autostart, a self-update against a
      fake server; macOS and Linux: the API, login item, player setup, a second start, and on macOS opening the app
      like the Finder does). It writes the `.sha256` files, attests the builds and publishes a **draft**.
   5. **`reproduce`** rebuilds the exe and both Linux programs from the tag on another runner at the same time
      (`tools/ci/reproduce.sh`); only `verify` waits for it.
   6. **`verify`** checks that the rebuilt files' checksums equal the release's (byte for byte), then runs
      `tools/ci/verify-release.ps1` on the draft: the ten assets, every checksum and build record, the exe's version.
      Only if these pass is the release made public (and latest). Then come the checks that need it public: that it is
      the latest release, and a real self-update from the previous release.
   7. **A draft that fails** stays a draft (nobody's updater sees it): fix, then delete the draft and the tag, or
      publish it by hand once checked.
4. **Read the `verify` job's log** (`gh run view --log`) and report. Nothing needs re-running locally after a release.
5. **Repository details.** A release with new features also updates the repository description and topics
   (`gh repo edit --description ... --add-topic ...`) so they name every player and system the program supports.

### Pitfalls
- The default branch is `master`, not `main`. Never name a branch like a tag (`v0.9.20`): `git push origin v0.9.20`
  becomes ambiguous; use `release-X.Y.Z`.
- **A rerun keeps the earlier attempts' logs.** After `gh run rerun`, read the failed attempt with
  `gh run view <id> --attempt 1 --log` (or `--log-failed`).
- A release run whose macOS job is cancelled with "not acquired by Runner" built nothing and left no draft:
  `gh run rerun <id> --failed` is the remedy.
- Versions stay `0.9.x`, three-part: the updater compares three numbers.

## Downloading release files
Release files are only downloaded through `tools/ci/dev-download.sh`, never with a bare `gh release download`, a
browser or curl. It records the download in a ledger on the `stats` branch, so the download counter
(`.github/workflows/downloads.yml`, see [development.md](development.md#ci)) leaves it out:

    tools/ci/dev-download.sh download BY TAG DIR [PATTERN]
    tools/ci/dev-download.sh record BY TAG/FILE[=N]

- `download` fetches into `DIR` (new or empty), then records it.
- `record` is for a download made another way, such as a test copy updating itself.
- `BY` says who downloaded ("release check v0.9.39 draft", "an automated agent"). It needs `gh` signed in with write access.

To check a release without downloading anything, GitHub lists each file's sha256 as `digest`
(`gh api repos/OWNER/REPO/releases/tags/vX.Y.Z`). How users check a download:
[SECURITY.md](../SECURITY.md#checking-a-download).

## Screenshots
The README's pictures (`docs/images/window-main.jpg`, `window-privacy.jpg`; `window-plex.jpg` is used by
[troubleshooting](troubleshooting.md)) are taken again when the part of the window they show changes:

1. Build the program and start it as a throwaway copy ([how](development.md#running-the-program-safely)), with showing
   on Discord off and `{"app": {"seenVersion": "<version>"}}` in its `config.json` so the What's new box stays closed.
2. Open its settings page (the address and token are in `ipc.json` in that folder) in dark mode, at the size the
   program itself uses: Edge's `--window-size=660,800` (`internal/winsys/window.go`). That is the outer window; about
   35 px go to the title bar and border, which leaves a 660 x 760 page. A high-DPI screen gives sharper pictures.
3. For `window-main.jpg`, paste `tools/screenshots/demo.js` into the page's console: it shows a made-up
   "Sample Show" playing and sample stats, and nothing is sent anywhere. Take the General tab. For
   `window-privacy.jpg` (Privacy tab, scrolled to "Services that may see your titles") and `window-plex.jpg`
   (Players tab, scrolled to Plex), reload the page first, so showing on Discord is off.
4. Save them 800 pixels wide (800 x 921). Only made-up titles; nothing from a real library or account.

## Previewing the README
`gh api markdown -f mode=markdown -f text="$(cat README.md)"` renders it the way GitHub does. `mode=gfm` misleads: it
renders differently from the repository page.

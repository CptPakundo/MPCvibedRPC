# How AI was used

MPRP is AI-generated ("vibe coded"). This page says what that means here, so you can judge it for yourself.

## Who did what
- **The AI** (Claude, by Anthropic) wrote the code, the tests, the CI workflows and the
  documentation, ran the tests, and read the CI results.
- **The maintainer** of the repository gives the direction (what to build, what to leave out), makes the product decisions
  ([listed here](decisions.md)), tries the results on their own computer, and approves every release. The maintainer does
  not present the code as their own.
- The idea comes from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC) (MIT).

## How the work is checked
Nobody reads every line by hand, so the checks have to be real:
- Unit tests for every package with logic of its own, and engine tests against a fake Discord and fake players, on
  Windows, macOS and Linux; two code analysers (staticcheck, deadcode) and a vulnerability check on every change,
  for the Windows code as well as the Linux code. One engine test runs random orders of events (the player pauses,
  Discord restarts or refuses, the settings change) and checks that Discord ends up showing what it should.
- Live tests on CI against real players (mpv, VLC, Celluloid, IINA, PotPlayer, Kodi) and real media servers (Plex,
  Jellyfin, Emby), each started fresh on a throwaway runner with made-up test videos.
- A weekly run against the newest players, servers and catalog services, which opens an issue when something outside
  changes.
- Every release is rebuilt from its tag on a second runner and must match byte for byte, and a real self-update from
  the previous release is tried before it goes public.
- Changes are explained with exactly what was tested for real, what only against fakes, and what not at all; the
  release notes and pull requests say so.

## Working with AI agents on it
What has held up while developing this project with AI agents (any tool, any model):
- **One agent decides, others carry out.** Design, triage, coupled edits and every final check (reading the whole
  diff, the evidence, CI) stay with the agent or person in charge. Routine procedures that follow a written checklist
  (a release from `docs/maintaining.md`, reading CI logs, edits from an explicit list), read-only audits with a fixed
  report format, and fixes on disjoint files in separate worktrees can go to helper agents, on a cheaper model.
- **A brief is the helper's only context.** State the goal, what is ruled out, the files, what done looks like and the
  report's shape. Check every claim before it goes into a brief: a helper builds on an unchecked guess as if it were
  fact (once nearly a regression in the autostart code). Tell it to source shared scripts it calls (`tools/ci/lib.sh`).
- **Findings are claims, not facts.** Check each against the intent and the tests before fixing: in one audit, three
  plausible fixes were wrong (one broke the frozen golden data, one contradicted a deliberate pinned behaviour, one
  would have hidden most Stremio playback). Run the package's tests after each fix; grep the tests for the behaviour.
- **Read the evidence, not the summary.** A helper's report can misname a cause or claim a fix that did not land
  (a commit that said it fixed a file and did not touch it); the logs, the diff and `git show` settle it.
- **Audit the merged result against the whole project.** A file-scoped helper's change once broke the setup button
  through a shared helper; only the Windows smoke test caught it.
- **Refactors by agents remove copies but rarely shrink the code:** say the gain is readability, not size.
- **Independent reviews help most when they have not seen the reasoning:** give a reviewer the code and the docs, not
  the conclusion. Two reviews with different briefs (facts against the code; plain language for a newcomer) found
  different problems.

## What that does not cover
- The program has not had a security audit ([SECURITY.md](../SECURITY.md)).
- macOS and Linux are mostly tested on CI runners, not on real desktops; Stremio has only been tested against fakes
  built from its source code.
- AI-written code can be wrong in ways tests do not catch. If you find such a case, please
  [open an issue](https://github.com/CptPakundo/MPRP/issues/new/choose).

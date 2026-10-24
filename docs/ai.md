# How AI was used

MPCvibedRPC is AI-generated ("vibe coded"). This page says what that means here, so you can judge it for yourself.

## Who did what
- **The AI** (Claude, by Anthropic, through Claude Code) wrote the code, the tests, the CI workflows and the
  documentation, ran the tests, and read the CI results.
- **The owner** of the repository gives the direction (what to build, what to leave out), makes the product decisions
  ([listed here](decisions.md)), tries the results on their own computer, and approves every release. The owner does
  not present the code as their own.
- The idea comes from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC) (MIT).

## How the work is checked
Nobody reads every line by hand, so the checks have to be real:
- Unit tests for every package with logic of its own, and engine tests against a fake Discord and fake players, on
  Windows, macOS and Linux; two code analysers (staticcheck, deadcode) and a vulnerability check on every change.
- Live tests on CI against real players (mpv, VLC, Celluloid, IINA, PotPlayer, Kodi) and real media servers (Plex,
  Jellyfin, Emby), each started fresh on a throwaway runner with made-up test videos.
- A weekly run against the newest players, servers and catalog services, which opens an issue when something outside
  changes.
- Every release is rebuilt from its tag on a second runner and must match byte for byte, and a real self-update from
  the previous release is tried before it goes public.
- Changes are explained with exactly what was tested for real, what only against fakes, and what not at all; the
  release notes and pull requests say so.

## What that does not cover
- The program has not had a security audit ([SECURITY.md](../SECURITY.md)).
- macOS and Linux are mostly tested on CI runners, not on real desktops; Stremio has only been tested against fakes
  built from its source code.
- AI-written code can be wrong in ways tests do not catch. If you find such a case, please
  [open an issue](https://github.com/CptPakundo/MPCvibedRPC/issues/new/choose).

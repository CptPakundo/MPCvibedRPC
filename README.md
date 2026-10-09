# MPCvibedRPC (hassle-free edition)

> **Entirely AI-generated ("vibe coded").** Every line of this program, its tests, its documentation and its build setup was written by an AI (Claude, by Anthropic) in conversation with the repository owner. The owner did not write it, takes **no credit** for it, and has not reviewed it line by line: their part was giving prompts and trying the result. Use it accordingly. There is no warranty (see `LICENSE`), no promise that it is correct, secure or maintained, and nobody here is an expert you can ask about the code. The idea and the approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); the credit for that belongs to its author.

Discord Rich Presence for MPC-HC, based on the approach of
[angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC):
it polls MPC-HC's built-in web interface and forwards what you're watching to Discord.
MIT licensed; see `LICENSE` and `THIRD-PARTY-NOTICES.md`.

<p align="center">
  <img src="docs/images/window-main.jpg" alt="The main window: status, a preview of the Discord card, and the settings tabs" width="380">
  <img src="docs/images/window-privacy.jpg" alt="The Privacy tab: switches for each online service that may see your titles" width="380">
</p>

## Use it
1. Download **MPCvibedRPC.exe** and run it. There is nothing to install and nothing else is needed.
2. A small window opens. Press **Start presence**. If MPC-HC isn't answering, the window offers
   **Turn on MPC-HC web interface** (it closes and reopens MPC-HC for you).
3. Optional: switch on **Start with Windows**. It then starts quietly at login, with a tray icon
   (double-click: settings; right-click: start/stop presence, quit). Turn off **Show this window when I open the app**
   (General tab) if you want it to go straight to the tray when you open it yourself too.

Run the program again at any time to bring the window back. Closing the window leaves it running in the tray.

Requirements: Windows 10/11 (Edge or Chrome for the window; Edge ships with Windows), MPC-HC, the **Discord
desktop app** with User Settings > Activity Privacy > "Share my activity" on.

Checking a download: each release has a `.sha256` file, and GitHub keeps a signed record that the exe was built from this
repository by its release workflow. With the [GitHub CLI](https://cli.github.com/):

    gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC

The program itself is not code-signed, so Windows SmartScreen may warn about it. See [SECURITY.md](SECURITY.md).

Everything it stores is in `%LOCALAPPDATA%\MPCvibedRPC`: `config.json` (settings), `mpcvibedrpc.log`,
`artwork-cache.json` and `window\` (the settings window's own browser profile, so your regular Edge is left alone). Delete that folder and the program to remove all traces (turn off "Start with Windows" first).

## Building the .exe (maintainers)
The program is written in Go and uses only the standard library: one small file (about 8 MB, a few MB of memory while
running), no runtime, nothing to install. Building needs [Go 1.24+](https://go.dev/dl/) and no internet beyond that.

    go run ./tools/mkrsrc -version 0.9.2 -out cmd/mpcvibedrpc/rsrc_windows_amd64.syso    (icon + version details, optional)
    go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=0.9.2" -o MPCvibedRPC.exe ./cmd/mpcvibedrpc

`-H=windowsgui` makes it a windowed program, so no console flashes up. You can build from any OS (`GOOS=windows`).
Discord's local protocol is implemented in `internal/discord`.

## Updating
The program can update itself from GitHub Releases. Put your repository (`owner/repo`) in **Updates** in the window
(or in `config.json` under `app.updateRepo`). It then checks daily, and **Check now / Install** downloads the new
`MPCvibedRPC.exe`, verifies it (and its `.sha256` if the release has one), swaps itself in and restarts. To publish
a version:

1. `git tag v0.9.2 && git push --tags` - the workflow in `.github/workflows/release.yml` runs the tests, builds the exe
   on a Windows runner, smoke-tests it (tray, Discord pipe, autostart, self-update) and attaches `MPCvibedRPC.exe`
   and its `.sha256` to a release. The tag is the version.

Without a GitHub repository, just replace the exe by hand; settings are kept.

## Where things are (for contributors)
| Path | Job |
|---|---|
| `cmd/mpcvibedrpc` | the program: single instance, wiring, log, flags (`--background`, `--after-update`) |
| `internal/engine` | polls MPC-HC, looks up artwork, sends presence; start/stop/apply settings live |
| `internal/core` | settings defaults, filename parsing, the Discord activity itself |
| `internal/artwork`, `internal/jsre` | catalog lookups and episode titles; a regex engine with JavaScript semantics, which the title-matching rules rely on |
| `internal/discord` | Discord's local IPC protocol (Unix socket / Windows named pipe) |
| `internal/store`, `internal/jsonx` | `config.json` in `%LOCALAPPDATA%`, the settings schema shown in the window, validation |
| `internal/server`, `internal/assets` | local settings page (`ui.html`) and API on `127.0.0.1`, protected by a per-launch token |
| `internal/winsys` | Windows-only bits: run at login, MPC-HC web interface, window, tray icon (Win32, no helper process) |
| `internal/updater` | GitHub release check, download, checksum, swap and restart |
| `tools/` | the resource (.syso) writer and the CI scripts |
| `docs/` | the full settings reference (`docs/config.md`) |

`go test ./...` runs everything: the parsers, artwork matching and activity builder are checked against recorded reference output
(tens of thousands of cases, stored as test data in each package's `testdata/`), the engine against a fake MPC-HC and a fake Discord, and the
whole program is built and driven end to end. On GitHub, Linux runs the tests with the race detector and Windows additionally runs a smoke test of the real exe.

## Behaviour
- **Watching, not Playing.** The activity is sent as type "Watching" with a real Discord
  progress bar (elapsed and total time) while the video plays. It follows seeks and
  playback speed. Discord can't freeze that bar, so while paused you get a text bar instead:
  `⏸ ▰▰▰▰▰▰▱▱▱▱▱▱ 25:00 / 50:00`.
- **Context-aware.** Your status reads "Watching <title>"; the second line shows the
  season and episode with the episode's title (`S01E03 · Episode title`) or the year for movies. For episodes with a known
  season, the image tooltip uses the "Season 1, Episode 3" form that Discord turns into S1E3.
- **Smart titles.** Release junk (BluRay, 1080p, x265, HDR, DDP5.1, WEB-DL, release groups...)
  is stripped, and titles, years, seasons and episodes are recognised in file names such as
  `Show Name Ep03 (2010).mkv`, `Show.Name.S05E14.720p.HDTV.x264-GRP.mkv` or
  `[Group] Show Name - 05 [1080p].mkv`.
- **Cover art, from several sources.** The title parsed from the filename is looked up on
  up to six sources at once, and the highest-priority one with a trustworthy match wins:
  TMDB (only if you add a free API key), IMDb's own search, Cinemeta, TVmaze, AniList, Kitsu and
  MyAnimeList (anime; AniList, then Kitsu, then MyAnimeList are asked first for fansub-style
  filenames, everything else goes IMDb-first) and, as a last resort, Wikipedia (Hebrew titles use
  he.wikipedia.org). Matching is accent-insensitive and checks alternate titles, so a title that a
  catalog lists under a translated name, or whose catalog entries are split into several parts, still resolves.
  A wrong cover is worse than none, so loose guesses are rejected. If a lookup is slow the
  presence appears right away and refreshes with the cover when it arrives. Results are cached
  in `artwork-cache.json`.
  - **Seasons that are different titles.** When a file names a season (`S04`) or a production
    code, the season is resolved in two ways. (1) Catalogs that list each season as its
    own title: a file marked `S04` resolves to the entry named like "<Show>: Fourth Stage" (movies
    included). Numbers are read from words such as "Fourth Stage", "2nd Season", "Season 3" and "II", and
    "Final" is the season after the last numbered one. A production code's letters also match a
    title's initials. (2) Shows with numbered seasons under one title: the season is taken from
    TVmaze (or TMDB with a key), together with that season's own poster. The season's name is shown
    on the second line (`<Season name> · S14 · <code>`).
    If a season can't be placed you get the show's poster and no season name, never a guess.
  - Still no cover? `mpcvibedrpc.log` lists what each source returned. Then pin it (see the end of this section):
  - **Episode titles.** When the filename has an episode number but no name
    (`Show.Name.Ep03...`), the name is looked up (TVmaze, Cinemeta, or TMDB with a key) and shown as
    `S01E03 · <Episode title>`. If a file's numbering doesn't match the catalog, nothing is guessed and
    you keep the plain `E03`. Turn off with `episodeTitles: false`.
  - **Info lines.** With a catalog match, a movie shows its year and genres (`1999 · Action, Adventure`) and
    its rating and director (`★ 7.1 · Dir. <name>`); an episode shows its title line and
    the genres and rating (`Action, Adventure · ★ 8.3`). The data comes from Cinemeta (keyed by the IMDb id, cached) and is
    skipped when there is no IMDb id (e.g. AniList-only anime). While paused, the second line
    becomes the pause bar. Turn off with `richInfo: false`.
  - **Robust lookups.** Responses are cached for 10 minutes, identical in-flight requests are
    shared, a timeout / 5xx / 429 is retried once, requests to AniList and MyAnimeList are spaced out,
    and a source that keeps failing is paused for a minute. Order is set by `artworkSources`
    (general) and `animeSources` (anime files).
  - **Anime title language.** For anime matched on AniList, `animeTitles: 'auto'` (default) shows the
    AniList name the filename resembles most: a file that reads like the romaji name gets the romaji
    title, one close to the English name gets the English title. `'english'` / `'romaji'` always use
    that one, and `'file'` keeps the spelling from the filename. Matching checks every name either way.
  - **Anime that IMDb identified.** If IMDb or TVmaze matched a title and its genres include
    Animation, AniList is asked once for the same title (exact name and year). Only the displayed
    name is affected, following `animeTitles`; the poster, link and ratings stay as they were. A
    difference in punctuation alone never replaces the nicer spelling.
  - **Odd filenames it copes with.** Edition and language notes after the year (`1994.DC`,
    `2019.KOREAN`, `(1982) Final Cut`), acronyms written with dots, `Title- Subtitle`
    (shown as `Title: Subtitle`), release-site banners (`www.Site.com - Title`), movie groups such as
    `[YTS.MX]` (not mistaken for anime), a title that ends in a year-like number, a studio possessive
    (`Studio's Show Name`), daily shows (`Show 2024.05.01 Guest`),
    four-digit anime episodes (`Show - 1087`, `S01E1000`) and Japanese `第28話` / `1000話`.
    Camera/phone/screen-recording names and bare dates are never looked up.
  - **Production codes that run across catalog entries.** For files named with a production code
    (`<Show> - S18 - AB065`), when the code number is higher than the matched entry's episode
    count, the next entry that continues the code is used and the number is counted inside it. The
    badge shows your file's season with that number. Episode titles come from that entry's own
    list on Kitsu (`episodeSources` includes `kitsu`). `folderEpisodeNumbers: true`
    instead numbers by position in the season folder.
  - **Episode titles from a fan wiki.** Files with production codes get their title from
    Bulbapedia's own episode list, since those codes are that wiki's numbers (`episodeSources` starts with
    `bulbapedia`; only the series it covers are known, other series fall back to the catalogs).
  - **Show name from the folder.** If a filename has only an episode number (`S01E02.mkv`,
    `Episode 3.mkv`, `03.mkv`), the folder's name is used as the show, e.g. `...\Show Name\Season 1\S01E02.mkv`.
    `Season N` folders are skipped (and supply the season for bare numbers); generic folders like
    `TV` or `Downloads` are ignored. Folder names get the same cleanup (`Show.Name.2008.1080p`).
    Disable with `useFolderName: false`.
  - **Movies named with a franchise prefix.** A file such as `Franchise Movie 15 - Subtitle`
    is retried by its subtitle, and a result is only accepted if it contains both the subtitle and
    the franchise name, so unrelated titles aren't matched.
    Advanced settings in the window (or `config.json`): `artworkAliases: { 'my show': 'Official Name' }` searches under another name,
    `artworkOverrides: { 'my show': 'https://.../poster.jpg' }` uses your own image.
  - Set `showArtwork: false` (the first switch on the Privacy tab) to stay fully offline (this also disables episode-title lookups) (otherwise the title parsed from your
    filename is sent to these services).
- **Preview.** While a status is showing, the window has a **How it looks on Discord** card: the title lines, cover,
  progress bar and link button, built from what was actually sent (so it also shows the basic-mode fallback).
- **Hiding things** (Privacy tab). **Hide what I'm watching** shows only "Watching a video" with the progress bar (no title, cover or buttons, and nothing looked up). **Never show these files** takes folders, names or wildcards (such as `Private` or `*.xyz`); a listed file shows no status at all and is never looked up. Hidden names are not written to the log either.
- **Privacy controls** (Privacy tab). Every online service the program can ask about a title has its own switch, with what it
  is used for and where requests go; a switched-off service is never contacted. One switch turns all lookups off (fully
  offline). **Clear my status when paused for N minutes** takes the status down after a long pause (0 = never) and brings
  it back when you play again or seek. **Clear cover cache** forgets everything that was looked up. There is no analytics or
  tracking, and file paths are never sent.
- Clears itself when you stop or close MPC-HC; reconnects on its own if Discord restarts.
  If Discord ever rejects the Watching format, it falls back to a basic presence automatically.
- Options live in the settings window and save as you change them (**Restore defaults** resets them all); the full list with defaults is in [`docs/config.md`](docs/config.md) (extra ones go in
  `config.json`). Set `activityType` to `playing` for the classic look. Log: `mpcvibedrpc.log` (also in the window).


## Troubleshooting
- Nothing shows: the window's MPC-HC pill says whether it is reachable. If not, press **Turn on MPC-HC web interface**,
  or check http://127.0.0.1:13579/variables.html while a video plays.
- Discord pill stays on "Waiting": use the Discord desktop app (not the browser) and start it before or after, either works.
- No icon: image keys in the settings must match art assets in the Discord application.
- No window appears: run the program again, or right-click its tray icon > Open settings.

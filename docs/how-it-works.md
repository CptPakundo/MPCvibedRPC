# How it works

The details behind what the [README](../README.md) promises: how titles are cleaned up, where covers and episode
names come from, and how each player is read.

## What it does
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
  - Still no cover? `mpcvibedrpc.log` lists what each source returned. Then pin it with `artworkAliases` or `artworkOverrides` on the Advanced tab (see the end of this list).
  - **Episode titles.** When the filename has an episode number but no name
    (`Show.Name.Ep03...`), the name is looked up (TVmaze, Cinemeta, or TMDB with a key) and shown as
    `S01E03 · <Episode title>`. If a file's numbering doesn't match the catalog, nothing is guessed and
    you keep the plain `E03`. Turn off with `episodeTitles: false`. An anime matched on AniList under its
    Japanese name is also looked up under its English name, which is how TVmaze lists it; a running episode
    number (`Show - 67`) is placed in the right season there, and the file's own number stays on the card.
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
    the franchise name, so unrelated titles aren't matched. The other way round, `Title A Franchise Mystery 2025`
    (from `Title: A Franchise Mystery`, a name catalogs often list as just `Title`) is retried as `Title` when
    nothing else matches, only with the file's year, and shown as `Title: A Franchise Mystery` (also for
    `A ... Story`, `Movie`, `Film`, `Adventure`, `Tale`, `Saga`, `Legend`, `Odyssey`).
    Advanced settings in the window (or `config.json`): `artworkAliases: { 'my show': 'Official Name' }` searches under another name,
    `artworkOverrides: { 'my show': 'https://.../poster.jpg' }` uses your own image.
  - Set `showArtwork: false` (the first switch on the Privacy tab) to stay fully offline (this also disables episode-title lookups) (otherwise the title parsed from your
    filename is sent to these services).
- **Preview.** While a status is showing, the window has a **How it looks on Discord** card: the title lines, cover,
  progress bar and link button, built from what was actually sent (so it also shows the basic-mode fallback).
- **Hiding things** (Privacy tab). **Hide what I'm watching** shows only "Watching a video" with the progress bar (no title, cover or buttons, and nothing looked up). **Never show these files** takes folders, names or wildcards (such as `Private` or `*.xyz`); a listed file shows no status at all and is never looked up. Hidden names are not written to the log either.
- **Privacy controls** (Privacy tab). Every online service the program can ask about a title has its own switch, with what it
  is used for and where requests go; a switched-off service is never contacted. One switch turns all lookups off (fully
  offline). **Clear my status when paused for N minutes** takes the status down after a long pause (30 minutes by default; 0 = never) and brings
  it back when you play again or seek. **Clear cover cache** forgets everything that was looked up. There is no analytics or
  tracking, and file paths are never sent.
- **Your stats** (General tab): how many videos were shown on Discord, how many were found in a catalog, how long they
  played and with which player. Numbers only, never titles, kept in `stats.json` on this computer; nothing is sent
  anywhere. **Reset stats** starts over. (The download count at the top of this page is GitHub's public count of release
  downloads.)
- **What's new.** After an update (and on a new install) the window shows the new version's changes once; the
  **Changelog** button at the top shows every version's.
- Clears itself when you stop or close the player; reconnects on its own if Discord restarts.
  If Discord ever rejects the Watching format, it falls back to a basic presence automatically.
- Options live in the settings window and save as you change them (**Restore defaults** resets them all); the full list with defaults is in [`docs/config.md`](config.md) (extra ones go in
  `config.json`). Set `activityType` to `playing` for the classic look. Log: `mpcvibedrpc.log` (also in the window).


## Players in detail
The program asks the players in this order and shows the first one that is playing something (if none is, the window still names the first that answers):

| Player | How it is read | Setup |
|---|---|---|
| MPC-HC | its web interface (`http://127.0.0.1:13579/variables.html`) | Options > Player > Web Interface > Listen on port, or the **Set up the player connection** button (it closes and reopens MPC-HC, and keeps a web interface it switches on to this PC only) |
| MPC-BE | the same web interface | the same; MPC-BE keeps the switch in `[WebServer]` of `mpc-be64.ini` or the registry, the button handles both (also this PC only) |
| MPC-QT | its own mpv-style connection (`cmdrkotori.mpc-qt.mpv`), always on | none (its web interface works too, if you turned it on) |
| mpv | its JSON IPC (`input-ipc-server`) | `input-ipc-server=mpvsocket` in `mpv.conf`; the button adds it (`portable_config\mpv.conf` next to a portable mpv, else `%APPDATA%\mpv\mpv.conf`; on macOS and Linux `~/.config/mpv/mpv.conf` with the full path of the socket, such as `/tmp/mpvsocket`) and keeps a name you already set. mpv reads it when it starts. |
| IINA (macOS) | its mpv core's JSON IPC | IINA Settings > Advanced: enable advanced settings and add the mpv option `input-ipc-server` with a socket path. The button does both (it keeps a socket you already set) and IINA uses it once it is reopened. |
| VLC | its web interface (`http://127.0.0.1:8080/requests/status.json`, with a password) | Tools > Preferences > All > Interface > Main interfaces: tick Web, and set a password under Lua > Lua HTTP; copy it to **Advanced > VLC web interface password**. Or press the button: it switches the interface on in `vlcrc` (`portable\vlcrc` next to a portable VLC, else `%APPDATA%\vlc\vlcrc`), gives it a password and fills it in for you. It keeps a password and port you already set, and an interface it switches on only answers this PC (`http-host=127.0.0.1`; VLC also uses that address for streams sent over HTTP without one). VLC rewrites `vlcrc` when it exits, so a running VLC is closed and reopened if something has to change. On macOS the same, in `~/Library/Preferences/org.videolan.vlc/vlcrc`. On Linux VLC needs none of this: it is found through MPRIS. |
| Linux video players | MPRIS over D-Bus: VLC, Celluloid, Haruna, SMPlayer, GNOME Videos (Totem), Showtime, Clapper, Dragon Player, Kodi, Parole, QMPlay2, MPC-QT, and mpv with the mpv-mpris script | none; **Advanced > Find video players through MPRIS** switches it off. Music players and web browsers are left out. |
| Plex | your Plex Media Server's play notifications, for whatever you play with your Plex account: the Plex app on this computer, Plex Web, a TV, a phone | **Advanced > Sign in to Plex**: plex.tv opens in your browser, you approve the sign-in there (the program never sees your password), and it follows your server (choose another one under the button; your own servers come first, then those shared with you). **Plex server address** is only needed when this computer cannot reach the server at the addresses plex.tv lists. |

MPC-HC (2.8.3 here) is what the program was built around; MPC-BE 1.9.1, MPC-QT 26.07, mpv 0.41.0 and VLC 3.0.24 were tried on Windows 11, set up as above (and a development build of VLC 4, which is not released yet). On macOS and Linux, mpv, IINA, VLC, mpv-mpris and Celluloid are tried on GitHub's test machines with every change, with a stand-in for Discord. IINA on a real Mac with the real Discord was tried by a tester; Linux has not been tried on a real desktop yet. Streams show their title (there is no file name or folder). With players other than the MPC family, the image tooltip names the player ("mpv", "IINA", "VLC media player", ...) instead of "Media Player Classic". VLC's web interface is asked after mpv and IINA, and only once its password is set; then MPRIS players, and Plex last, only while you are signed in.

Plex: the program signs in like a Plex app (it introduces itself to Plex as "MPCvibedRPC") and
keeps the sign-in in `config.json`; **Sign out** or **Restore defaults** removes it. It only reads: titles, episode
numbers and the position of what you play. On a server you own, it shows only your own playback, not that of people you
share with; music and photos are never shown as "Watching". Plex playback on any of your devices counts, not only on this
computer. The episode or movie name comes from your server, and cover art is looked up the same way as for files. Plex
is tried on GitHub's test machines with every change: the official Plex Media Server, without a Plex account, with a
stand-in player reporting playback to it. Signing in with a real Plex account, servers shared by someone else and real
Plex apps have not been tried yet.


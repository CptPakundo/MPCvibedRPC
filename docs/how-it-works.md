# How it works

The details behind what the [README](../README.md) promises: what shows on Discord, how titles are cleaned up, where
covers and episode names come from, and how each player is read. Every setting with its default is in
[All settings](config.md).

## The window and the menu
**The tray menu** (Windows) and **the menu bar menu** (macOS) hold:
- a status line (what plays, or what is missing);
- **Open settings**;
- **Show on Discord** / **Stop showing on Discord**;
- **Hide what I'm watching** (ticked when on);
- **Show the title on my streams** (ticked when on);
- **Quit**.

Linux has no tray icon, and closing the window with its X quits the program there too. Keep the window open or
minimised, or let the program start at login; running it again brings the window back. Quit is in the window.

**The settings window** has a status block at the top (with a preview of your status on Discord) and these tabs:

| Tab | What is there |
|---|---|
| General | starting with the system, showing on Discord when the app opens, updates |
| Players | the players on this computer, and the Plex, Stremio and Jellyfin / Emby sign-ins |
| Look | how the status looks: style, progress bar, info lines, links, streams |
| Titles & art | episode titles, title spelling, anime titles, folder names, **Fix a match**, a TMDB key |
| Privacy | hiding titles and files, clearing after a pause, which online services may see a title, Recent connections |
| Advanced | the Discord application ID, how often players are checked, Maintenance (Restore defaults, Open settings folder) |
| Help | Run a check, Copy diagnostics, the log, your stats, credits |

Settings save as you change them; **Restore defaults** (Advanced > Maintenance) resets them all after asking. Closing
the window with its X quits the program (on macOS it closes to the menu bar); minimizing it on Windows sends it to the
tray.

## What shows on Discord
- **Watching, not Playing.** Your status reads "Watching <title>" with Discord's progress bar (elapsed and total time).
  It follows seeking and playback speed. Discord can't freeze that bar, so while paused you get a text bar instead:
  `⏸ ▰▰▰▰▰▰▱▱▱▱▱▱ 25:00 / 50:00`. **Look > Style** switches to the classic "Playing" look.
- **The second line** shows the season and episode with the episode's title (`S01E03 · Episode title`), or the year
  for a movie. For an episode with a known season, the image tooltip reads "Season 1, Episode 3", which Discord turns
  into S1E3.
- **Clickable title and cover.** With **Look > Link to the title's page** on (the default) and the title found in a
  catalog that has a page (IMDb, AniList, ...), both text lines and the cover open that page, next to a "View on IMDb"
  style button. A second button opens another page of the title when there is one (IMDb next to Letterboxd or
  AniList, or TVmaze next to IMDb), and the player's logo opens the player's site. Others can click them on your
  profile; Discord doesn't let you click your own.
  - If Discord refuses these links, the same status is sent without them from then on (it does not drop to the basic
    status). A "View on ..." button Discord refuses is left out of that update only.
  - **Look > Link to Letterboxd when it has the title** (off by default) points the button ("View on Letterboxd") and
    the lines at the title's Letterboxd page instead, when Letterboxd has one. Finding it asks letterboxd.com once per
    title (by the IMDb or TMDB id, never the file name); it is the only switch that contacts Letterboxd.
- **Preview.** While a status shows, the block at the top of the window shows how it looks on Discord: the title
  lines, the cover with the player's logo, the progress bar and the link buttons, built from what was actually sent
  (so it also shows the basic fallback). The second line is underlined (dotted) when it is a link. **Wrong title or
  cover?** below it leads to **Titles & art > Fix a match**.
- **Music.** A song played in a video player (`.mp3`, `.flac` and the like) shows as "Listening to <file name>" (with
  the title hidden: "Listening to music"). It is never looked up in the catalogs and is not counted in the stats. A
  video playing in another player comes first.
- **Clearing and reconnecting.** The status clears itself when you stop or close the player. A player that misses one
  answer (busy for a moment) is asked again a second later before the status goes. If Discord restarts, the program
  reconnects by itself; a status that was cleared after a long pause stays cleared then, and only the player brings it
  back.
- **Fallbacks.** If Discord ever rejects the Watching format for a title, that title gets a basic status instead. A
  status Discord does not take at all is tried again after a growing wait (5 seconds, up to a minute).
- **What's new.** After an update (and on a new install) the window shows the new version's changes once; the version
  number at the top shows every version's.
- **Updates.** The program looks for a new version as it starts and every six hours, and asks or installs it by itself:
  see [Troubleshooting > Updates](troubleshooting.md#updates).
- The log is `mpcvibedrpc.log` (also on the **Help** tab).

## Titles and covers

### Clean titles
Release junk (BluRay, 1080p, x265, HDR, DDP5.1, WEB-DL, release groups, ...) is stripped, and titles, years, seasons and
episodes are recognised in file names such as `Show Name Ep03 (2010).mkv`, `Show.Name.S05E14.720p.HDTV.x264-GRP.mkv`
or `[Group] Show Name - 05 [1080p].mkv` (also `Show Name [S01E02]`, `S2E5`, `E05-E06` and `Ep. 5`). A title the catalog
spells differently (with punctuation a file name can't hold) is shown its way (**Titles & art > Use the catalog's
spelling of titles**).

### Where covers come from
- The title read from the file name is looked up on every cover source at once (seven by default), and the
  highest-priority one with a trustworthy match wins: TMDB (only with your own free API key, **Titles & art > More**),
  IMDb's own search, Cinemeta, TVmaze, AniList and Kitsu (anime) and, as a last resort, Wikipedia (Hebrew titles use
  he.wikipedia.org). Fansub-style file names ask AniList, then Kitsu first; everything else goes IMDb first. The order
  can be changed in `config.json` ([All settings](config.md#titles--art)).
- Matching ignores accents and checks alternate titles, so a title that a catalog lists under a translated name, or
  whose entries are split into several parts, still resolves. A wrong cover is worse than none, so loose guesses are
  rejected.
- If a lookup is slow, the status appears right away and refreshes with the cover when it arrives.
- Answers are cached in `artwork-cache.json`, and forgotten 90 days after the title was last played.
- **Robust lookups:** responses are cached for 10 minutes, identical requests in flight are shared, a timeout or server
  error is retried once, requests to AniList are spaced out, and a source that keeps failing is paused for a minute.
  TVmaze's search can take up to a minute for a show it has not been asked about lately: the program goes on without
  it, keeps waiting in the background, and looks the title up again as soon as the answer is in.
- **Privacy > Look up cover art and titles online** off keeps the program fully offline (no covers, no episode titles).
  Otherwise the title read from your file name is sent to these services; each one has its own switch.

### Seasons
When a file names a season (`S04`) or a production code, the season is found in one of two ways:
1. **Catalogs that list each season as its own title:** a file marked `S04` resolves to the entry named like
   "<Show>: Fourth Stage" (movies included). Numbers are read from words such as "Fourth Stage", "2nd Season",
   "Season 3" and "II", and "Final" is the season after the last numbered one. A production code's letters also match
   a title's initials.
2. **Shows with numbered seasons under one title:** the season comes from TVmaze (or TMDB with a key), with that
   season's own poster. The season's name is shown on the second line (`<Season name> · S14 · <code>`).

If a season can't be placed, you get the show's poster and no season name, never a guess.

**Production codes that run across catalog entries** (`<Show> - S18 - AB065`): when the code number is higher than the
matched entry's episode count, the next entry that continues the code is used and the number is counted inside it. The
badge shows your file's season with that number. **Titles & art > More > Number episodes by their place in the season
folder** numbers by position in the folder instead.

### Episode titles
**Titles & art > Look up episode titles** (on by default).
- When the file name has an episode number but no name (`Show.Name.Ep03...`), the name is looked up (TVmaze, Cinemeta,
  Kitsu, or TMDB with a key) and shown as `S01E03 · <Episode title>`. If a file's numbering doesn't match the catalog,
  nothing is guessed and you keep the plain `E03`.
- An anime found on AniList under its Japanese name is also looked up under its English name, which is how TVmaze
  lists it. A running episode number (`Show - 67`) is placed in the right season there, and the file's own number stays
  on the card. Cinemeta is not used to count a running number through an anime's seasons, because its seasons can hold
  specials that shift the count.
- When several shows share a name (an anime and a later live-action series), the one from the matched show's year is
  used.
- **Specials:** `S00E02` is looked up in season 0. A special named in words (`Show - Special 02`,
  `Show (2010) - OVA 2`, `Show - SP2`) is first searched as a title of its own (AniList lists many OVAs that way) and,
  if nothing is found, as season 0 of the show. It keeps the file's own label (`OVA 2`).
- **Production codes** get their episode title from Bulbapedia's own episode list, since those codes are that wiki's
  numbers (only the series it covers; others fall back to the catalogs). An entry continued as above takes its titles
  from that entry's own list on Kitsu.
- If a catalog does not answer, the title is asked for again while the episode plays (after about 2.5, 5 and 15
  minutes).

### Info lines
**Look > Genres, rating and director** (on by default). With a catalog match, a movie shows its year and genres
(`1999 · Action, Adventure`) and its rating and director (`★ 7.1 · Dir. <name>`); an episode shows its title line and
the genres and rating (`Action, Adventure · ★ 8.3`). The data comes from Cinemeta (by the IMDb id, cached) and is
skipped when there is no IMDb id (for example an anime found only on AniList). While paused, the second line becomes
the pause bar.

### Anime titles
**Titles & art > Anime titles**, for anime found on AniList:
- *Closest AniList name* (the default) shows the AniList name the file name resembles most: a file that reads like
  the romaji name gets the romaji title, one close to the English name gets the English title.
- *Always English* or *Always romaji* always use that one; *Keep the file's spelling* keeps it as in the file.
  Matching checks every name either way.
- If IMDb or TVmaze found a title whose genres include Animation, AniList is asked once for the same title (exact name
  and year). Only the displayed name changes; the poster, link and ratings stay. A difference in punctuation alone
  never replaces the nicer spelling.

### Odd file names
- Edition and language notes after the year (`1994.DC`, `2019.KOREAN`, `(1982) Final Cut`), acronyms written with dots,
  `Title- Subtitle` (shown as `Title: Subtitle`), release-site banners (`www.Site.com - Title`), movie groups such as
  `[YTS.MX]` (not mistaken for anime), a title that ends in a year-like number, a studio possessive
  (`Studio's Show Name`), daily shows (`Show 2024.05.01 Guest`), four-digit anime episodes (`Show - 1087`,
  `S01E1000`) and Japanese `第28話` / `1000話`.
- Camera, phone and screen-recording names, bare numbers and bare dates are never looked up, unless you name them in
  [Fix a match](#fixing-a-match).
- **Franchise prefixes:** `Franchise Movie 15 - Subtitle` is retried by its subtitle, and a result is only accepted if it
  contains both the subtitle and the franchise name. The other way round, `Title A Franchise Mystery 2025` (from
  `Title: A Franchise Mystery`, which catalogs often list as just `Title`) is retried as `Title` when nothing else
  matches, only with the file's year, and shown as `Title: A Franchise Mystery` (also for `A ... Story`, `Movie`,
  `Film`, `Adventure`, `Tale`, `Saga`, `Legend`, `Odyssey`).

### Folder names
**Titles & art > Use the folder name when the file is just an episode number** (on by default). If a file name has only
an episode number (`S01E02.mkv`, `Episode 3.mkv`, `03.mkv`), the folder's name is used as the show, for example
`...\Show Name\Season 1\S01E02.mkv`. `Season N` folders are skipped (and supply the season for bare numbers); generic
folders like `TV` or `Downloads` are ignored. Folder names get the same cleanup (`Show.Name.2008.1080p`).

### Blu-ray and DVD
A disc's video files are named by number (`BDMV\STREAM\00062.m2ts`, `BDMV\PLAYLIST\00800.mpls`,
`VIDEO_TS\VTS_01_1.VOB`). With the folder-name switch above on, the title comes from:
1. the folder that holds `BDMV` or `VIDEO_TS` (`Sample Movie (2020)\BDMV\...`; a `Disc 1` folder is passed over);
2. for a disc played from a drive (`E:\BDMV\...`): the disc's own title, which many Blu-rays carry, or else its label
   (on macOS and Linux the disc's folder under `/Volumes` or `/media` is its label).

A camcorder's discs and cards (`AVCHD`) and labels that say nothing (`BDROM`, `DVD_VIDEO`) are left as they are.

### Fixing a match
Still no cover, or the wrong one? Use **Titles & art > Fix a match**: search under another name, or pin your own cover
image. Each line starts with the title as the program read it, which the log shows after `Playing:` (for a Blu-ray
file named `00062.m2ts` that is `00062`): `00062 = Sample Movie`. A line applies to every file read as that title.
The log also lists what each catalog returned. A free TMDB key (**Titles & art > More**) improves some matches.

## On your streams
When you share your screen, Discord names the stream after your *Playing* status and after no other kind, so
"Watching Sample Show" never reaches a stream. So while Discord shares your screen, and only then, the status is sent
as "Playing Sample Show", and viewers see "Streaming Sample Show".
- Discord gives a Playing status no progress bar, so it has a running clock and a text bar with the length
  (`▰▰▰▱▱▱ of 45:00`) that moves on as the video plays. The season and episode are in the first line, the rating and
  director next to the genres.
- It turns back into the usual status when the stream ends. Music stays "Listening to", and a hidden title stays
  hidden.
- **How it knows:** while it shares, the Discord app runs a piece of work that Windows lists by name, and the program
  only reads that list (and Windows' own record of which program captures the screen, where it can be read). This
  works on Windows 10 and 11.
- **Look > Show the title on my streams:** on Windows *While Discord shares my screen* (the default), *Always* or
  *Never*; the tray menu has a tick for it. On macOS and Linux the program cannot tell, so the choice is *Always* or
  *Never*.

If a stream still has no name: [Troubleshooting](troubleshooting.md#my-stream-doesnt-say-what-i-watch).

## Privacy
- **Hide what I'm watching** (Privacy tab, and the tray / menu bar menu) shows only "Watching a video" with the
  progress bar: no title, cover or buttons, and nothing looked up. An open window follows the menu.
- **Never show these files** takes folders, names or wildcards (such as `Private` or `*.xyz`). A listed file shows no
  status at all and is never looked up. For Plex, Jellyfin and Emby it also matches the file's path on the server and
  the Plex library's name, so one entry can hide a whole library. Hidden names are not written to the log either.
  [How entries match](config.md#privacy).
- **Services that may see your titles:** every online service the program can ask about a title has its own switch,
  with what it is used for and where requests go; a switched-off service is never contacted. One switch turns all
  lookups off.
- **Clear my status when paused for N minutes** takes the status down after a long pause (30 minutes by default; 0 =
  never) and brings it back when you play again or seek.
- **Clear cover cache** forgets everything that was looked up.
- **Recent connections** lists every address outside this computer the program sent a web request to since it started
  (players and the window on this computer are not counted), with how often and when last. It is kept in memory only,
  and never says what was asked. Every address the code can reach is in
  [SECURITY.md](../SECURITY.md#where-it-connects).
- **Your stats** (Help tab): how many videos were shown on Discord, how many were found in a catalog, how long they
  played and with which player. Numbers only, never titles, kept in `stats.json` on this computer; nothing is sent
  anywhere. **Reset stats** starts over. (The download count in the README is GitHub's public count of release
  downloads.)
- There is no analytics or tracking, and file paths are never sent.

## Players in detail

| Player | How it is read | Setup |
|---|---|---|
| MPC-HC | its web interface (`http://127.0.0.1:13579/variables.html`) | Options > Player > Web Interface > Listen on port, or the **Set up the player connection** button (it closes and reopens MPC-HC, and keeps a web interface it switches on to this PC only) |
| MPC-BE | the same web interface | the same; MPC-BE keeps the switch in `[WebServer]` of `mpc-be64.ini` or the registry, and the button handles both (also this PC only) |
| MPC-QT | its own mpv-style connection (`cmdrkotori.mpc-qt.mpv`), always on | none (its web interface works too, if you turned it on) |
| mpv | its JSON IPC (`input-ipc-server`) | `input-ipc-server=mpvsocket` in `mpv.conf`; the button adds it (`portable_config\mpv.conf` next to a portable mpv, else `%APPDATA%\mpv\mpv.conf`; on macOS and Linux `~/.config/mpv/mpv.conf` with the full path of the socket, such as `/tmp/mpvsocket`) and keeps a name you already set. mpv reads it when it starts. |
| IINA (macOS) | its mpv core's JSON IPC | IINA Settings > Advanced: enable advanced settings and add the mpv option `input-ipc-server` with a socket path. The button does both (it keeps a socket you already set), and IINA uses it once it is reopened. |
| VLC | its web interface (`http://127.0.0.1:8080/requests/status.json`, with a password) | See [VLC setup](#vlc-setup) below. On Linux VLC needs none of it: it is found through MPRIS. |
| Linux video players | MPRIS over D-Bus: VLC, Celluloid, Haruna, SMPlayer, GNOME Videos (Totem), Showtime, Clapper, Dragon Player, Kodi, Parole, QMPlay2, MPC-QT, and mpv with the mpv-mpris script | none; **Players > Find video players through MPRIS** switches it off. Music players and web browsers are left out. |
| PotPlayer (Windows) | the window messages PotPlayer documents for remote control (play state, position, length, speed, and the file's path) | none |
| Kodi | its JSON-RPC web server (`http://127.0.0.1:8080/jsonrpc`, with a user name and password); a Kodi on another device works with its own address. Library episodes and movies are named from Kodi's own information. | In Kodi: Settings > Services > Control > Allow remote control via HTTP, with a user name and password; enter them on the **Players** tab. An empty password means Kodi is not asked. On Linux, Kodi is also found through MPRIS without this. |
| Plex | your Plex Media Server's play notifications, for whatever you play with your Plex account: the Plex app on this computer, Plex Web, a TV, a phone | **Players > Sign in to Plex**; see [Plex](#plex) below |
| Stremio | your Stremio account's library, which Stremio's player updates while you watch: the desktop app, Stremio Web, a TV, a phone | **Players > Sign in to Stremio**; see [Stremio](#stremio) below |
| Jellyfin, Emby | your own server's list of sessions, for whatever you play on it: any Jellyfin or Emby app, the web player, a TV, a phone | **Players > Jellyfin / Emby**; see [Jellyfin and Emby](#jellyfin-and-emby) below |

**Which player shows.** The program shows the first player that plays a video; if none does, the first that plays
music; then the first that is paused (so a paused player left open does not hide another that plays); if none is, the
window still names the first that answers. It asks them in this order:
1. MPC-HC, MPC-BE and MPC-QT (their web interface), then MPC-QT's own connection
2. mpv, then IINA (macOS)
3. VLC (only once its password is set)
4. the MPRIS players (Linux)
5. PotPlayer (Windows)
6. Kodi (only once its password is set)
7. Plex, then Stremio, and last Jellyfin / Emby, each only while you are signed in

**How often.** Players are checked every 5 seconds (**Advanced > Check every**). mpv, IINA, Kodi and the Linux players
read through MPRIS also tell the program when they pause, resume, seek or change file, so that shows on Discord at once
(at most one early update every 4 seconds: Discord takes about five in 20 seconds). Kodi sends these notifications on
port 9090 of the same address ("Allow remote control from applications on this system", or "on other systems" for a
Kodi elsewhere, both under Settings > Services > Control); without them it is checked as the others are.

**Names and streams.** With players outside the MPC family, the image tooltip names the player ("mpv", "IINA", "VLC
media player", ...) instead of "Media Player Classic". Streams show their title (there is no file name or folder).

What was tried with which player, and how, is in [What has been tried](#what-has-been-tried).

### VLC setup
- **By hand:** Tools > Preferences > All > Interface > Main interfaces: tick Web, and set a password under Lua > Lua
  HTTP. Copy it to **Players > VLC web interface password**.
- **With the button:** it switches the interface on in `vlcrc` (`portable\vlcrc` next to a portable VLC, else
  `%APPDATA%\vlc\vlcrc`; on macOS `~/Library/Preferences/org.videolan.vlc/vlcrc`), gives it a password and fills it in
  for you. It keeps a password and port you already set.
- An interface the button switches on only answers this PC (`http-host=127.0.0.1`; VLC also uses that address for
  streams sent over HTTP without one).
- VLC rewrites `vlcrc` when it exits, so a running VLC is closed and reopened if something has to change.

### Plex
- **Signing in:** **Players > Sign in to Plex** opens plex.tv in your browser, and you approve the sign-in there (the
  program never sees your password). The program introduces itself to Plex as "MPCvibedRPC" and keeps the sign-in in
  `config.json`; **Sign out** or **Restore defaults** asks plex.tv to end the sign-in, then removes it from this
  computer (also when plex.tv does not answer). The program may stay listed on Plex's *Authorized Devices* page,
  where its entry can be deleted.
- **Your server:** it follows your server; choose another one under the button (your own servers come first, then
  those shared with you). **Players > Plex server address** is only needed when this computer cannot reach the server
  at the addresses plex.tv lists.
- **What it reads:** only titles, episode numbers and the position of what you play, on any of your devices. On a
  server you own, only your own playback shows, not that of people you share with. Music and photos are never shown.
- The episode or movie name comes from your server; cover art is looked up as for files.
- What was tried: see [What has been tried](#what-has-been-tried).

### Stremio
- **Signing in:** **Players > Sign in to Stremio** works the way Stremio's TV apps do: stremio.com opens in your
  browser and you approve a code there (the program never sees your password). The account's key is kept in
  `config.json`; **Sign out** (or **Restore defaults**) ends that session on Stremio's side and removes it.
- **What it reads:** Stremio has no live "now playing" for other apps, so the program reads the account's library:
  every 20 seconds it asks which entries changed, and fetches only those.
- Stremio's player updates the entry of what you watch when it starts, pauses, resumes and stops, and every 180 seconds
  while it plays (90 seconds in older Stremio apps; the program learns which). An update that finds the video where it
  was is a start or a resume; one that comes early, with the video moved on, is a pause or a stop, and the status is
  taken down. When the regular updates stop coming, the video stopped without a word.
- Series episodes come from the entry's video ("Sample Show - S01E02"); cover art is looked up as for files. Entries of
  type "other" (which Stremio does not save to the account) are never shown.
- What was tried: see [What has been tried](#what-has-been-tried).

### Jellyfin and Emby
- **Address:** type your server's address on the **Players** tab (`http://` is added if missing, and a path is kept for
  a reverse proxy). The program asks the server whether it is Jellyfin or Emby and names the player that way.
- **Jellyfin signs in with Quick Connect:** the window shows a 6-digit code; enter it in a Jellyfin app or web page
  where you are already signed in (profile > Quick Connect). The program waits up to 10 minutes.
- **Use a password instead** is the way for Emby (no Quick Connect) or a Jellyfin server with Quick Connect off. The
  password goes to your own server once and is never stored.
- **What is kept:** only the server's access token, in `config.json` as plain text like other apps keep theirs; the
  window and **Copy diagnostics** never show it. **Sign out** and **Restore defaults** end the session on the server
  and forget the sign-in.
- **What it reads:** the server's session list, every 5 seconds while you are signed in. Only your own sessions count
  (an administrator would see everyone's); music, audiobooks and photos are skipped; a playing session beats a paused
  one. A playing session that has not reported for 5 minutes (its player went away) no longer counts. The position is
  moved on by how long ago the player last reported, by the server's clock (Emby does not say when, so there the time
  runs on from when a report is first seen).
- Titles are written like a file name ("Sample Show - S01E02 - Title", "Sample Movie (2020)"), so the usual parsing,
  cover art and episode lookups apply. Problems (the server refused the sign-in, or can't be reached) show as a hint in
  the window.
- What was tried: see [What has been tried](#what-has-been-tried).

## Resting while Discord is closed
While Discord is not running and the settings window is closed, the program rests: no player is asked, the Plex server,
the Stremio account and the Jellyfin / Emby server are not followed, and memory the program does not need is handed
back to the system. At each check (every 5 seconds by default) it only looks whether Discord's connection is there; it
never connects to see. Once Discord starts, everything picks up at the next check; opening the window does so at once.
Resting needs no setting: with Discord closed there is nobody to show a status to.

## What has been tried
"Real" means the real program with the real player or service on the system named; "CI" means GitHub's test machines,
on every change, with a stand-in for Discord (and a stand-in player for the media servers); "not yet" means nobody has
tried it.

The list of what has only run against fakes, or not at all, is in [Not yet tried for real](development.md#not-yet-tried-for-real).

| Player or service | System | How |
|---|---|---|
| MPC-HC 2.8.3 | Windows 11 | real (the player the program was built around) |
| MPC-BE 1.9.1, MPC-QT 26.07, mpv 0.41.0, VLC 3.0.24 | Windows 11 | real, set up as described above (also a development build of VLC 4, not released yet) |
| PotPlayer, Kodi | Windows | CI: the real players from Chocolatey with a generated test video |
| Kodi | macOS, Linux, another device | only against a stand-in |
| mpv, VLC, mpv-mpris, Celluloid | Linux | CI; a real desktop: not yet |
| Blu-ray and DVD titles | any | made-up disc folders in tests; a real Blu-ray disc was recognised once in a real test |
| mpv, IINA | macOS | CI; IINA on a real Mac with the real Discord: once, by a tester |
| Plex | any | CI: the official Plex Media Server without a Plex account, with a stand-in player; a real Plex account, shared servers and real Plex apps: not yet |
| Stremio | any | against a stand-in for Stremio's servers built from Stremio's open-source core; a real Stremio account: not yet |
| Jellyfin, Emby | any | unit tests against stand-ins, and CI against the official server images with a stand-in player; a real server and the real apps: not yet |

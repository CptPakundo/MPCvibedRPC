# Settings reference

Every setting with its default and what it does, grouped by the settings window's tabs. The window covers the main
ones and saves them as you change them. The others go in `config.json`:
- Windows: `%LOCALAPPDATA%\MPCvibedRPC\config.json`
- macOS: `~/Library/Application Support/MPCvibedRPC/config.json`
- Linux: `~/.local/share/MPCvibedRPC/config.json`

`config.json` is JSON: use the names below and write values as the Default column does, for example
`{"artworkSources": ["imdb", "tvmaze"]}`. The settings of the General tab go inside `"app"`:
`{"app": {"checkUpdates": false}}`. Anything missing falls back to its default. Settings marked *(config.json only)*
are not in the window.

## General

These live under `"app"` in `config.json`. The update switches and **Check now** are under **General > Updates**.

| Setting | Default | What it does |
|---|---|---|
| `autoStart` | `false` | **Start with Windows** (macOS and Linux: at login): start straight to the tray (macOS and Linux: in the background). The real switch is the login item itself: the registry's Run key on Windows, a LaunchAgent on macOS, an entry in `~/.config/autostart` on Linux. |
| `openWindow` | `true` | **Show this window when I open the app.** `false` goes straight to the tray (macOS and Linux: the background), as at login. |
| `minimizeToTray` | `true` | **Minimize to the tray** (Windows): minimizing the window closes it, and the program stays in the tray. Off: it minimizes to the taskbar. Either way, the window's X quits the program (on macOS it closes to the menu bar). |
| `startPresence` | `true` | **Show on Discord when the app opens.** Otherwise it waits until you press Show on Discord. |
| `checkUpdates` | `true` | **Check for updates automatically:** as the program starts, then every six hours ([how updates work](troubleshooting.md#updates)). |
| `autoUpdate` | `true` | **Install updates automatically** (Windows and Linux, with `checkUpdates` on): a found update installs itself once nothing is playing and the window is closed. Off: the window and a tray notice offer it, and **Install** installs it. |
| `updateRepo` | `"CptPakundo/MPCvibedRPC"` | *(config.json only)* The GitHub repository (`owner/repo`) that publishes releases. A release is only installed when it carries a `.sha256` checksum file for the download; otherwise the window links to its page. |

## Players

| Setting | Default | What it does |
|---|---|---|
| `port` | `13579` | Web interface port of MPC-HC, MPC-BE and MPC-QT (MPC-QT also works without its web interface). |
| `mpvPipe` | `"mpvsocket"` | The `input-ipc-server` name mpv listens on. Empty: don't look for mpv. [Details](#pipe-names). |
| `vlcPassword` | `""` | The password of VLC's web interface (`http-password` in `vlcrc`). Empty: don't look for VLC. **Set up the player connection** fills it in. Kept as plain text, as VLC keeps it. |
| `vlcPort` | `8080` | The port of VLC's web interface (`http-port`). |
| `kodiAddress` | `"127.0.0.1:8080"` | Kodi's web server (host:port, or a full `http://` address), on this computer or another device. Left out of Copy diagnostics. |
| `kodiUser` | `"kodi"` | The user name set in Kodi (Settings > Services > Control). |
| `kodiPassword` | `""` | The password set in Kodi. Empty: don't look for Kodi. Kept as plain text, like `vlcPassword`. |
| `iinaPipe` | `"iina-mpvsocket"` | macOS only: the `input-ipc-server` socket in IINA's mpv options, named like `mpvPipe`. **Set up the player connection** adds it to IINA. Empty: don't look for IINA. |
| `mpris` | `true` | Linux only: **Find video players through MPRIS** (D-Bus): VLC, Celluloid, Haruna, SMPlayer, GNOME Videos, Clapper, mpv with mpv-mpris and others. Music players and browsers are never shown. |
| `plexAddress` | `""` | A fixed address for your Plex server, such as `"http://192.168.1.20:32400"`. Empty: the addresses plex.tv lists (your network first, then the others, then Plex's relay). |
| `jellyfinAddress` | `""` | Your Jellyfin or Emby server, such as `"http://192.168.1.20:8096"`: typed in the **Jellyfin / Emby** sign-in and saved by it (`http://` is added if missing, a path is kept for a reverse proxy). **Sign out** removes it. |

The sign-ins themselves (Plex, Stremio, Jellyfin / Emby) write the values in [Written by the program](#written-by-the-program).

### Pipe names
A name such as `mpvsocket` is the pipe `\\.\pipe\mpvsocket` on Windows, and a socket of that name in the temporary
folder on macOS and Linux. A full path or `~/...` is used as it is. See
[Players in detail](how-it-works.md#players-in-detail).

## Look

| Setting | Default | What it does |
|---|---|---|
| `activityType` | `"watching"` | **Style.** `"watching"`: "Watching ..." with a real progress bar. `"playing"`: the classic "Playing ..." with an elapsed-time counter. |
| `titleAsName` | `true` | **Show the title as the activity name:** "Watching <title>" instead of "Watching Media Player Classic". |
| `streamTitle` | `"auto"` | **Show the title on my streams:** `"auto"` while Discord shares your screen, `"always"`, or `"off"`. [Details](#streamtitle). |
| `textProgressBar` | `true` | **Text progress bar while paused or streaming:** `▰▰▰▱▱▱ 12:34 / 45:00` while paused; `▰▰▰▱▱▱ of 45:00` (and the speed when it is not 1x) in the stream form. Sent again each time the bar fills another cell. |
| `progressWidth` | `12` | *(config.json only)* Width of that text bar (0 to 40). |
| `showRemainingTime` | `false` | *(config.json only)* Counts down instead of up. Only used with `"playing"` or in the stream form. |
| `richInfo` | `true` | **Genres, rating and director** on the extra lines. `false` leaves just the year or episode. |
| `linkButton` | `true` | **Link to the title's page** (IMDb, AniList, ...): a "View on ..." button, clickable text lines and cover, a second button when there is another page, and the player's logo linking its site. If Discord refuses these links, they are left out from then on. |
| `letterboxdLink` | `false` | **Link to Letterboxd when it has the title:** with `linkButton` on, the button and lines open the Letterboxd page instead. Asks letterboxd.com once per title (by its IMDb id, or a film's TMDB id); the answer is kept with the cover. |
| `episodeInDetails` | `false` | **Also show S01E02 in the title line** (Discord already shows a badge when the card is expanded). |

### streamTitle
- `"auto"`: the stream form ("Playing <title>", so viewers see "Streaming <title>") only while Discord shares your
  screen. Only Windows can tell; on macOS and Linux `"auto"` never changes the status.
- `"always"`: the stream form all the time. `"off"`: never.

What the stream form looks like and how a share is detected: [On your streams](how-it-works.md#on-your-streams).

## Titles & art

The title read from the file name is looked up online. In the window, **Fix a match** holds `artworkAliases` and
`artworkOverrides`, and **More** holds `tmdbApiKey` and `folderEpisodeNumbers`.

| Setting | Default | What it does |
|---|---|---|
| `episodeTitles` | `true` | **Look up episode titles** when the file doesn't have one (`Ep03` becomes `S01E03 · Episode title`). |
| `catalogTitle` | `true` | **Use the catalog's spelling of titles**, including punctuation a file name can't hold. `false` keeps the title as read from the file name. |
| `animeTitles` | `"auto"` | **Anime titles**, for anime found on AniList: `"auto"` uses the AniList name (English or romaji) the file name resembles most; `"english"` or `"romaji"` always use that one; `"file"` keeps the file's spelling. |
| `useFolderName` | `true` | **Use the folder name when the file is just an episode number** (skipping `Season 1` style folders). |
| `artworkAliases` | `{}` | **Search under another name**, for example `{"filename title": "Catalog Title"}`. |
| `artworkOverrides` | `{}` | **Pin a cover image**, for example `{"some title": "https://example.com/poster.jpg"}`. |
| `tmdbApiKey` | `""` | **TMDB API key**, free at <https://www.themoviedb.org/settings/api>. TMDB is skipped unless this is set. |
| `folderEpisodeNumbers` | `false` | **Number episodes by their place in the season folder**, for files with a running production code, instead of the catalog's count. |
| `artworkSources` | `["tmdb", "imdb", "cinemeta", "tvmaze", "anilist", "kitsu", "wikipedia"]` | *(config.json only)* Cover sources, asked at the same time; the first in this list with an exact title match wins. |
| `animeSources` | `["anilist", "kitsu"]` | *(config.json only)* Anime-style file names ask these first, in this order. |
| `episodeSources` | `["bulbapedia", "tmdb", "tvmaze", "cinemeta", "kitsu"]` | *(config.json only)* Episode-title sources, in order. A `jikan` you add goes first for anime, since MyAnimeList numbers episodes the way anime files do. `tmdb` needs `tmdbApiKey`. |
| `artworkWaitMs` | `1500` | *(config.json only)* How long to hold the status back waiting for a cover. It refreshes when the cover arrives. |

Sources:

| Name | Covers |
|---|---|
| `tmdb` | TMDB (best data; skipped unless `tmdbApiKey` is set). |
| `cinemeta` | IMDb-based Stremio catalog. |
| `imdb` | IMDb's own search (finds titles Cinemeta misses). |
| `tvmaze` | TV shows worldwide. |
| `anilist` | Anime (knows romaji, English and Japanese titles); asked first for fansub-style names. |
| `kitsu` | Another anime database, used if AniList is down or misses. |
| `jikan` | MyAnimeList through Jikan. Not in the default lists, because the public Jikan API (api.jikan.moe) closed on 2026-10-01. A Jikan you host yourself still works: set `jikanBase` and add `jikan` to the lists. |
| `wikipedia` | Last resort: the article's lead image (Hebrew titles use he.wikipedia.org). |
| `bulbapedia` | Episode titles only, from its episode lists, for files named with a production code. |

If a title still has no cover, the log says what each source returned. Then add an alias or pin an image.

## Privacy

| Setting | Default | What it does |
|---|---|---|
| `hideTitle` | `false` | **Hide what I'm watching:** only "Watching a video" with the progress bar; no title, cover, episode or buttons, nothing looked up, no title in the log. Also in the tray and menu bar menu. |
| `hideFiles` | `[]` | **Never show these files:** a listed file shows no status and is not looked up. [How entries match](#hidefiles). |
| `pauseClearMinutes` | `30` | **Clear my status when paused for (minutes)** (0 = never). It comes back when you play again, seek, or turn this off. |
| `showArtwork` | `true` | **Look up cover art and titles online.** `false` stays completely offline (also no episode-title lookups). |
| `disabledSources` | `[]` | **Services that may see your titles:** the services that must never be contacted, whatever the source lists say. Ids: `tmdb`, `imdb`, `cinemeta`, `tvmaze`, `anilist`, `kitsu`, `jikan` (MyAnimeList), `wikipedia`, `bulbapedia`. The window shows a switch for each. |

### hideFiles
- Entries are matched without regard to case; `/` and `\` are alike.
- A plain entry hides a file whose whole path (folders and name) contains it: `Private`, `D:\Videos\Private`.
- An entry with `*` (any run of characters) or `?` (one character) must match the whole file name or the whole path:
  `*.xyz`.
- For Plex, Stremio, Jellyfin and Emby an entry is also matched against the title as "Show - S01E02 - Episode" or
  "Movie (2020)". For Plex, Jellyfin and Emby it is also matched against the file's path on the server (when the
  server gives it) and the Plex library's name: `Kids` hides a whole library called Kids.
- At most 100 entries of 300 characters. Entries are only matched, never shown or logged.

## Advanced

| Setting | Default | What it does |
|---|---|---|
| `clientId` | `"427863248734388224"` | **Discord application ID** (it holds the `mpc-hc` images below). For your own name and artwork, create an app at <https://discord.com/developers/applications> and paste its Application ID here. |
| `pollInterval` | `5000` | **Check every (ms).** The status is only sent when something changes. mpv, IINA, Kodi and MPRIS players also report a pause, seek or new file at once. While Discord and the window are closed, a check only looks whether Discord has started ([Resting](how-it-works.md#resting-while-discord-is-closed)). |
| `plexJwt` | `false` | *(config.json only)* Sign in to Plex with its newer tokens: they last 7 days and are renewed in time with a key of this install's own (`plexKey`), as Plex's developer documentation describes. Off: the long-lived token Plex still accepts. Takes effect at the next **Sign in to Plex**. |

**Restore defaults** and **Open settings folder** are under **Advanced > Maintenance**.

### Discord art assets
*(config.json only.)* These are uploaded to the Discord application whose `clientId` you use.

| Setting | Default | What it does |
|---|---|---|
| `largeImageKey` | `"mpc-hc"` | Shown when no cover is found. While it is the default, players with a logo in `docs/players` (mpv, VLC, IINA, PotPlayer, Kodi, SMPlayer, Plex, Stremio, Jellyfin, Emby) show that logo instead. |
| `smallImageKey` | `"mpc-hc"` | Small corner icon when a cover is shown. While it is the default, players outside the MPC family show their own logo there, or none. |
| `appName` | `"Media Player Classic"` | Name used when `titleAsName` is off, and the text of the images. While it is the default, other players are named as themselves (mpv, VLC media player, Plex, ...). |

### File-name cleanup
*(config.json only.)* Smart mode (the default) strips release junk (BluRay, 1080p, x265, HDR, audio tags, release
groups) and finds titles, seasons and episodes. Basic mode only applies the switches below.

| Setting | Default | What it does |
|---|---|---|
| `smartFormat` | `true` | Smart mode on or off. |
| `ignoreBrackets` | `true` | Hide `[Group]` style tags. |
| `replaceUnderscore` | `true` | Basic mode only (smart mode always does this). |
| `replaceDots` | `true` | Basic mode only (smart mode always does this). |

## Written by the program

Not typed: the sign-ins on the **Players** tab and the window write these. The sign-ins are kept in `config.json` as
plain text, as other apps keep theirs; the window and **Copy diagnostics** never show them (diagnostics only say
whether you are signed in).

| Setting | Default | What it does |
|---|---|---|
| `plexToken`, `plexAccount`, `plexUser`, `plexServer`, `plexServerName`, `plexClient` | `""` / `0` | **Sign in to Plex:** the account's sign-in, its id and name, the server followed (its machine identifier and name) and this install's identifier towards Plex. Empty `plexToken`: Plex is not followed and plex.tv is never contacted. **Sign out** removes all but `plexClient`. |
| `plexKey` | `""` | The `plexJwt` sign-in: this install's private signing key for Plex. A secret, like `plexToken`; removed by **Sign out**. |
| `stremioKey` | `""` | **Sign in to Stremio:** the key of the account's session. Empty: Stremio is not followed and its servers are never contacted. **Sign out** ends the session on Stremio's side and removes it. |
| `jellyfinToken`, `jellyfinUser`, `jellyfinUserName`, `jellyfinServerName`, `jellyfinProduct`, `jellyfinDevice` | `""` | **Jellyfin / Emby** sign-in: the server's access token, the user's id and name, the server's name, whether it is Jellyfin or Emby, and this install's device id (made at the first sign-in, kept after signing out). Empty `jellyfinToken`: the server is never contacted. **Sign out** and **Restore defaults** end the session on the server and remove the sign-in (`jellyfinDevice` is kept). A password is sent to your own server once and never stored. |
| `seenVersion` | (none) | Under `"app"`: the version whose changes the window last showed; a different version (an update, a new install) shows its changes once. **Restore defaults** keeps it. |
| `welcomeSeen` | `false` | Under `"app"`: whether the first-run welcome card was closed. **Restore defaults** shows the card again. |

## For testing

These point the online lookups somewhere else (a test server or a mirror), or are read only by tests. They are not in
the window, and a service switched off on the Privacy tab stays off whatever address it has here.

| Setting | Default |
|---|---|
| `cinemetaBase` | `"https://v3-cinemeta.strem.io"` |
| `imdbSuggestBase` | `"https://v3.sg.media-imdb.com"` |
| `tvmazeBase` | `"https://api.tvmaze.com"` |
| `anilistUrl` | `"https://graphql.anilist.co"` |
| `kitsuBase` | `"https://kitsu.io/api/edge"` |
| `jikanBase` | `"https://api.jikan.moe/v4"` (the closed public API; point it at your own Jikan) |
| `wikipediaBase` | `"https://{lang}.wikipedia.org"` |
| `letterboxdBase` | `"https://letterboxd.com"` |
| `bulbapediaBase` | `"https://bulbapedia.bulbagarden.net"` |
| `requestGapMs` | `{"graphql.anilist.co": 250, "api.jikan.moe": 400}` (the least time between two requests to a host, in milliseconds) |
| `ignoreFiletype` | `true` (only the recorded parsing tests read it; a title never carries the file's extension either way) |

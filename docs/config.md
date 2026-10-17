# Settings reference

Every setting with its default and what it does. The settings window covers the main ones and saves them as you
change them. For the rest, add them to `config.json` (in `%LOCALAPPDATA%\MPCvibedRPC`; macOS:
`~/Library/Application Support/MPCvibedRPC`; Linux: `~/.local/share/MPCvibedRPC`) using the same names, for
example `{"artworkSources": ["imdb", "tvmaze"]}`. Anything missing from `config.json` falls back to the default
shown here.

## Connection

| Setting | Default | What it does |
|---|---|---|
| `clientId` | `427863248734388224` | Discord application ID (it holds the `mpc-hc` images below). For your own name and artwork, create an app at <https://discord.com/developers/applications> and paste its Application ID here. |
| `port` | `13579` | Web interface port of MPC-HC, MPC-BE and MPC-QT (MPC-QT also works without its web interface). |
| `mpvPipe` | `'mpvsocket'` | The `input-ipc-server` name mpv listens on (set in `mpv.conf`; a name such as `mpvsocket` is the pipe `\\.\pipe\mpvsocket`, on macOS and Linux a socket of that name in the temporary folder; a full path or `~/...` is used as it is). Empty: don't look for mpv. See [Players in detail](how-it-works.md#players-in-detail). |
| `iinaPipe` | `'iina-mpvsocket'` | macOS only: the `input-ipc-server` socket in IINA's mpv options (IINA Settings > Advanced), named like `mpvPipe`. **Set up the player connection** adds it to IINA. Empty: don't look for IINA. |
| `mpris` | `true` | Linux only: find video players through MPRIS (D-Bus): VLC, Celluloid, Haruna, SMPlayer, GNOME Videos, Clapper, mpv with mpv-mpris and others. Music players and browsers are never shown. They are asked after the other players on this computer, before Plex, Stremio and Jellyfin / Emby. |
| `vlcPassword` | `''` | The password of VLC's web interface (`http-password` in VLC's `vlcrc`). Empty: don't look for VLC. **Set up the player connection** fills it in. Kept in `config.json` as plain text, like VLC keeps it in `vlcrc`. |
| `vlcPort` | `8080` | The port of VLC's web interface (`http-port`). |
| `kodiAddress` | `'127.0.0.1:8080'` | Kodi's web server (host:port, or a full `http://` address) for a Kodi on this computer or on another device. Left out of Copy diagnostics. |
| `kodiUser` | `'kodi'` | The user name set in Kodi (Settings > Services > Control). |
| `kodiPassword` | `''` | The password set in Kodi. Empty: don't look for Kodi. Kept in `config.json` as plain text, like `vlcPassword`. |
| `plexAddress` | `''` | A fixed address for your Plex server, such as `http://192.168.1.20:32400`. Empty: use the addresses plex.tv lists for it (on your network first, then the others, then Plex's relay). |
| `plexToken`, `plexAccount`, `plexUser`, `plexServer`, `plexServerName`, `plexClient` | `''` / `0` | Written by **Sign in to Plex** (Accounts tab), not typed: the account's sign-in, its id and name, the server followed (its machine identifier and name) and this install's identifier towards Plex. Empty `plexToken`: Plex is not followed and plex.tv is never contacted. **Sign out** removes all but `plexClient`. The sign-in is kept in `config.json` as plain text, like other Plex apps keep theirs; the settings window and **Copy diagnostics** never show it. |
| `stremioKey` | `''` | Written by **Sign in to Stremio** (Accounts tab), not typed: the key of the account's session. Empty: Stremio is not followed and its servers are never contacted. **Sign out** ends the session on Stremio's side and removes it. Kept in `config.json` as plain text, like Stremio keeps its own; the settings window and **Copy diagnostics** never show it. |
| `jellyfinAddress` | `''` | The address of your Jellyfin or Emby server, such as `http://192.168.1.20:8096` (typed in the Jellyfin / Emby sign-in on the Accounts tab and saved by it; `http://` is added if missing, and a path is kept for a reverse proxy; **Sign out** removes it). |
| `jellyfinToken`, `jellyfinUser`, `jellyfinUserName`, `jellyfinServerName`, `jellyfinProduct`, `jellyfinDevice` | `''` | Written by the **Jellyfin / Emby** sign-in (Accounts tab), not typed: the server's access token, the user's id and name, the server's name, whether it is Jellyfin or Emby, and this install's device id (made at the first sign-in, kept after signing out). Empty `jellyfinToken`: Jellyfin / Emby is not followed and the server is never contacted. **Sign out** ends the session on the server and removes the sign-in (`jellyfinDevice` is kept); **Restore defaults** ends it too. The token is kept in `config.json` as plain text, like other apps keep theirs; a password is sent to your own server once and never stored. The settings window and **Copy diagnostics** never show these (diagnostics only say whether you are signed in). |
| `pollInterval` | `5000` | Milliseconds between checks. Presence is only sent when something changes. mpv and IINA also report a pause, seek or new file at once, which brings the next check forward. While Discord is closed and the window is too, a check only looks whether Discord has started (see [Resting](how-it-works.md#resting-while-discord-is-closed)). |

## Look tab

| Setting | Default | What it does |
|---|---|---|
| `activityType` | `'watching'` | `'watching'` is "Watching ..." with a real progress bar (elapsed and total time). `'playing'` is the classic "Playing ..." with an elapsed-time counter. |
| `titleAsName` | `true` | Shows "Watching <title>" instead of "Watching Media Player Classic". |
| `textProgressBar` | `true` | Shows a text bar (`▰▰▰▱▱▱ 12:34 / 45:00`) while paused, because Discord can't freeze its own bar. |
| `progressWidth` | `12` | Width of that text bar (0 to 40). |
| `showRemainingTime` | `false` | Only used when `activityType` is `'playing'`. |
| `episodeInDetails` | `false` | `true` repeats the season and episode number in the title line too (Discord already shows a badge when the card is expanded). |
| `richInfo` | `true` | Genres, IMDb rating and director on the extra lines. `false` leaves just the year or episode. |
| `linkButton` | `true` | Link to the title's page (IMDb, AniList, ...): adds a "View on IMDb" style button and makes both text lines and the cover clickable (others can click them on your profile; Discord doesn't let you click your own). If Discord refuses these links, they are left out from then on. |

## Privacy

These are all on the **Privacy** tab of the settings window.

| Setting | Default | What it does |
|---|---|---|
| `hideTitle` | `false` | Show only "Watching a video" with the progress bar: no title, cover, episode or buttons, and nothing is looked up online. The log does not record the title either. Also a tick item, **Hide what I'm watching**, in the Windows tray menu and the macOS menu bar menu (not on Linux, which has no tray). |
| `hideFiles` | `[]` | Files that are never shown. Each entry is matched without regard to case; `/` and `\` are alike. A plain entry hides a file whose whole path (folders and name) contains it, for example `Private` or `D:\Videos\Private`. An entry with `*` (any run of characters) or `?` (one character) must match the whole file name or the whole path, for example `*.xyz`. A hidden file shows no status at all and is not looked up. At most 100 entries of 300 characters. For Plex, Stremio, Jellyfin and Emby an entry is also matched against the title as "Show - S01E02 - Episode" or "Movie (2020)", and for Plex, Jellyfin and Emby against the file's path on the server (when the server gives it) and the Plex library's name: `Kids` hides a whole library called Kids. These are only matched, never shown or logged. |
| `showArtwork` | `true` | Look up cover art and titles online. `false` stays completely offline (it also turns off episode-title lookups). |
| `disabledSources` | `[]` | Online services that must never be contacted, whatever the source lists below say. Ids: `tmdb`, `imdb`, `cinemeta`, `tvmaze`, `anilist`, `kitsu`, `jikan` (MyAnimeList), `wikipedia`, `bulbapedia`. The window shows a switch for each; the stored list holds the ones that are off. |
| `pauseClearMinutes` | `30` | Take the status down after the video has been paused this many minutes (0 = never). It comes back when you play again, seek, or turn this off. |

## Filename cleanup

Smart mode (`smartFormat: true`, the default) strips release junk and finds titles, seasons and episodes. It
understands release names: BluRay, 1080p, x265, HDR, audio tags and release groups. Basic mode (`smartFormat: false`)
only applies the switches below.

| Setting | Default | What it does |
|---|---|---|
| `smartFormat` | `true` | Smart mode on or off. |
| `ignoreBrackets` | `true` | Hide `[Group]` style tags. |
| `replaceUnderscore` | `true` | Basic mode only (smart mode always does this). |
| `replaceDots` | `true` | Basic mode only (smart mode always does this). |

## Titles & art tab

The title parsed from the filename is looked up online (IMDb-based Cinemeta by default, no key needed). Set
`showArtwork` to `false` to stay fully offline. In the window, **Fix a match** holds `artworkAliases` and
`artworkOverrides` (search under another name, pin a cover image), and **More** holds `tmdbApiKey` and
`folderEpisodeNumbers` (labelled "Number episodes by their place in the season folder").

| Setting | Default | What it does |
|---|---|---|
| `artworkSources` | `['tmdb', 'imdb', 'cinemeta', 'tvmaze', 'anilist', 'kitsu', 'wikipedia']` | Sources are asked at the same time; the first in this list with an exact title match wins. See the source list below. |
| `animeSources` | `['anilist', 'kitsu']` | Anime-style filenames ask these first, in this order. |
| `tmdbApiKey` | `''` | TMDB API key. Free at <https://www.themoviedb.org/settings/api>. TMDB is skipped unless this is set. |
| `artworkAliases` | `{}` | Search under another name, for example `{ 'filename title': 'Catalog Title' }`. |
| `artworkOverrides` | `{}` | Pin an image URL, for example `{ 'some title': 'https://example.com/poster.jpg' }`. |
| `artworkWaitMs` | `1500` | How long to hold the presence back waiting for a cover. It refreshes when the cover arrives. |
| `useFolderName` | `true` | If the file is just an episode number, use the folder's name as the show (skipping `Season 1` style folders). |
| `episodeTitles` | `true` | Look up the real episode name when the file doesn't have one (`Ep03` becomes `S01E03 · Episode title`). |
| `episodeSources` | `['bulbapedia', 'tmdb', 'tvmaze', 'cinemeta', 'kitsu']` | Order tried. A `jikan` you add (a self-hosted Jikan, see `jikanBase`) goes first for anime, since MyAnimeList numbers episodes the way anime files do. `tmdb` needs `tmdbApiKey`; the others need no key. |
| `folderEpisodeNumbers` | `false` | `true` numbers files with a running production code by their place in the season folder instead of the catalog's count. |
| `animeTitles` | `'auto'` | For anime matched on AniList: `'auto'` uses whichever AniList name (English or romaji) the filename resembles most; `'english'` or `'romaji'` always use that one; `'file'` keeps the filename's spelling. |
| `catalogTitle` | `true` | Show a recognised title as the catalog spells it, including punctuation a file name can't hold. `false` keeps it as parsed from the filename. |

Sources:

| Name | Covers |
|---|---|
| `tmdb` | TMDB (best data; skipped unless `tmdbApiKey` is set). |
| `cinemeta` | IMDb-based Stremio catalog. |
| `imdb` | IMDb's own search (finds titles Cinemeta misses). |
| `tvmaze` | TV shows worldwide. |
| `anilist` | Anime (knows romaji, English and Japanese titles); asked first for fansub-style names. |
| `kitsu` | Another anime database, used if AniList is down or misses. |
| `jikan` | MyAnimeList through Jikan. Not in any default list: the public Jikan API (api.jikan.moe) closed on 2026-10-01. To use a Jikan you host yourself, set `jikanBase` and add `jikan` to the lists. |
| `wikipedia` | Last resort: the article's lead image (Hebrew titles use he.wikipedia.org). |
| `bulbapedia` | Episode titles only, from its episode lists, for files named with a production code. |

If a title still has no cover, the log says what each source returned. Then either add an alias or pin an image.

## Discord art assets

These are uploaded to the Discord application whose `clientId` you use.

| Setting | Default | What it does |
|---|---|---|
| `largeImageKey` | `'mpc-hc'` | Shown when no cover is found. While it is the default, players with a logo in `docs/players` (mpv, VLC, IINA, PotPlayer, Kodi, SMPlayer, Plex, Stremio, Jellyfin, Emby) show that logo instead. |
| `smallImageKey` | `'mpc-hc'` | Small corner icon when a cover is shown. While it is the default, players outside the MPC family show their own logo there (see `largeImageKey`), or none. |
| `appName` | `'Media Player Classic'` | Name used when `titleAsName` is off, and the text of the images. While it is the default, other players are named as themselves (mpv, VLC media player, Plex, ...). |

## Program settings

These belong to the program itself and live under `app` in `config.json`. All but `updateRepo` are in the settings window.
**Restore defaults** and **Open settings folder** are under **Advanced > Maintenance**; the update switch and **Check now** are under **General > Updates**.

| Setting | Default | What it does |
|---|---|---|
| `autoStart` | `false` | Start when you log in, straight to the tray (macOS and Linux: in the background). The real switch is the login item itself: the registry's Run key on Windows, a LaunchAgent on macOS, an autostart entry in `~/.config/autostart` on Linux. |
| `startPresence` | `true` | Begin sending presence as soon as the app opens (the General switch **Show on Discord when the app opens**). |
| `openWindow` | `true` | Show the window when you open the app yourself. `false` goes straight to the tray (macOS and Linux: the background), like at login. |
| `checkUpdates` | `true` | Look for a newer release a few seconds after the program starts, then once a day. |
| `updateRepo` | `CptPakundo/MPCvibedRPC` | The GitHub repository (`owner/repo`) that publishes releases. A release is only installed by the program when it carries a `.sha256` checksum file for the download; otherwise the window links to its page. |
| `seenVersion` | (none) | The version whose changes the window last showed. Written by the window; a different version (an update, a new install) shows its changes once. **Restore defaults** keeps it. Not in the settings window. |
| `welcomeSeen` | `false` | Whether the first-run welcome card was closed. Written by the window; **Restore defaults** shows the card again. Not in the settings window. |

## For testing

These point the online lookups somewhere else (a test server or a mirror). They are not in the settings window, and
a service switched off on the Privacy tab stays off whatever address it has here.

| Setting | Default |
|---|---|
| `cinemetaBase` | `https://v3-cinemeta.strem.io` |
| `imdbSuggestBase` | `https://v3.sg.media-imdb.com` |
| `tvmazeBase` | `https://api.tvmaze.com` |
| `anilistUrl` | `https://graphql.anilist.co` |
| `kitsuBase` | `https://kitsu.io/api/edge` |
| `jikanBase` | `https://api.jikan.moe/v4` (the closed public API; point it at your own Jikan) |
| `wikipediaBase` | `https://{lang}.wikipedia.org` |
| `bulbapediaBase` | `https://bulbapedia.bulbagarden.net` |
| `requestGapMs` | `{"graphql.anilist.co": 250, "api.jikan.moe": 400}` (the least time between two requests to a host, in milliseconds) |

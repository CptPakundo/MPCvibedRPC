# Settings reference

Every setting with its default and what it does. The settings window covers the main ones and saves them as you
change them. For the rest, add them to `config.json` (in `%LOCALAPPDATA%\MPCvibedRPC`) using the same names, for
example `{"artworkSources": ["imdb", "tvmaze"]}`. Anything missing from `config.json` falls back to the default
shown here.

## Connection

| Setting | Default | What it does |
|---|---|---|
| `clientId` | `427863248734388224` | Discord application ID (the one used by the original MPC-DiscordRPC project). For your own name and artwork, create an app at <https://discord.com/developers/applications> and paste its Application ID here. |
| `port` | `13579` | Web interface port of MPC-HC, MPC-BE and MPC-QT (MPC-QT also works without its web interface). |
| `mpvPipe` | `'mpvsocket'` | The `input-ipc-server` name mpv listens on (set in `mpv.conf`; a name such as `mpvsocket` is the pipe `\\.\pipe\mpvsocket`). Empty: don't look for mpv. See [Players](../README.md#players). |
| `pollInterval` | `5000` | Milliseconds between checks. Presence is only sent when something changes. |

## How it looks

| Setting | Default | What it does |
|---|---|---|
| `activityType` | `'watching'` | `'watching'` is "Watching ..." with a real progress bar (elapsed and total time). `'playing'` is the classic "Playing ..." with an elapsed-time counter. |
| `titleAsName` | `true` | Shows "Watching <title>" instead of "Watching Media Player Classic". |
| `textProgressBar` | `true` | Shows a text bar (`▰▰▰▱▱▱ 12:34 / 45:00`) while paused, because Discord can't freeze its own bar. |
| `progressWidth` | `12` | Width of that text bar. |
| `showRemainingTime` | `false` | Only used when `activityType` is `'playing'`. |
| `episodeInDetails` | `false` | `true` repeats the season and episode number in the title line too (Discord already shows a badge when the card is expanded). |
| `richInfo` | `true` | Genres, IMDb rating and director on the extra lines. `false` leaves just the year or episode. |
| `linkButton` | `true` | Adds a "View on IMDb / AniList / ..." button (visible to others). |

## Privacy

These are all on the **Privacy** tab of the settings window.

| Setting | Default | What it does |
|---|---|---|
| `hideTitle` | `false` | Show only "Watching a video" with the progress bar: no title, cover, episode or buttons, and nothing is looked up online. The log does not record the title either. |
| `hideFiles` | `[]` | Files that are never shown. Each entry is matched without regard to case; `/` and `\` are alike. A plain entry hides a file whose whole path (folders and name) contains it, for example `Private` or `D:\Videos\Private`. An entry with `*` (any run of characters) or `?` (one character) must match the whole file name or the whole path, for example `*.xyz`. A hidden file shows no status at all and is not looked up. At most 100 entries of 300 characters. |
| `showArtwork` | `true` | Look up cover art and titles online. `false` stays completely offline (it also turns off episode-title lookups). |
| `disabledSources` | `[]` | Online services that must never be contacted, whatever the source lists below say. Ids: `tmdb`, `imdb`, `cinemeta`, `tvmaze`, `anilist`, `kitsu`, `jikan` (MyAnimeList), `wikipedia`, `bulbapedia`. The window shows a switch for each; the stored list holds the ones that are off. |
| `pauseClearMinutes` | `30` | Take the status down after the video has been paused this many minutes (0 = never). It comes back when you play again, seek, or turn this off. |

## Filename cleanup

Smart mode understands release names: it strips BluRay, 1080p, x265, HDR, audio tags and release groups, and finds
titles, years, seasons and episodes.

| Setting | Default | What it does |
|---|---|---|
| `smartFormat` | `true` | Smart mode on or off. |
| `ignoreBrackets` | `true` | Hide `[Group]` style tags. |
| `ignoreFiletype` | `true` | Hide `.mkv` / `.mp4`. |
| `replaceUnderscore` | `true` | Basic mode only (smart mode always does this). |
| `replaceDots` | `true` | Basic mode only (smart mode always does this). |

## Cover art and titles

The title parsed from the filename is looked up online (IMDb-based Cinemeta by default, no key needed). Set
`showArtwork` to `false` to stay fully offline.

| Setting | Default | What it does |
|---|---|---|
| `artworkSources` | `['tmdb', 'imdb', 'cinemeta', 'tvmaze', 'anilist', 'kitsu', 'jikan', 'wikipedia']` | Sources are asked at the same time; the first in this list with an exact title match wins. See the source list below. |
| `animeSources` | `['anilist', 'kitsu', 'jikan']` | Anime-style filenames ask these first, in this order. |
| `tmdbApiKey` | `''` | TMDB API key. Free at <https://www.themoviedb.org/settings/api>. TMDB is skipped unless this is set. |
| `artworkAliases` | `{}` | Search under another name, for example `{ 'filename title': 'Catalog Title' }`. |
| `artworkOverrides` | `{}` | Pin an image URL, for example `{ 'some title': 'https://example.com/poster.jpg' }`. |
| `artworkWaitMs` | `1500` | How long to hold the presence back waiting for a cover. It refreshes when the cover arrives. |
| `useFolderName` | `true` | If the file is just an episode number, use the folder's name as the show (skipping `Season 1` style folders). |
| `episodeTitles` | `true` | Look up the real episode name when the file doesn't have one (`Ep03` becomes `S01E03 · Episode title`). |
| `episodeSources` | `['bulbapedia', 'tmdb', 'tvmaze', 'cinemeta', 'kitsu']` | Order tried. `tmdb` needs `tmdbApiKey`; the others need no key. |
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
| `kitsu`, `jikan` | More anime databases (Kitsu; MyAnimeList via Jikan), used if AniList is down or misses. |
| `wikipedia` | Last resort: the article's lead image (Hebrew titles use he.wikipedia.org). |

If a title still has no cover, the log says what each source returned. Then either add an alias or pin an image.

## Discord art assets

These are uploaded to the Discord application whose `clientId` you use.

| Setting | Default | What it does |
|---|---|---|
| `largeImageKey` | `'mpc-hc'` | Shown when no cover is found. |
| `smallImageKey` | `'mpc-hc'` | Small corner icon when a cover is shown. |
| `appName` | `'Media Player Classic'` | Name used when `titleAsName` is off. |

## Program settings

These belong to the program itself and live under `app` in `config.json`. They are all in the settings window.

| Setting | Default | What it does |
|---|---|---|
| `autoStart` | `false` | Start when you log in, straight to the tray. |
| `startPresence` | `true` | Begin sending presence as soon as the app opens. |
| `openWindow` | `true` | Show the window when you open the app yourself. `false` goes straight to the tray, like at login. |
| `checkUpdates` | `true` | Look for a newer release now and then. |
| `updateRepo` | `CptPakundo/MPCvibedRPC` | The GitHub repository (`owner/repo`) that publishes releases. |

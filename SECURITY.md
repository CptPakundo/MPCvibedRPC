# Security

This program has not been audited, so please treat it accordingly. It runs on your own
computer, talks to Discord and the player (MPC-HC, MPC-BE, MPC-QT, mpv, VLC, PotPlayer or IINA, or on Linux a video player
over D-Bus) locally, and to Kodi at the address you enter once you set its password (this computer by default).
Every other address it reaches is in the list below; a test fails when the code names one that is not.

## Where it connects
**Asked by the program.** The catalogs, each with its own switch on the **Privacy** tab:

| Address | Service | When |
|---|---|---|
| `api.themoviedb.org` | TMDB | Only with your own TMDB key. |
| `v3.sg.media-imdb.com` | IMDb | Title search. |
| `v3-cinemeta.strem.io` | Cinemeta | Titles, genres, ratings and runtimes. |
| `api.tvmaze.com` | TVmaze | TV shows, seasons and episode names. |
| `graphql.anilist.co` | AniList | Anime titles and covers. |
| `kitsu.io` | Kitsu | Anime titles, covers and episode names. |
| `api.jikan.moe` | MyAnimeList (via Jikan) | Only if you add your own Jikan server (the public one closed). |
| `wikipedia.org` | Wikipedia | A last-resort cover (the article's language site, such as en.wikipedia.org). |
| `bulbapedia.bulbagarden.net` | Bulbapedia | Episode names for files named with a production code. |

And these, only for what they are named for:

| Address | Service | When |
|---|---|---|
| `api.github.com` | GitHub | Check for updates is on (**General > Updates**): asks for the newest release. |
| `github.com` | GitHub | The same: the release page when the API's hourly limit is used up, and the download. |
| `release-assets.githubusercontent.com` | GitHub | An update is installed: GitHub sends the download from here. |
| `plex.tv` | Plex | Signing in to Plex, and while signed in (your servers). |
| `clients.plex.tv` | Plex | While signed in to Plex: your servers and their addresses. |
| `link.stremio.com` | Stremio | Signing in to Stremio (the link code). |
| `api.strem.io` | Stremio | While signed in to Stremio: your library every 20 seconds, and signing out. |

Once signed in, it also reads from your own Plex server, and from the Jellyfin or Emby server whose address you
entered (Accounts tab): to sign in, to read its session list and to sign out.

**Loaded by the window and by Discord**, not by the program: the cover on your status and in the preview comes from
the image host the catalog named for it (`image.tmdb.org` for TMDB), and the player's logo from this repository on
`raw.githubusercontent.com`.

**Only linked**, opened in your browser or shown as a button on your status: `app.plex.tv` (approving the Plex
sign-in), `stremio.com` (entering the Stremio link code), and the title's page on `www.imdb.com`,
`www.themoviedb.org`, `www.tvmaze.com`, `kitsu.io` or the catalog that found it.

## What is kept, and what to know
The Plex, Stremio, Jellyfin and Emby sign-ins are kept in `config.json` as plain text; a password you type goes to your
own server once and is never stored. Kodi's password is kept in `config.json` as plain text too, as VLC's is.

MPC-HC's web interface has no password and lets any web page read it (it answers every origin). While it is on, a
website open in your browser could see what you play and send the player commands. "Set up the player connection"
limits it to this computer, which keeps other devices out but not web pages you open here. Switch it off in MPC-HC
(**Options > Player > Web Interface**) when you stop using this program.

## Reporting a problem
If you find a security problem, please report it **privately** through
[GitHub's private vulnerability reporting](https://github.com/CptPakundo/MPCvibedRPC/security/advisories/new)
rather than in a public issue. There is no guaranteed response time or fix, but reports are read.

## Checking a download
Release files come with a SHA-256 checksum and a signed build record from GitHub. To check that a downloaded
`MPCvibedRPC.exe` (or `MPCvibedRPC-macos.zip`, `MPCvibedRPC-linux-amd64`, ...) was built from this repository by its
release workflow:

```
gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC
```

The program itself is not code-signed, so Windows SmartScreen may warn about it. The macOS app only has an ad-hoc
signature (no Apple Developer ID, not notarized), so macOS asks you to confirm its first start.

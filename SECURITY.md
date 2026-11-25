# Security

This program has not had a security audit, so please treat it accordingly.

It runs on your own computer. There it talks to Discord and to your player: MPC-HC, MPC-BE, MPC-QT, mpv, VLC,
PotPlayer or IINA, or on Linux a video player over D-Bus. Once you set Kodi's password, it also talks to Kodi at the
address you enter (this computer by default): its web server, and port 9090 there for its notifications.

Every other address it reaches is in the list below, and a test fails when the code names one that is not. The
**Privacy** tab's **Recent connections** shows which of them it actually asked since it started, how often and when.

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
| `plex.tv` | Plex | Signing in to Plex, while signed in (your servers), and signing out (ends the sign-in on Plex's side). |
| `clients.plex.tv` | Plex | While signed in to Plex: your servers and their addresses. |
| `link.stremio.com` | Stremio | Signing in to Stremio (the link code). |
| `api.strem.io` | Stremio | While signed in to Stremio: your library every 20 seconds, and signing out. |
| `letterboxd.com` | Letterboxd | **Link to Letterboxd when it has the title** is on (**Look** tab, off by default): asks once per title, by its IMDb or TMDB id, whether Letterboxd has a page for it; the button on your status then links that page. |

Once signed in, it also reads from your own Plex server, and from the Jellyfin or Emby server whose address you
entered (Players tab): to sign in, to read its session list and to sign out.

**Loaded by the window and by Discord**, not by the program: the cover on your status and in the preview comes from
the image host the catalog named for it (`image.tmdb.org` for TMDB), and the player's logo from this repository on
`raw.githubusercontent.com`.

**Only linked**, opened in your browser or shown as a button on your status:

| What | Addresses |
|---|---|
| Approving a sign-in | `app.plex.tv` (Plex), `stremio.com` (the Stremio link code) |
| The title's page | `www.imdb.com`, `www.themoviedb.org`, `www.tvmaze.com`, `kitsu.io` or the catalog that found it; `letterboxd.com` with Link to Letterboxd |
| The player's site, from its logo on your status | `github.com` (the MPC players), `mpv.io`, `www.videolan.org`, `iina.io`, `kodi.tv`, `www.smplayer.info`, `www.plex.tv`, `www.stremio.com`, `jellyfin.org`, `emby.media` |

## What is kept, and what to know
The Plex, Stremio, Jellyfin and Emby sign-ins are kept in `config.json` as plain text. A password you type goes to
your own server once and is never stored. Kodi's password is kept in `config.json` as plain text too, as VLC's is.

Your Plex sign-in goes only to plex.tv and to a server that has first shown, without it, that it is the server you
chose. A server shared with you gets the key plex.tv made for it, never your account's.

The cover cache (`artwork-cache.json`) holds the looked-up answers for the titles played in the last 90 days, at most
2000; **Clear cover cache** (Privacy tab) empties it.

The settings window is a page the program serves on this computer only (127.0.0.1). Its API needs a key made at
every start, and answers no other website (a Host check, and no cross-origin permission). The browser is started with
a code that works once, for two minutes at most, so the key itself never appears on a command line that other users
of the computer could read. A settings file that cannot be read is never overwritten, and it is written to disk
before it replaces the old one.

To show the title on a stream only while one is on, the program reads (never writes) two things on
Windows: the names and start times of the Discord app's threads (not its memory), and Discord's entry in Windows'
record of which program is capturing the screen. Nothing of either is sent anywhere.

MPC-HC's web interface has no password and lets any web page read it (it answers every origin). While it is on, a
website open in your browser could see what you play and send the player commands. "Set up the player connection"
limits it to this computer, which keeps other devices out but not web pages you open here. Switch it off in MPC-HC
(**Options > Player > Web Interface**) when you stop using this program. The missing password is MPC-HC's own design.

## Reporting a problem
If you find a security problem, please report it **privately** through
[GitHub's private vulnerability reporting](https://github.com/CptPakundo/MPCvibedRPC/security/advisories/new)
rather than in a public issue. There is no guaranteed response time or fix, but reports are read.

## How updates are checked
An update is checked against the `.sha256` file published in the same release, and installed only when the download
matches it. That is not a signature and not the build record described below: the program does not verify either. So
whoever can publish a release in the repository the program updates from decides what installs, and so does a user who
points `updateRepo` (a setting in `config.json`) at another repository. Reducing this trust (a signed release
manifest, immutable releases) is an open decision in the
[roadmap](docs/roadmap.md#3-updates-you-can-trust-without-a-personal-signature).

## Checking a download
Release files come with a SHA-256 checksum and a signed build record from GitHub. To check that a downloaded
`MPCvibedRPC.exe` (or `MPCvibedRPC-macos.zip`, `MPCvibedRPC-linux-amd64`, ...) was built from this repository by its
release workflow:

```
gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC --signer-workflow CptPakundo/MPCvibedRPC/.github/workflows/release.yml
```

You can also rebuild `MPCvibedRPC.exe` and the Linux programs yourself and compare: every release is published only
after a second runner rebuilt them from the tag byte for byte. Check out the tag with the files as committed (LF line
endings, for example `git clone -c core.autocrlf=false`), install the Go version named in
`go.mod` (the `go` line), run `tools/ci/reproduce.sh X.Y.Z out` and compare the files' SHA-256 with the
release's `.sha256` files. The macOS app is built with Apple's tools and cannot be rebuilt this way.

The program itself is not code-signed, so Windows SmartScreen may warn about it. The macOS app only has an ad-hoc
signature (no Apple Developer ID, not notarized), so macOS asks you to confirm its first start.

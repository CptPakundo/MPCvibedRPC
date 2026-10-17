## MPCvibedRPC 0.9.31

**Fixes for changes made by others: Stremio's newer apps, the closed MyAnimeList service, Discord's renamed settings,
and a newer Go for every build.**

### Stremio
- Stremio's newer apps save your place in the library every 3 minutes instead of every 90 seconds. With them, a
  playing video dropped off Discord for half a minute every three minutes, and a pause was often missed. The program now learns which of the two your
  Stremio app uses, so a video stays shown with both, and a pause still takes it down.
- Also fixed: the first regular update after a short pause was taken for another pause, so the status went away for
  a while although the video played on.
- If Stremio is closed without a word, the status now goes after about four minutes.

### Covers and episode titles
- MyAnimeList's public service (Jikan) closed on 2026-10-01, so it is no longer asked. Anime are still found through
  AniList and Kitsu. The switch on the Privacy tab stays, for a Jikan server you run yourself (see
  [All settings](https://github.com/CptPakundo/MPCvibedRPC/blob/master/docs/config.md)). If you edited
  `artworkSources`, `animeSources` or `episodeSources` in `config.json` by hand, remove `jikan` from them.
- The Help tab and THIRD-PARTY-NOTICES.md credit TMDB (with its logo, as its terms ask) and TVmaze.

### Discord
- Discord renamed its *Activity Settings* section to *Games & Apps*. The welcome card, the README and the
  troubleshooting page name both, so *User Settings > Activity Privacy > "Share my activity"* is found in old and new
  Discord versions.

### macOS
- **macOS 13 or later** is needed now. The program is built with Go 1.27, which needs macOS 13, and Discord itself no
  longer runs on macOS 11 or 12.
- On macOS 15 and later, macOS asks before an app reaches your home network. The app now says why it wants to, so a
  Plex, Jellyfin, Emby or Kodi server there can be allowed (System Settings > Privacy & Security > Local Network).

### Under the hood
- Built with Go 1.27.2 instead of Go 1.24, which no longer gets security fixes. Every change is now also checked for
  known Go security issues.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

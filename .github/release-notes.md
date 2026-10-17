## MPCvibedRPC 0.9.32

**A weekly check for changes made by others. Nothing in the program itself changed.**

### Why
The players, media servers and cover and title services the program reads change without notice. In one month a
catalog service closed and Stremio changed how often it saves your place, and in both cases the program only noticed
once something already looked wrong on Discord.

### What's new
- Every Monday the whole test suite runs against the newest PotPlayer, Kodi, IINA and mpv (macOS) and the latest Plex,
  Jellyfin and Emby servers.
- The same run asks each cover and title service (Cinemeta, IMDb, TVmaze, AniList, Kitsu, Wikipedia, Bulbapedia) about
  one well-known title through the program's own lookup code. It checks for a match, the year, a link, a cover and the
  first episode's name.
- When something fails, an issue is opened in this repository, and it is closed once a later run passes, so problems
  are found within a week.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

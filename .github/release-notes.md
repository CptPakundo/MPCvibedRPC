## MPCvibedRPC 0.9.30

**PotPlayer and Kodi, each player's own logo on Discord, hiding a whole Plex library, and a clear word when Discord
runs as administrator.**

### New players
- **PotPlayer** (Windows): found by itself, nothing to set up. It is read through the window messages PotPlayer offers
  for remote control: play state, position, length, speed and the playing file's path, so the Never show list can
  match its folders as for any other player.
- **Kodi** (Windows, macOS, Linux, or a Kodi on another device such as a TV box): turn on *Settings > Services >
  Control > Allow remote control via HTTP* in Kodi with a user name and password, and enter them under **Advanced >
  Players** (the address too, for a Kodi elsewhere). Episodes and movies from Kodi's library are named from Kodi's own
  information. On Linux, Kodi is also found through MPRIS without this.
- Both were tried with the real players on GitHub's Windows test machines: PotPlayer and Kodi installed fresh, a
  test video playing in each.

### On Discord
- **Each player's own logo.** mpv, VLC, IINA, PotPlayer, Kodi, SMPlayer, Plex, Stremio, Jellyfin and Emby show their
  logo as the small icon next to the cover, and as the picture when no cover is found. That spot used to show
  Media Player Classic's logo, or nothing. MPC-HC, MPC-BE and MPC-QT look as before. Discord loads the logos from this
  project's GitHub page; nothing about you is sent.
- With **Link to the title's page** on, both text lines open the title's page, not only the first. Others can click
  them; Discord doesn't let you click your own, and the switch's help now says so.

### Privacy
- **Never show these files** also hides by where a server keeps a video. For Plex it matches the library's name, so
  one entry such as `Kids` hides a whole library. For Plex, Jellyfin and Emby it also matches the file's path on the
  server: `Private` hides that folder, as it does for files on this computer. Real Jellyfin and Emby servers were
  checked to send that path. These names and paths are only compared, never shown, logged or sent anywhere.

### Help
- When Discord runs as administrator and this program doesn't, Windows keeps the two apart. The window now says
  "Discord refused the connection ... start it normally" instead of only "Waiting for Discord".
- **Wrong title or cover?** under the preview takes you to Titles & art > Fix a match.
- Troubleshooting covers more of the usual reasons a status doesn't show: Discord run as administrator, several
  Discord apps open at once, your own buttons not being clickable, "Watching Media Player Classic" instead of the
  title, mpv-based players, and Windows' warning for this unsigned program.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time, System Settings >
  Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on. To check a
download, see [SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

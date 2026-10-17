## MPCvibedRPC 0.9.26

**Hide a whole Plex library, a clear word when Discord runs as administrator, and clickable text lines.**

### Privacy
- **Never show these files** now also hides by where a server keeps a video. For Plex it matches the library's name,
  so one entry such as `Kids` hides a whole library. For Plex, Jellyfin and Emby it also matches the file's path on
  the server, when the server gives it: `Private` hides that folder, just as it does for files on this computer.
  These names and paths are only compared. They are never shown, written to the log or sent anywhere.

### Discord
- When Discord runs as administrator and this program doesn't, Windows keeps the two apart. The window now says so
  ("Discord refused the connection ... start it normally") instead of only "Waiting for Discord".
- With **Link to the title's page** on, both text lines on your Discord status open the title's page, not just the
  first. Others can click them; Discord doesn't let you click your own, and the switch's help now says so.

### Window
- **Wrong title or cover?** under the "How it looks on Discord" preview takes you to Titles & art > Fix a match.

### Help
- Troubleshooting covers more of the usual reasons a status doesn't show: Discord run as administrator, several
  Discord apps open at once, your own buttons not being clickable, and "Watching Media Player Classic" instead of the
  title. It also covers mpv-based players and Windows' "Windows protected your PC" warning for this unsigned program.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time, System Settings >
  Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on. To check a
download, see [SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

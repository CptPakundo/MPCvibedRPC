## MPCvibedRPC 0.9.24

**A tidier settings window, and cleaner code underneath. What is shown on Discord doesn't change.**

### The window
- **Settings in grouped cards.** Each group of settings (Startup, Plex, What other people see on Discord, ...) is one
  card with a plain heading, so the tabs are quicker to scan.
- **Seven tabs instead of eight.** The Updates rows moved into **General**, under Startup: **Check now**, **Install**
  and the automatic check are all there. The notice at the top when an update is available leads to them.
- **A quieter status card.** While nothing is shown on Discord, the card is one line and the **Show on Discord**
  button. The Discord and player lights appear once it runs. When no player answers, the notice below explains it, so
  the card no longer also says "Nothing playing".
- **The version number opens the changelog.** The separate Changelog button is gone.
- The update source (`updateRepo`) is no longer a field under Advanced. It stays a setting in `config.json`, for
  anyone who builds their own copy.

### Underneath
- The program's start-up, the window's API, updates and the tray each have a file of their own. The Plex, Stremio and
  Jellyfin sign-ins share one piece of code for "wait until the user approves".
- The engine reads Plex, Stremio and Jellyfin as one list. The rule for which player wins (a video that plays, then
  music, then a paused one) lives in one small piece of code.
- The cover lookup and its title matching are split into named steps. The regex engine drops features no rule uses.
- The tests share their setup instead of repeating it. The cover-lookup tests run in 5 seconds instead of 12.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates tab of the version you have.
  Your settings are kept.
- **macOS:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time, System Settings >
  Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on. To check a
download, see [SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

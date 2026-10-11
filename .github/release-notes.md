## MPCvibedRPC 0.9.23

**The right player wins, music is "Listening to", and a full round of fixes.**

### What's shown
- **A paused player no longer hides one that plays.** Leave MPC-HC paused and start a video in mpv, VLC or Plex: the
  video that plays is shown. When nothing plays, the paused one is shown again.
- **Music is "Listening to".** A song played in a video player (`.mp3`, `.flac`, `.ogg`, `.m4a`, ...) now shows as
  "Listening to Artist - Song" instead of "Watching", named as the file is, and its name is no longer sent to the movie
  and TV catalogs. A video playing in another player still comes first.
- Players other than MPC-HC, MPC-BE and MPC-QT no longer show Media Player Classic's logo in the corner of the cover.
- A video whose length is only known after a moment (a stream, a slow network file) gets its progress bar once the
  length arrives.
- A "View on ..." button that Discord turns down is left out of that one update, instead of switching the whole status
  to the plain fallback until the next restart.

### Accounts
- Your Plex sign-in is no longer passed on when plex.tv or a server redirects to another address. Jellyfin and Emby
  don't follow a redirect to another server, or from https to http, at all.
- A sign-in that finishes at the moment you press **Sign out** or **Restore defaults** can no longer save itself
  afterwards. Pressing Jellyfin's **Sign in** twice no longer leaves the first attempt running.
- Jellyfin Quick Connect: when you approved the code but the server doesn't finish the sign-in, the window says so
  after a few seconds instead of waiting ten minutes and then claiming the code wasn't approved.
- A Jellyfin player that went away without saying so (a crashed app, a TV that lost its network) stops showing after 5
  minutes.
- A server whose https certificate isn't trusted says exactly that, instead of "no connection".

### Reliability
- Covers: a title looked up while a catalog was down, slow or offline is tried again after 2 minutes, not 30. The cover
  cache keeps the answers you used recently when it trims itself, and a restart can no longer write an older cache over
  a newer one, or bring back a cache you just cleared.
- Two saves at the same moment (the window and the tray, or a sign-in finishing) can no longer leave the program on
  the older settings.
- Emptying the "Clear my status when paused for" box no longer quietly turns pause clearing off; the old value stays.
- When the system refuses **Start with Windows** / **Start at login**, the window says so instead of "Saved.".
- **Restore defaults** no longer shows this version's news again.
- The log no longer grows without limit in a copy that runs for weeks.
- Self-update only installs a release that has a checksum file; otherwise it points you to the release page.
- Windows: the tray icon appears even when the taskbar comes up later than the program at login. A frozen Discord
  can no longer hold the program up, and an idle connection to Discord wakes the program far less often.
- **Copy diagnostics** also leaves out passwords in web addresses and more kinds of folder paths, and names every
  Linux video player it finds.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates tab of an earlier version;
  your settings are kept.
- **macOS:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time, System Settings >
  Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on. To check a
download, see [SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

## MPCvibedRPC 0.9.22

**A log that can explain a wrong title, a simpler window, and a round of fixes.**

### The log now names the file
When a title comes out wrong ("Sample Show" shows up as "Sample Movie"), the only way to find out why is to see the
file name it was read from. Until now the log recorded only the recognised title, and **Copy diagnostics** left titles
and file names out, so a report about a wrong match couldn't be traced back to its cause.

- Each title in the log now names its source: `Playing: Sample Show · S01E02 (MPC-HC: "Sample.Show.S01E02.1080p.mkv")`.
  When the show's name came from the folder, the line says so.
- **Copy diagnostics** (Help tab) keeps titles and file names. It still leaves out folder paths (they can contain your
  user name), the paths and queries of web addresses, server names, sign-ins, keys and your **Never show** list.
- Nothing changes for what you keep private: with **Hide what I'm watching** on, or for a file on your **Never show**
  list, the log records no title and no file name, as before. Those files are never looked up, so there is nothing to
  trace. The log stays on your computer; only what you choose to paste into a report leaves it.

### A simpler window
- Sign-ins for Plex, Stremio and Jellyfin / Emby have their own **Accounts** tab, and the three look alike.
- One way of saying it: **Show on Discord** / **Stop showing**. The tray menu (Windows) and the menu bar menu (macOS)
  start with a line that says what the app is doing.
- Shorter texts and a shorter welcome. Settings are grouped: Advanced has **Players** (with **Set up the player
  connection**), **Discord** and **Maintenance** (Restore defaults, the settings folder, the update source);
  Titles & art has **Fix a match** and **More**. The **Log** tab is now **Help**, with **Report a problem**.
- A line at the top of the window says when an update is available. The tabs work with the arrow keys.

### Fixes
- A dropped connection while Discord restarts no longer leaves a plain, cover-less status behind until you restart.
- Plex no longer brings back a video that stopped while the connection was down. Stremio no longer misses a start when
  one request fails. VLC keeps the folder name for the whole video.
- Settings, the cover cache and the players' own settings files are written safely, so a crash can't leave half a file.
  "Clear cover cache" now clears everything.
- Updates are refused when their checksum file is missing or broken, and a double click no longer starts two installs.
- On macOS and Linux, the settings and sign-ins can only be read by your own user.
- When Discord turns the program away, the log says why (for example "Invalid Client ID").

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates tab of an earlier version;
  your settings are kept.
- **macOS:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time, System Settings >
  Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on. To check a
download, see [SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

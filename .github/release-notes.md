## MPCvibedRPC 0.9.40

**Updates install themselves, and the icon shows when one is waiting.**

### What's new
- **Automatic updates** (Windows and Linux). When a new version is out, the program installs it by itself as soon as
  nothing is playing and the settings window is closed, so a status on Discord is never cut off mid-episode. It
  restarts quietly in the tray and says once that it was updated. Every update is still checked against its checksum,
  and if the new version does not start, the old one keeps running. Switch it off under **General > Updates > Install
  updates automatically**; then the window and the tray notice offer the update as before.
- **A red dot on the icon** while an update waits: on the tray icon on Windows, and on the menu bar icon on macOS
  (where the window links to the new download).
- **More frequent checks**: every six hours instead of once a day, besides the check a few seconds after starting.

From this version on, updates arrive by themselves. Copies of an older version still show the update notice and
need one click on **Install**.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

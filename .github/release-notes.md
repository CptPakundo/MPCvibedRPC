## MPCvibedRPC 0.9.34

**Faster updates from mpv and IINA, a clearer view of what the program does, and releases anyone can rebuild.**

### What's new
- **mpv and IINA: changes show at once.** Pausing, resuming, seeking or opening another file now reaches Discord
  within a moment, from the player's own event, instead of at the next check (up to 5 seconds later). Discord accepts
  only a few updates in a short time, so early updates are at most one every 4 seconds. Other players are checked
  every 5 seconds as before.
- **Why this title?** Copy diagnostics (Help tab) now explains the title on Discord: the file name the player gave,
  what was read from it (title, year, season, episode), which catalog matched it and as what, and where the episode
  title came from. When nothing matched, it lists the closest results, or says that a catalog was slow and is asked
  again soon.
- **Recent connections** (Privacy tab): every address outside your computer the program sent a request to since it
  started, how often and when last. It never shows or stores what was asked, and it is kept in memory only. Every
  address the code can reach is listed in [SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md#where-it-connects).
- **Rebuildable releases.** Every release is now rebuilt from its source on a second machine and published only when
  the Windows program and both Linux programs match the release byte for byte. You can do the same yourself
  (SECURITY.md says how). The workflows that build releases use pinned versions of their tools and only the
  permissions they need.
- **Two new pages:** [why the program behaves as it does](https://github.com/CptPakundo/MPCvibedRPC/blob/master/docs/decisions.md),
  and [how it is made with AI and checked](https://github.com/CptPakundo/MPCvibedRPC/blob/master/docs/ai.md).

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

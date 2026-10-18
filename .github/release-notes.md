## MPCvibedRPC 0.9.39

**A download counter that counts people.** The program itself is unchanged.

### What's new
- **Downloads by people.** The download badge on the project page used to add up every file of every release,
  checksums included, and most of those downloads were the project's own release checks. It now counts only the
  program files (the Windows exe, the Linux programs and the macOS app), from a fresh start on 2026-10-18 (every
  release file was replaced by its identical copy, so links, checksums and updates are unchanged), minus every
  download the project makes itself: the release checks and the AI agents that build the program record each of
  theirs, and those are taken off. The program never reports anything about anyone; it only reads GitHub's counts.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

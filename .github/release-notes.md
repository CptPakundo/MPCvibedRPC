## MPCvibedRPC 0.9.11

**Better episode titles for anime.**

- Anime episode titles now come from **MyAnimeList** first. It numbers episodes the way anime files do, so episode 1,100 of a long-running show is simply episode 1,100.
- Long-running anime no longer show the title of a neighbouring episode (one catalog files specials among the episodes, which shifted the count), or no title at all (another picked a live-action series of the same name).
- When a catalog does not answer in time, the episode title is looked up again while the episode plays, and appears on Discord as soon as it is found.
- Titles saved by earlier versions that may be off are looked up again once.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it; there is nothing to install. An earlier version can install this one from its Updates tab; your settings are kept. Needs Windows 10/11.
- **macOS:** download **`MPCvibedRPC-macos.zip`**, move the app to Applications and open it. The app is not notarized by Apple, so the first time go to System Settings > Privacy & Security and choose **Open Anyway** (see the README).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Presence starts by itself: play something. If no player is answering, the window offers to set up the connection for you. Every system needs the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- Plex is tested on GitHub's test machines with every change: the official Plex Media Server, a stand-in player and a stand-in for Discord. Signing in with a real Plex account and real Plex apps have not been tried yet; reports are welcome.
- The macOS and Linux versions are tested on GitHub's test machines with real players (mpv, IINA, VLC, Celluloid) and a stand-in for Discord. A tester has also used the Mac version with IINA and the real Discord. Linux has not been tried on a real desktop yet. Reports are welcome.
- The programs are **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn, and macOS asks you to confirm the first start. Check a download with its `.sha256` file, or with `gh attestation verify <file> --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC` (macOS: `~/Library/Application Support/MPCvibedRPC`; Linux: `~/.local/share/MPCvibedRPC`).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.

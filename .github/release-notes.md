## MPCvibedRPC 0.9.20

**Stremio, and a program that rests while Discord is closed.**

- **Stremio:** press **Sign in to Stremio** on the Advanced tab and approve it on stremio.com. What you watch with your Stremio account then shows on Discord, on any device: the desktop app, Stremio Web, a TV or a phone. The program reads your library, which Stremio's player updates while it plays, so a video shows up within about 20 seconds; pausing takes the status down, and it comes back when you resume. It only reads, and **Sign out** also ends the session on Stremio's side.
- **Resting:** while Discord is not running and the settings window is closed, the program asks no player, follows no Plex server or Stremio account, and hands its memory back to the system (on Windows it drops to about 3 MB). It only looks whether Discord has started, and picks up by itself when it does. Opening the window wakes it at once.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it; there is nothing to install. An earlier version can install this one from its Updates tab; your settings are kept. Needs Windows 10/11.
- **macOS:** download **`MPCvibedRPC-macos.zip`**, move the app to Applications and open it. The app is not notarized by Apple, so the first time go to System Settings > Privacy & Security and choose **Open Anyway** (see the README).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Presence starts by itself: play something. If no player is answering, the window offers to set up the connection for you. Every system needs the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- Stremio is tested against a stand-in for Stremio's servers, built from Stremio's own open-source core. Signing in with a real Stremio account has not been tried yet; reports are welcome. Newer Stremio apps can show a Discord status of their own: turn that off in Stremio, or you may see two.
- Plex is tested on GitHub's test machines with every change: the official Plex Media Server, a stand-in player and a stand-in for Discord. Signing in with a real Plex account and real Plex apps have not been tried yet; reports are welcome.
- The macOS and Linux versions are tested on GitHub's test machines with real players (mpv, IINA, VLC, Celluloid) and a stand-in for Discord. A tester has also used the Mac version with IINA and the real Discord. Linux has not been tried on a real desktop yet. Reports are welcome.
- The programs are **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn, and macOS asks you to confirm the first start. Check a download with its `.sha256` file, or with `gh attestation verify <file> --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC` (macOS: `~/Library/Application Support/MPCvibedRPC`; Linux: `~/.local/share/MPCvibedRPC`).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.

## MPCvibedRPC 0.9.9

**macOS and Linux.** MPCvibedRPC now runs on Macs and on Linux too, alongside Windows.

- **macOS** (11 or later, Apple silicon and Intel): **IINA**, mpv, VLC and MPC-QT. **Set up the player connection** adds the mpv option IINA needs to its settings (reopen IINA afterwards). The app has no Dock or menu bar icon: it runs in the background, opening it again shows its window, and Quit is in the window.
- **Linux** (64-bit x86 and ARM): VLC, Celluloid, Haruna, SMPlayer, GNOME Videos, Clapper and other video players are found through MPRIS with nothing to set up; mpv through the mpv-mpris script or one line in `mpv.conf` (the button adds it). Music players and web browsers are never shown. No tray icon: run the program again to show its window.
- **Start at login** works on both (a LaunchAgent on macOS, an autostart entry on Linux).
- Updates: Linux updates itself like Windows; on macOS the Updates tab links to the release page.
- **Windows is unchanged.**

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it; there is nothing to install. An earlier version can install this one from its Updates tab; your settings are kept. Needs Windows 10/11.
- **macOS:** download **`MPCvibedRPC-macos.zip`**, move the app to Applications and open it. The app is not notarized by Apple, so the first time go to System Settings > Privacy & Security and choose **Open Anyway** (see the README).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Presence starts by itself: play something. If no player is answering, the window offers to set up the connection for you. Every system needs the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- The macOS and Linux versions have been tested on GitHub's test machines with real players (mpv, IINA, VLC, Celluloid) and a stand-in for Discord, not yet on a real desktop with the real Discord. Reports are welcome.
- The programs are **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn, and macOS asks you to confirm the first start. Check a download with its `.sha256` file, or with `gh attestation verify <file> --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC` (macOS: `~/Library/Application Support/MPCvibedRPC`; Linux: `~/.local/share/MPCvibedRPC`).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.

## MPCvibedRPC 0.9.12

**Update checks, tidied up.**

- New versions are looked for a few seconds after the program starts, as well as once a day. An open settings window now shows a new version as soon as it is found, without reopening it.
- The Updates tab shows what is new in an update with its paragraphs and bullet points, instead of one long run of text.

Updating from 0.9.11 or earlier, the Updates tab still shows these notes as plain text one last time; from 0.9.12 on they are laid out properly.

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

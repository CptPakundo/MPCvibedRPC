## MPCvibedRPC 0.9.10

**Plex.** MPCvibedRPC can now show what you watch on Plex, on Windows, macOS and Linux.

- Press **Sign in to Plex** on the Advanced tab and approve the sign-in on plex.tv in your browser. MPCvibedRPC never sees your password.
- What you play with your Plex account shows on Discord, on any device: the Plex app on your computer, Plex Web, a TV or a phone. The episode or movie comes from your Plex server, and cover art is found the same way as for your files.
- On a server you own, only your own playback is shown, not that of people you share with. Music and photos are never shown as "Watching". Choose another of your servers under the button, and **Sign out** at any time (Restore defaults signs you out too).
- Nothing changes until you sign in: plex.tv and your server are only contacted while you are signed in, and the players you already use are asked first, as before.

**Also new:** the Quit, Restore defaults and "close the player for a moment" questions are now shown inside the window, with a clear title and buttons, instead of a browser box headed with the window's local address.

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

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

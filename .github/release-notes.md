## MPCvibedRPC 0.9.21

**Jellyfin and Emby, a clickable title, and hiding from the tray.**

- **Jellyfin and Emby:** on the Advanced tab, enter your server's address and press **Sign in**. On Jellyfin you approve a code in a Jellyfin app or page where you are already signed in (your profile > Quick Connect); on Emby, or when Quick Connect is off, you sign in with your user name and password, which go to your server once and are never stored. What you play on that server then shows on Discord, on any device: a TV, a phone, the web page or an app on this computer. Only your own playback counts, music is left out, and the program only reads. **Sign out** also ends the session on your server.
- **Clickable title and cover:** when a title is recognised, the second line and the cover on your Discord card now open its page (IMDb, AniList, ...), just like the "View on IMDb" button. It follows the same switch as the button (How it looks > Link to the title's page), which is on unless you turned it off.
- **Hide what I'm watching from the tray:** the tray menu (Windows) and the menu bar menu (macOS) have a **Hide what I'm watching** item with a tick, so you can hide the title without opening the window. An open window follows along.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it; there is nothing to install. An earlier version can install this one from its Updates tab; your settings are kept. Needs Windows 10/11.
- **macOS:** download **`MPCvibedRPC-macos.zip`**, move the app to Applications and open it. The app is not notarized by Apple, so the first time go to System Settings > Privacy & Security and choose **Open Anyway** (see the README).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Presence starts by itself: play something. If no player is answering, the window offers to set up the connection for you. Every system needs the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- Jellyfin and Emby are tested on GitHub's test machines with every change: the official Jellyfin and Emby servers, a stand-in player and a stand-in for Discord, through the real sign-in (password and Quick Connect). Real Jellyfin and Emby apps and your own server have not been tried yet; reports are welcome.
- The clickable title and cover use fields Discord added recently. Should your Discord refuse them, the program carries on without the links (the button stays).
- Stremio is tested against a stand-in for Stremio's servers, built from Stremio's own open-source core; Plex on GitHub's test machines against the official Plex Media Server. Real Stremio and Plex accounts have not been tried yet; reports are welcome.
- The macOS and Linux versions are tested on GitHub's test machines with real players (mpv, IINA, VLC, Celluloid) and a stand-in for Discord. A tester has also used the Mac version with IINA and the real Discord. Linux has not been tried on a real desktop yet. Reports are welcome.
- The programs are **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn, and macOS asks you to confirm the first start. Check a download with its `.sha256` file, or with `gh attestation verify <file> --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC` (macOS: `~/Library/Application Support/MPCvibedRPC`; Linux: `~/.local/share/MPCvibedRPC`).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.

## MPCvibedRPC 0.9.8

**VLC support.** VLC media player now shows up on Discord like the other players.

- **VLC** (3.0; a development build of VLC 4 works too) is read through its own web interface. **Set up the player connection** switches that interface on in VLC's settings, gives it a password and fills the password in for you. A password and port you already set in VLC are kept and taken over. An interface the button switches on only answers this PC. VLC has to be closed for a moment if its settings change (it reopens; you are asked first).
- If VLC refuses the password, or another program answers on its port, the window says so instead of just "No player found".
- VLC is asked after the other players and only once a VLC password is set, so nothing changes if you don't use VLC.
- Two new settings under **Advanced**: VLC web interface password and port. Copy diagnostics shows the password only as "(set)".

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

### Get it
1. Download **`MPCvibedRPC.exe`** below and run it from wherever you like. There is nothing to install. If you already have an earlier version, the Updates tab can install this one for you, or replace the exe by hand; your settings are kept.
2. Presence starts by itself: play something. If no player is answering, the window offers to set up the connection for you.
3. Optional: switch on **Start with Windows** to start quietly in the tray at login.

Needs Windows 10/11, a supported player (MPC-HC, MPC-BE, MPC-QT, mpv or VLC) and the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- The exe is **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn about it. Check it with the `.sha256` file, or with `gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC`. The program can update itself from GitHub releases (Updates tab).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.
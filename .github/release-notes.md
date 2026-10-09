## MPCvibedRPC 0.9.6

New in this version:

- **MPC-QT support.** Works out of the box: MPC-QT always offers an mpv-style connection, and the program reads it, so there is nothing to switch on. (If you turned MPC-QT's web interface on, that works too.)
- **mpv support.** mpv is read through its JSON IPC. **Set up the player connection** adds `input-ipc-server=mpvsocket` to the `mpv.conf` mpv reads (next to a portable mpv, or in `%APPDATA%\mpv`), keeps a name you already set, and never closes mpv: restart it once afterwards. The name is under Advanced (`mpvPipe`).
- **MPC-BE confirmed.** Tried with MPC-BE 1.9.1; the "untested" note is gone.
- The window names the player it found (MPC-HC, MPC-BE, MPC-QT or mpv), and **Copy diagnostics** includes it.
- The **Turn on the web interface** button is now **Set up the player connection**, since it also handles mpv.
- **Fix:** Quit from the tray could hang for good if Discord stopped responding mid-update. It now finishes within a few seconds.

Tried with MPC-BE 1.9.1, MPC-QT 26.07 and mpv 0.41.0 on Windows 11, besides MPC-HC.

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

### Get it
1. Download **`MPCvibedRPC.exe`** below and run it from wherever you like. There is nothing to install. If you already have an earlier version, the Updates tab can install this one for you, or replace the exe by hand; your settings are kept.
2. Press **Start presence**. If no player is answering, the window offers to set up the connection for you.
3. Optional: switch on **Start with Windows** to start quietly in the tray at login.

Needs Windows 10/11, a supported player (MPC-HC, MPC-BE, MPC-QT or mpv) and the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- The exe is **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn about it. Check it with the `.sha256` file, or with `gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC`. The program can update itself from GitHub releases (Updates tab).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.
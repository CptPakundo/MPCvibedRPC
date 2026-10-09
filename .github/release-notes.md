## MPCvibedRPC 0.9.4

New in this version:

- **Sharp on high-DPI screens.** The program now declares itself DPI-aware, so the tray icon and its menu are no longer blurry at 150% or 200% scaling. The icon has extra sizes (24, 40 and 64 pixels) for the in-between scale factors.
- **MPC-BE (untested).** MPC-BE has the same web interface as MPC-HC, so it should work. The **Turn on the web interface** button now also knows where MPC-BE keeps that setting. This is based on MPC-BE's source code and has not been tried on a real MPC-BE install yet; please report how it goes.

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

### Get it
1. Download **`MPCvibedRPC.exe`** below and run it from wherever you like. There is nothing to install. If you already have an earlier version, the Updates tab can install this one for you, or replace the exe by hand; your settings are kept.
2. Press **Start presence**. If MPC-HC isn't answering, the window offers to turn its web interface on for you.
3. Optional: switch on **Start with Windows** to start quietly in the tray at login.

Needs Windows 10/11, MPC-HC (or MPC-BE) and the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- The exe is **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn about it. Check it with the `.sha256` file, or with `gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC`. The program can update itself from GitHub releases (Updates tab).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.
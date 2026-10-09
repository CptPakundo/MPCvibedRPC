## MPCvibedRPC 0.9.1

Fixes a leftover: after the settings window was closed (or the program quit), Edge could stay running in the background. The window now runs in its own browser profile with Edge's background relaunch switched off, quitting the program closes that browser, and the next start clears up after a crash. Your regular Edge is never touched.

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

### Get it
1. Download **`MPCvibedRPC.exe`** below and run it from wherever you like. There is nothing to install. If you already have 0.9.0, the Updates tab can install this version for you, or replace the exe by hand; your settings are kept.
2. Press **Start presence**. If MPC-HC isn't answering, the window offers to turn its web interface on for you.
3. Optional: switch on **Start with Windows** to start quietly in the tray at login.

Needs Windows 10/11, MPC-HC and the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- The exe is **unsigned**, so Windows SmartScreen or your antivirus may warn about it. Check the download against `MPCvibedRPC.exe.sha256` if you want to be sure it is the published file.
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC`. The program can update itself from GitHub releases (Updates tab).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.
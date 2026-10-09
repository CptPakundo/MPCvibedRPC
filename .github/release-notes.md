## MPCvibedRPC 0.9.5

New in this version:

- **Your status now clears after a 30-minute pause (changed default).** Before, a paused video kept its status until you resumed. The setting is **Clear my status when paused for (minutes)** on the Privacy tab; set it to `0` to keep the old behaviour. It applies to existing installs that never changed it, too. The status comes back when you resume or seek.
- **Copy diagnostics** (Log tab). One click copies the version, Windows version, the settings that differ from the defaults and the recent log, for pasting into a bug report. Titles, file names, paths, URL queries and keys are left out, and the text is shown so you can check it first.
- **Refreshed screenshots** in the README, and a few small wording fixes.

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
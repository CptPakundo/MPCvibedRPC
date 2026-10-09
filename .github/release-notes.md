## MPCvibedRPC 0.9.3

New in this version:

- **Hide what I'm watching** (Privacy tab). One switch makes your status say only "Watching a video" with the progress bar: no title, cover, episode or buttons, and nothing is looked up online. The title is kept out of the window and the log too.
- **Never show these files** (Privacy tab). A list of folders, names or wildcards (such as `Private` or `*.xyz`). A listed file shows no status at all, is never looked up and is not named in the log.
- **Update notice.** When the daily check finds a newer version, a notification appears next to the tray icon (once per version). Click it to open the window.
- **Welcome card** for new installs, reminding you about Discord's "Share my activity" setting and pointing to the Privacy tab. Existing installs never see it.
- **A copy running from its own data folder** (a portable setup, a test) no longer touches the installed program's "Start with Windows" entry.

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

### Get it
1. Download **`MPCvibedRPC.exe`** below and run it from wherever you like. There is nothing to install. If you already have an earlier version, the Updates tab can install this one for you, or replace the exe by hand; your settings are kept.
2. Press **Start presence**. If MPC-HC isn't answering, the window offers to turn its web interface on for you.
3. Optional: switch on **Start with Windows** to start quietly in the tray at login.

Needs Windows 10/11, MPC-HC and the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- The exe is **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn about it. Check it with the `.sha256` file, or with `gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC`. The program can update itself from GitHub releases (Updates tab).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.
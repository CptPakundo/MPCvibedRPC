## MPCvibedRPC 0.9.2

New in this version:

- **Privacy tab.** Every online service the program can ask about a title (TMDB, IMDb, Cinemeta, TVmaze, AniList, Kitsu, MyAnimeList, Wikipedia, Bulbapedia) now has its own switch, with what it is used for and where requests go. A switched-off service is never contacted. One switch still turns all lookups off, so the program stays fully offline.
- **Clear my status when paused for N minutes** (off by default). A long pause no longer leaves "Paused" on your profile; the status comes back when you play again or seek.
- **Clear cover cache** button.
- **Preview.** While a status is showing, the window shows how the Discord card looks (title lines, cover, progress bar, link button), built from what was actually sent.
- **Verifiable downloads.** Release files now come with a signed GitHub build record. Check a download with `gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC`, or against the `.sha256` file.

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

### Get it
1. Download **`MPCvibedRPC.exe`** below and run it from wherever you like. There is nothing to install. If you already have an earlier version, the Updates tab can install this one for you, or replace the exe by hand; your settings are kept.
2. Press **Start presence**. If MPC-HC isn't answering, the window offers to turn its web interface on for you.
3. Optional: switch on **Start with Windows** to start quietly in the tray at login.

Needs Windows 10/11, MPC-HC and the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- The exe is **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn about it. The checksum and build record above let you check that it is the published file.
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC`. The program can update itself from GitHub releases (Updates tab).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.
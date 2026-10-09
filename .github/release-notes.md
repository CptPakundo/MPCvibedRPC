## MPCvibedRPC 0.9.7

Fixes and polish after testing on a real PC:

- **MPC-HC was shown as "MPC-QT".** Newer MPC-HC pages look like MPC-QT's in one detail; MPC-HC is recognised correctly again, and its playback speed is used again (it was treated as normal speed).
- **The cover now shows in the window's preview.** The "How it looks on Discord" card always fell back to the app icon because the page blocked outside images.
- **Safer web interface setup.** When **Set up the player connection** switches MPC-HC's or MPC-BE's web interface on, it now only answers this PC. The players' own default also opens it (a remote control and file browser) to your whole network. A web interface you had on already keeps your choice.
- **"Discord on standby"** instead of "Waiting for Discord" while nothing plays: Discord is only contacted once something plays, so it was never actually missing.
- Saving a setting that does not affect the status (such as Start with Windows) no longer briefly clears and reconnects your Discord status.
- Small wording fixes: the welcome card and README no longer ask you to press Start presence (it starts by itself), and Restore defaults says it also clears your Never show list.

> **Entirely AI-generated ("vibe coded").** This whole program was written by an AI (Claude, by Anthropic) in conversation with the repository owner, who wrote none of it, takes no credit for it and has not reviewed it line by line. No warranty, no promise that it is correct, secure or maintained. See the README.

### Get it
1. Download **`MPCvibedRPC.exe`** below and run it from wherever you like. There is nothing to install. If you already have an earlier version, the Updates tab can install this one for you, or replace the exe by hand; your settings are kept.
2. Presence starts by itself: play something. If no player is answering, the window offers to set up the connection for you.
3. Optional: switch on **Start with Windows** to start quietly in the tray at login.

Needs Windows 10/11, a supported player (MPC-HC, MPC-BE, MPC-QT or mpv) and the Discord desktop app (User Settings > Activity Privacy > "Share my activity" on).

### Good to know
- The exe is **unsigned** (no code-signing certificate), so Windows SmartScreen or your antivirus may warn about it. Check it with the `.sha256` file, or with `gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC` (a signed GitHub record that it was built from this repository).
- Settings are kept in `%LOCALAPPDATA%\MPCvibedRPC`. The program can update itself from GitHub releases (Updates tab).
- The idea and approach come from [angeloanan/MPC-DiscordRPC](https://github.com/angeloanan/MPC-DiscordRPC); see `LICENSE` and `THIRD-PARTY-NOTICES.md`.
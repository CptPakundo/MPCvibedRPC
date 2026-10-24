## MPCvibedRPC 0.9.43

**A release for the project's own checks. Nothing changes in what the program does.**

### Changed
- **Faster tests.** The engine's tests, which drive a fake Discord and fake players, now run side by side and ask their
  fakes every 25 ms instead of every 250 ms: the whole suite takes about a tenth of the time it did. The pace of the
  real program is unchanged (it still asks a player every 5 seconds by default, never more often than every 250 ms).
- **Checks that keep the texts true.** Four new tests fail when the documentation drifts from the code: every setting
  must have its row and default in `docs/config.md`, every text that lists the players must name them all, the release
  notes must be for the version being built, and the settings page's script must parse.
- **Two code analysers on every change.** `staticcheck` and `deadcode` run on the Linux CI job; the few findings they
  had are fixed (a stale field, a constant block in the wrong file, two style points).
- The exe's file description and the developer notes name every player.

Nothing to do: the update installs itself, and your settings and sign-ins are kept.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

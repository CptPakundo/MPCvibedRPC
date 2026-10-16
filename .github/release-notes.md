## MPCvibedRPC 0.9.25

**A small fix: quitting and updating always clean up after themselves.**

- When the program quits or installs an update, it removes the small file that tells a second copy where the running
  one is (`ipc.json` in the settings folder). On Windows, that removal could fail if another program was reading the
  file at that moment, and the file was left behind. The next start handled that fine, but now the program tries
  again for up to a second, so the file is always cleaned up.

Everything else is as in 0.9.24: settings in grouped cards, Updates inside General, and the version number at the top
opens the changelog.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time, System Settings >
  Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on. To check a
download, see [SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

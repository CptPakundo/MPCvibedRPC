## MPCvibedRPC 0.9.37

**More file names are recognised as episodes.**

### What's new
- **Episode codes in square brackets.** `Sample Show [S01E02].mkv` (or `[1x02]`) lost its episode number, because
  square brackets usually hold release tags. Now the episode is read, so its name is looked up like any other.
- **More episode forms.** `Sample Show S2E5` is shown as `S02E05`, a double episode written `E05-E06` (or
  `s01e05-e06`) is read as one file covering both, and `Sample Show Ep. 5` with a dot is an episode too.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

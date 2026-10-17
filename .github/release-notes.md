## MPCvibedRPC 0.9.36

**Specials named in words are found, and the weekly check covers the newest mpv.**

### What's new
- **Specials named in words.** A file such as `Sample Show - Special 02.mkv`, `Sample Show - OVA 2.mkv` or
  `[Group] Sample Show - SP2.mkv` used to find no cover when no catalog lists that special as a title of its own.
  The name is still searched as it is first (AniList lists many OVAs on their own, and those keep their own cover);
  when nothing matches, the program now searches the show and shows the special as its season 0: the show's cover
  and the special's name (`Season 0, Episode 2`, as a file named `S00E02` already gets). Also `OAD`, `ONA` and a
  season marker before it (`Sample Show S01 OVA 2`).
- **Wider weekly check.** mpv is now also tried every week as Flathub ships it (its makers' newest release), over its
  IPC socket, with a pause that must show at once, next to VLC and Celluloid.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

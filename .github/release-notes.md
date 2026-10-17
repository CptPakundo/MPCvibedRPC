## MPCvibedRPC 0.9.38

**Specials named in words, fixed for real catalogs, and double episodes.**

### What's new
- **Specials named in words, now with real catalogs.** 0.9.36 taught the program that `Sample Show - Special 01`
  is the show's special (season 0). A check against the real Cinemeta found two cases it still missed: with a year
  in brackets (`Sample Show (2010) - Special 01`) the special's words were dropped as an edition note, and when a
  catalog answers the whole name with the show itself, the show's cover came without the special's name. Both now
  show the special's name. The weekly check looks one real special up this way, so a change at the catalog is noticed.
- **Specials keep their own label.** Such a special now shows the file's own words (`OVA 3`, `Special 1`) on the
  cover and, when no catalog knows its name, as its second line, instead of `Season 0, Episode 3`. Shows that are
  themselves OVA series number their episodes that way, and the TV databases' season 0 may not agree. A show cached
  for such a file by an earlier version gets the special's name too.
- **Double episodes.** `Sample Show - S01E05 - S01E06` showed "S01E06" as the episode's title; it is now
  `S01E05E06`.
- **Security check.** CI's Go vulnerability check is updated to govulncheck 1.9.0.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

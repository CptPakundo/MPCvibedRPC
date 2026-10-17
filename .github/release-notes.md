## MPCvibedRPC 0.9.35

**Instant updates for Kodi and the Linux players, names for specials, and Plex's newer sign-in made ready.**

### What's new
- **Kodi and the Linux players: changes show at once.** Kodi and the players read through D-Bus on Linux (VLC,
  Celluloid, mpv with mpv-mpris and the like) now tell the program when they pause, resume, seek or change file, so
  that reaches Discord within a moment instead of at the next check, as mpv and IINA already did. Kodi sends these
  notifications on port 9090 of its address when *Allow remote control from applications on this system* is on (the
  default; *on other systems* for a Kodi on another device). Without it, Kodi is checked every 5 seconds as before.
- **Specials get their names.** A file named as a special (such as `Sample Show - S00E01.mkv`, as TV databases and
  media servers number them) now gets its episode name from Cinemeta without a TMDB key. Files without a season
  number are never counted onto a special. The many catalog services stay on purpose: every show, film and special
  should be found.
- **Plex's newer sign-in, ready but off.** Plex is moving to tokens that last 7 days and are renewed with a key of each
  app's own. The program can now sign in that way and renew the token in time, but it stays off (`plexJwt` in
  `config.json`) until Plex requires it: the long-lived sign-in keeps working, and Plex has set no end date.
- **Wider weekly check.** Besides Ubuntu's packages, VLC and Celluloid are now also tried every week as Flathub
  ships them (their makers' newest releases), and the check of the catalog services includes a special.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).

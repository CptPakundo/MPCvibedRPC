## MPCvibedRPC 0.9.42

**A maintenance release from a full checkup of the code: eight small fixes, nothing new to set up.**

### Fixed
- **The status appears at once.** When Discord is reached, the program showed what you were watching only at its
  next check (up to five seconds later). Now it shows it the moment the connection is made.
- **Automatic updates cannot crash the program.** An update that was waiting for a quiet moment and then called off
  (a newer check found nothing, or **Restore defaults**) could end the program when the moment came.
- **A stale "updated itself" note.** If the program was started by hand after an automatic update had not got going,
  the next update by hand was mistaken for an automatic one. The leftover is now cleared on every start by hand.
- **Plex: no sign-in left behind.** Signing out at the very moment a token renewal or a server change finished could
  leave a sign-in in the settings file with no account. Both now happen under the same lock as the sign-out.
- **Copy diagnostics hides more.** Besides the folders it already left out, it now also leaves out home folders
  (`~/...`), any Unix path of two or more parts, `file:` addresses, the addresses of computers on your network and the
  Kodi user name. Titles and file names stay, as before, so a wrong title can still be traced. A test now fails when
  a setting whose name says it is a sign-in, a key, a password or an address is not covered.
- **Faults in background tasks are logged.** A fault in the Discord connection, a server watcher, a sign-in or the
  update check used to close the program without a word; it now ends only that task and writes a line to the log.
- **Linux: a broken D-Bus message can no longer hang the program.** A message on the session bus with an empty
  dictionary entry kept the reader busy for ever; it is now refused like any other broken message.
- PotPlayer, Kodi and IINA are named everywhere the players are listed (the player order in the docs, the
  program's file description); dead code and stale comments from the checkup are gone.

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

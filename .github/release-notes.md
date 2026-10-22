## MPCvibedRPC 0.9.41

**A security update: newer certificate checks, and a tighter hold on your Plex sign-in.**

### Fixed
- **Built with Go 1.27.3.** Go's makers found a set of web certificates no longer safe to trust and published a fix
  on 2026-10-19 (GO-2026-6809). Every connection the program makes over https depends on these checks: plex.tv,
  Stremio, the catalogs, and GitHub, where updates come from.
- **Plex: your sign-in only goes where it belongs.** Before, the program sent your Plex sign-in along when it tried
  each address plex.tv lists for your server, so a different computer at a local address (on another network, say)
  could have received it. Now it first asks the address who it is, without the sign-in, and sends it only once the
  answer is your server. A server shared with you only ever gets the key plex.tv made for it, never your account's
  own sign-in.

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

# Security

This program has not been audited, so please treat it accordingly. It runs on your own
computer, talks to Discord and the player (MPC-HC, MPC-BE, MPC-QT, mpv, VLC, PotPlayer or IINA, or on Linux a video player
over D-Bus) locally, and to Kodi at the address you enter once you set its password (this computer by default). It only
contacts the online services listed in its **Privacy** tab, and GitHub to look for updates
(switch off under **General > Updates**). Once you sign in to Plex, it also
contacts plex.tv (to sign in and to find your servers) and your Plex server, which it only reads from. Once you sign in
to Stremio, it contacts Stremio's servers (link.stremio.com to sign in, api.strem.io to read your library and to sign
out). Once you enter a Jellyfin or Emby server's address and sign in (Accounts tab), it contacts only that server (to sign in, to read
its session list and to sign out). The server's access token is kept in `config.json` as plain text; a password you type
goes to your own server once and is never stored. Kodi's password is kept in `config.json` as plain text too, as VLC's is.
The player logos on Discord are loaded by Discord itself from this repository on GitHub.

If you find a security problem, please report it **privately** through
[GitHub's private vulnerability reporting](https://github.com/CptPakundo/MPCvibedRPC/security/advisories/new)
rather than in a public issue. There is no guaranteed response time or fix, but reports are read.

Release files come with a SHA-256 checksum and a signed build record from GitHub. To check that a downloaded
`MPCvibedRPC.exe` (or `MPCvibedRPC-macos.zip`, `MPCvibedRPC-linux-amd64`, ...) was built from this repository by its
release workflow:

```
gh attestation verify MPCvibedRPC.exe --repo CptPakundo/MPCvibedRPC
```

The program itself is not code-signed, so Windows SmartScreen may warn about it. The macOS app only has an ad-hoc
signature (no Apple Developer ID, not notarized), so macOS asks you to confirm its first start.

# Security

This program is entirely AI-generated and has not been audited, so please treat it accordingly. It runs on your own
computer, talks to Discord and the player (MPC-HC, MPC-BE, MPC-QT, mpv, VLC or IINA, or on Linux a video player over
D-Bus) locally, and only contacts the online services listed in its **Privacy** tab. Once you sign in to Plex, it also
contacts plex.tv (to sign in and to find your servers) and your Plex server, which it only reads from.

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

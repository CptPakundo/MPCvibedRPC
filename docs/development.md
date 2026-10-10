# Development

How MPCvibedRPC is built, tested and released. For using it, see the [README](../README.md).

## Building
The program is written in Go and uses only the standard library: one small file (about 8 MB, a few MB of memory while
running), no runtime, nothing to install. Building needs [Go 1.24+](https://go.dev/dl/) and no internet beyond that.

    go run ./tools/mkrsrc -version 0.9.11 -out cmd/mpcvibedrpc/rsrc_windows_amd64.syso    (icon + version details, optional)
    go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=0.9.11" -o MPCvibedRPC.exe ./cmd/mpcvibedrpc

`-H=windowsgui` makes it a windowed program, so no console flashes up. You can build from any OS (`GOOS=windows`).
Discord's local protocol is implemented in `internal/discord`.

The Linux program is built the same way (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w"
-o MPCvibedRPC-linux-amd64 ./cmd/mpcvibedrpc`). The macOS app is put together on a Mac by
`tools/ci/package-macos.sh VERSION OUTDIR` (both processor types in one program, the app icon, an ad-hoc signature, a zip).

## Testing
`go test ./...` runs everything: the parsers, artwork matching and activity builder are checked against recorded reference output
(tens of thousands of cases, stored as test data in each package's `testdata/`), the engine against a fake MPC-HC, a fake mpv, a fake VLC, a fake Plex and a fake Discord, and the
whole program is built and driven end to end. On GitHub, Linux runs the tests with the race detector, Windows and macOS run them too, each system's
program gets a smoke test, and the engine is run against real players on Linux (mpv, VLC, mpv-mpris, Celluloid, and a real Plex Media Server) and macOS (mpv, and IINA after the program has set it up).

Running a copy that cannot touch an installed one: `MPCRPC_HOME` gives it its own data folder (and its own
run-at-login entry), and `MPCRPC_BROWSER` names the browser to show the window in (one that understands `--app`), or
`none` for no window at all.

## Releasing
To publish a version:

1. `git tag v0.9.11 && git push --tags` - the workflow in `.github/workflows/release.yml` runs the tests, builds the
   programs on Windows, Linux and macOS runners, smoke-tests each of them (Windows: tray, Discord pipe, autostart,
   self-update; macOS and Linux: the API, login item, player setup, a second start, and on macOS opening the app like
   the Finder does) and attaches them with their `.sha256` files to a release. The tag is the version.

Before tagging: the version's lines go at the top of `internal/assets/changelog.json` (the window shows them once
after the update; a test checks that the newest entry is the version being built), `.github/release-notes.md` becomes
the release text, and the screenshots below are taken again.

## Screenshots
The README's pictures (`docs/images/window-*.jpg`) show the current version and are taken again for every release:

1. Build the program and start it with a throwaway data folder (`MPCRPC_HOME`), with presence off and
   `{"app": {"seenVersion": "<version>"}}` in its `config.json` so the What's new box stays closed.
2. Open its settings page (the address and token are in `ipc.json` in that folder) in a browser at 620 x 700 pixels,
   dark mode (a high-DPI screen gives sharper pictures).
3. For `window-main.jpg` paste `tools/screenshots/demo.js` into the page's console: it shows a made-up
   "Sample Show" playing and sample stats, and nothing is sent anywhere. Take the General tab. For
   `window-privacy.jpg` (Privacy tab, scrolled to "Services that may see your titles") and `window-plex.jpg`
   (Advanced tab, scrolled to Plex) reload the page first, so presence shows as off.
4. Save them 800 pixels wide (800 x 903). Only made-up titles; nothing from a real library or account.

## Where things are
| Path | Job |
|---|---|
| `cmd/mpcvibedrpc` | the program: single instance, wiring, log, flags (`--background`, `--after-update`; on macOS `--serve`, the copy that keeps running after the app is opened) |
| `internal/engine` | polls the player, looks up artwork, sends presence; start/stop/apply settings live |
| `internal/core` | settings defaults, filename parsing, the Discord activity itself |
| `internal/artwork`, `internal/jsre` | catalog lookups and episode titles; a regex engine with JavaScript semantics, which the title-matching rules rely on |
| `internal/discord` | Discord's local IPC protocol |
| `internal/mpvipc`, `internal/pipe` | mpv's JSON IPC (mpv and MPC-QT); named pipes on Windows, Unix sockets elsewhere |
| `internal/vlchttp` | VLC's web interface (read-only: `status.json`, `playlist.json`) |
| `internal/mpris`, `internal/dbus` | Linux video players through MPRIS; a small D-Bus client (session bus, method calls) |
| `internal/plex` | Plex: the plex.tv sign-in, the account's servers, and a server's play notifications (read-only) |
| `internal/store`, `internal/jsonx` | `config.json` in `%LOCALAPPDATA%`, the settings schema shown in the window, validation |
| `internal/server`, `internal/assets` | local settings page (`ui.html`) and API on `127.0.0.1`, protected by a per-launch token |
| `internal/winsys` | the operating system: run at login (registry, LaunchAgent, autostart entry), setting up the player connection (web interface, `mpv.conf`, `vlcrc`, IINA's settings), window, tray icon (Win32, no helper process); on macOS the menu bar icon and the settings window (`macapp_darwin.go`, Cocoa and WebKit, in the macOS build only) |
| `internal/updater` | GitHub release check, download, checksum, swap and restart |
| `tools/` | the resource (.syso) and icon writers and the CI scripts (smoke tests, live player tests, the macOS app) |
| `docs/` | the settings reference (`config.md`), how it works (`how-it-works.md`), this file, and the README's images (`images/`) |


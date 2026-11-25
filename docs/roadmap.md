# Roadmap

Where MPCvibedRPC is heading, what it deliberately leaves out, and the outside changes it has to watch. It comes from
a research pass over Discord's documentation, the projects the program reads from, similar presence tools and their
users, and distribution channels (last reviewed 2026-11-25, with 0.9.58).

Keep it current: when a change moves a goal, finishes a step, or settles a decision, update this page in the same pull
request. Re-check the watch list about twice a year (with each Go release, in February and August).

## Principles
Every goal has to fit these:
- **One job, done well**: show what you watch on Discord, accurately and nicely.
- **Tiny and self-contained**: one file, the Go standard library only, nothing to install, no server to run.
- **Private by design**: no telemetry, no account of its own, no image uploads, stats that are counts only, and every
  online service switchable off.
- **Honest**: openly made with AI, and exact about what was tested for real.
- **It acts by itself in the right situation**: a feature that can tell from what is happening should do the right
  thing on its own rather than be a switch someone must remember (the stream title and resting, in
  [decisions](decisions.md#behaviour)).
- **Windows first**: MPC-HC's behaviour never changes when something is added; macOS and Linux follow without
  costing Windows anything.

## Next up
Work anyone can pick up without a decision from the maintainer (work that waits on one is under [Open decisions](#open-decisions-the-maintainers)). Each entry says what and where, and how to tell it is
done. (Work in progress is a branch and pull request: [CONTRIBUTING](../CONTRIBUTING.md#picking-up-work).)
1. **A settings change restarts the run and reconnects to Discord** (`internal/engine/engine.go`, `ApplySettings`).
   Restart only what a setting needs, so the status does not blink and Discord's rate limit is not hit. Done when a
   change that touches no player or Discord setting leaves the connection open (an engine test against the fake Discord).
2. **Behaviour tests for the settings page's script** (`internal/assets/ui.js`): a few tests with a stubbed `fetch` for
   the saver and the status sync. Today only `TestPageScriptParses` (`node --check`) covers it.
3. **Tests for how the window's X quits** (`cmd/mpcvibedrpc/window.go`): the order of the dropped `/api/alive`
   connection and the window-handle polling. Also check whether `inTray` stays true when closing the window fails; if
   it does, a later X would not quit.
4. **Historic names in the code.** The API's `mpc-web`, `winsys.EnableMpcWebInterface` and `Status.MPC` cover every
   player. Rename them; the page and the server ship together, so they change together. Done when no name says MPC for
   a thing that serves all players.
5. **Split `core.Config`.** User settings, sign-in secrets and test hooks are one flat struct, compared with
   `reflect.DeepEqual` to decide a restart (`engine.go`). Separate them so a secret or a hook cannot cause a restart.
6. **The settings window in Chrome, Brave or Vivaldi** instead of Edge: does the taskbar identity (name, pin, icon)
   apply there?
7. **MPC-QT's multi-window mode** with a portable build and the live engine test.
8. **The stream title on Linux:** research how to tell that Discord shares the screen (PipeWire, the screen-cast
   portal). Today Linux chooses always or never ([decision](decisions.md#behaviour)).
9. **CI: the Chocolatey Kodi download** (TLS errors from its mirror) fails `players-windows` now and then, and the
    canary uses it. Add a retry or another source ([known flaky checks](development.md#known-flaky-checks)).
10. **Fix a match, first version** (settled 2026-11-25; part of version 0.9.60). Build what
    [design/fix-a-match.md](design/fix-a-match.md) describes: the dialog and the list with the scope choice, covers from
    the other catalogs, offsets and "Tell the project". *Where:* `internal/artwork`, the `cmd/mpcvibedrpc` API,
    `internal/assets` (the window), `core.Config` and the store schema (`matchFixes`), the docs. Done when the dialog and
    the list replace the two text boxes, old `artworkAliases` and `artworkOverrides` lines still act as before (they
    become fixes once), and the tests the design names pass.
11. **Presence templates, first version** (settled 2026-11-25; part of version 0.9.60). Build
    [design/presence-templates.md](design/presence-templates.md): the Text lines row with Standard, Short, Detailed and
    Custom, including a film's cover text. *Where:* `internal/core` (`lines.go`, `BuildActivity`), the store schema
    (`textLines`, `customLines`), `internal/assets` (Look tab), the docs. Done when Standard reproduces the recorded
    golden data byte for byte, Custom lines show on the card, and the tests the design names pass.
12. **The update notices on Linux and macOS** (settled 2026-11-25; part of version 0.9.60). Windows already shows three
    native notifications through the tray (`Tray.Balloon`): "updated to <version>" once after an automatic update
    (`cmd/mpcvibedrpc/app.go`, `autoFrom`), "could not update to ..." after a rollback, and "<version> is available"
    when automatic installs are off (`update.go`). Linux and macOS show none (`Balloon` returns false in
    `internal/winsys/tray_other.go` and `macapp_darwin.go`). Bring the same three there: Linux through the desktop
    notification service over D-Bus (`org.freedesktop.Notifications`, with `internal/dbus`), macOS through the menu bar
    app (cgo; tested on CI only). A click opens the settings window as on Windows. Done when each system shows the
    three notices, tests cover the D-Bus message against a fake bus, and the docs and changelog say so.
13. **Sign-ins encrypted on Windows** (settled 2026-11-25; part of version 0.9.60). Encrypt the secrets in `config.json`
    with the per-user Windows DPAPI (`CryptProtectData` / `CryptUnprotectData` through `syscall`, standard library only):
    `plexToken`, `plexKey`, `stremioKey` and `jellyfinToken` (the sign-ins), and, to decide while building, the secrets
    typed in the window (`vlcPassword`, `kodiPassword`, `tmdbApiKey`). *Where:* `internal/store` (load and save),
    `core.Config`, SECURITY.md, `docs/config.md`. Existing plain values are read as before and encrypted at the next
    write; a config copied to another user or computer cannot be decrypted, so those sign-ins count as signed out and
    ask to sign in again; macOS and Linux are unchanged. Done when a saved config holds no plain secret on Windows, a
    plain one still loads, an undecryptable one reads as signed out (a test with a fake decrypter), and Diagnostics
    still leaves the values out.

## Goals

### 1. Keep working when nobody is looking
The services and players the program reads change without notice: in one month a catalog service closed, Stremio
changed how often it saves playback, and Jellyfin, IINA and Kodi shipped new major versions.
- Done when no outside change goes unnoticed for more than a week.
- Steps: a weekly scheduled CI run of the live player and server tests against the latest releases; a small check per
  catalog service (one known title, the answer's shape); the Go vulnerability check; each failure opens an issue.
- *Status:* done. The weekly run, the catalog check and the issue (`canary.yml`); VLC, Celluloid and mpv are also tried
  as Flathub ships them (their makers' newest releases, `live-flathub.sh`).

### 2. Own the foundations
- **Discord application**: the default application ID belongs to the project this one started from, which has been
  inactive since 2022. If it is deleted, every install loses its status at once. Register a project-owned application
  and keep the old ID as a fallback. *Needs the maintainer.*
- **Go toolchain**: move to the newest Go release every February and August; the supported operating systems follow
  Go's (Go 1.27: macOS 13 or later).
- **Plex sign-in**: Plex is moving to short-lived tokens (7 days, Ed25519 device keys). Old tokens still work and no end
  date is announced. Prepare the new sign-in with the standard library, and ship it when Plex requires it.
  - *Status:* prepared. `plexJwt` (off by default) signs in with a key of the install's own and renews the 7-day
    token; tested only against a fake plex.tv built from developer.plex.tv.
  - *Left:* a real sign-in (needs the maintainer's OK), then turning it on by default.

### 3. Updates you can trust without a personal signature
The updater checks a checksum from the same release, so anyone able to publish a release could ship to every install.
- Steps: a release manifest signed with an Ed25519 key that the maintainer keeps offline, whose public key is built into the
  program; GitHub's immutable releases; CI actions pinned by commit and write permissions only where needed; a CI job
  that rebuilds the Windows and Linux files from the tag and compares them byte for byte.
- *Status:* done: the pins, read-only workflows by default, and the rebuild (`tools/ci/reproduce.sh`; a release is only
  published when every program file matches).
- *Left:* the signed manifest (needs the maintainer's key) and immutable releases (open decision 2).
- Not planned: checking GitHub's build attestations inside the program (much security code, a moving format, narrow
  benefit). The attestations stay available to check by hand.

### 4. Private by design, and provably so
- One list of every host the program may contact, used by the Privacy tab and SECURITY.md, and a test that fails when
  the code contacts a host not on it.
  - *Status:* done: `core.Hosts` with `core.Sources`, the table in SECURITY.md (linked from the Privacy tab), and
    `TestEveryHostIsListed`, which scans the code and the page for addresses.
- Optionally, a "recent connections" view: host, count and time, never titles.
  - *Status:* done (Privacy tab; every request through Go's default transport is counted by host, in memory only).
- MPC-HC's web interface answers any web page once switched on (no password, open to all origins). The Privacy tab and
  SECURITY.md say so.
- Keep sign-ins out of plain text in `config.json`.
  - *Status:* settled 2026-11-25 as Windows' per-user DPAPI encryption (Next up 13); macOS and Linux stay as they are,
    and the full system key store (the macOS Keychain, the Secret Service) is not planned.

### 5. Readable by a human, decisions in the open
- A public record of the product decisions and of the non-goals below, with the reason for each.
  - *Status:* done: `docs/decisions.md` (the non-goals stay below).
- A short note on how AI was used (a file, not a banner) and an AI and pull-request section in CONTRIBUTING.md.
  - *Status:* done: `docs/ai.md`, linked once from the README.
- Size limits: no dependencies, short functions.
  - *Status:* two code analysers (staticcheck, deadcode) run on every change, and the four longest functions (the
    activity builder, the engine's check, the player order and the player setup) are split into named steps.

### 6. Depth in the core, not breadth
- Sturdier catalog services, without losing coverage. The many catalogs are there on purpose (the maintainer's
  intent, 2026-10-18): every show, film, special or the like should be found. Never drop a catalog that finds titles
  the others miss; make each one more reliable, and fill gaps (such as specials) where a catalog has the data.
  - *Status:* specials (S00E01) get their names from Cinemeta's season 0, without a TMDB key. Specials named in words
    ("Special 02", "OVA 2") that no catalog lists on their own are found as the show's season 0.
- "Why this title?" in Copy diagnostics.
  - *Status:* done: the file name, what was read from it, which catalog matched (or the closest results) and where the
    episode title came from.
- A title-language choice for anime (the services already return every variant).
  - *Status:* done for AniList matches (`animeTitles`: auto, English, romaji or the file's spelling).
- React to player events where players offer them (mpv, Kodi, Linux MPRIS), so pause and seek show at once.
  - *Status:* done for mpv and IINA, Kodi (its notifications on port 9090) and MPRIS players (D-Bus signals)
    (`engine/events.go`).
- macOS stays "best effort": the maintainer has no Mac, CI and testers are the test bed.
- Later, if asked for: a native Windows ARM64 build, translatable window texts, a Linux tray icon.

## Not goals
Each of these sank or burdened a similar tool, or breaks a principle:
- Uploading or hosting cover images (image hosts break and see private libraries).
- Accounts, telemetry, licence servers, funding pop-ups.
- Signing in to Discord alternatives with users' login tokens; only the local Discord connection is used.
- A browser extension or streaming websites.
- Reading window titles, OCR, or detecting "any app" by default. A Windows media-session reader only as an opt-in with
  an allowlist, if ever.
- Built-in scrobbling (Trakt, Simkl, AniList); at most a local event hook that other tools can read.
- Watch-together features (they need servers) and a stored history of titles (the stats stay counts only).
- A Microsoft Store edition (its sandbox breaks the MPC-HC setup and self-update) and Flathub (its rules on
  AI-generated apps and its sandbox), unless real demand appears.

## Considered and left
Ideas weighed and not done, each with its reason, so nobody has to weigh them again:
- **A `.gitattributes` for line ends:** `eol=lf` would change every Windows working copy at once, and `* text=auto`
  makes Git use `core.eol` (CRLF on Windows) even where `core.autocrlf` is off, which gave the CI's Windows build a
  CRLF checkout and broke the reproducible exe (tried once, reverted). Without one, each checkout follows its own
  `core.autocrlf`.
- **Pinning the live tests' images and player versions in pull-request runs:** pins need upkeep, and the weekly canary
  tries the newest versions anyway.
- **Building with `-tags nethttpomithttp2`:** saves about 0.6 MB, not worth the divergence.
- **UPX compression:** antivirus programs flag compressed executables.
- **Fetching episode lists less often than every 10 minutes:** that is the cache's life; a longer one delays a new
  episode's title.
- **MPRIS keeping its bus connection** instead of opening one per check: a kept connection adds states that only CI
  could test.
- **Stremio's 403 during a sign-in:** what the real service answers is unknown without a real account.
- **Ending the window's browser politely** instead of with `TerminateProcess`: a polite close makes the window go more
  slowly at Quit.
- **On macOS, the menu bar app wakes four times a second, and a logout skips the clean shutdown:** both are cgo code
  that only CI can test.
- **An episode number in the basic fallback status:** the recorded golden data pins it without one.

## Open decisions (the maintainer's)
1. Register a project-owned Discord application.
2. Turn on immutable releases (a repository setting; [how they fit the release flow](research/immutable-releases.md)).
3. Keep an offline signing key and sign each stable release (one manual step each).
4. Turn `plexJwt` on by default. It needs one real Plex sign-in to test it first (goal 2), which needs someone with a Plex
   account to try it; the maintainer has none.
5. A popup for a found update when no settings window is open. Today the program updates itself silently, or shows the
   tray notice when automatic installs are off.

Settled: the macOS minimum is 13. On 2026-11-25 the maintainer settled the Fix a match redesign, presence templates (both in Next up), the notice after an automatic update (on by default) and the sign-ins' storage: encrypted with Windows' DPAPI on Windows, plain on macOS and Linux, not the full key store (goal 4).

## Watch list
Outside facts the program depends on, with the date they were last checked.

| Topic | State (checked 2026-10-17 unless a row says another date) | What would change things |
|---|---|---|
| Discord local presence | Documented and supported without sign-in (changelog 2026-04-03; Android since 2026-07-28). Settings group "Activity" renamed "Games & Apps" (2026-08-04). No later change to Rich Presence in the developer changelog (checked 2026-11-11). | Any move to signed-in-only presence, or limits on Watching or the activity name for non-games. |
| Discord stream labels | A screen share is named after the streamer's first *Playing* activity, never a Watching one, and a profile shows one card per application, Playing first (checked 2026-11-21; [decision](decisions.md#behaviour)). The program tells a share from the thread `RsVideoSource` in Discord's app (Discord 1.0.9267, checked 2026-11-11). Windows' capture record (`ConsentStore\graphicsCaptureProgrammatic`) is no longer written to the registry on Windows 11 25H2 and later (it moved to a database only administrators can read). `SCREENSHARE_STATE_UPDATE` needs `rpc.screenshare.read`, which only approved applications get. Which of two Playing statuses names a stream is not documented. What each field shows on a card: [research](research/discord-rich-presence.md). | Discord naming streams after Watching activities too (the setting could then go), Discord opening the screen-share event to local programs, Discord renaming that thread (then `auto` sees no stream: Help > Run a check shows it), or a way to tell on macOS and Linux. |
| Go and operating systems | Go 1.27 needs macOS 13; Windows 10 stays supported. Discord dropped macOS 11 and 12 in 2026. Go 1.27.3 (2026-10-19) distrusts a set of CA intermediates (GO-2026-6809); the builds use it (checked 2026-10-22). Go 1.27.4 fixes a crash of HTTP servers that limit a request's size, as the settings page's server does (GO-2026-6859, published 2026-11-11); the builds use it. Go 1.27.4 is still the newest release (checked 2026-11-11). | Each Go release's notes (February, August), and govulncheck on CI for security releases between them. |
| Plex | New short-lived tokens since 2025-09; old tokens still accepted. "Plex2Shell" (CVE-2026-96652/96654/96655/96656): Plex Media Server 1.43.2 and older can be taken over without a sign-in and give away the server owner's token; fixed in 1.43.3.10861, exploited since 2026-10-17 (checked 2026-10-22). Not a breach of plex.tv. The program sends a server a token only after it has shown its identity, and never the account's token to a shared server. Plex's download feed lists Plex Media Server 1.43.5.11079 as current (checked 2026-11-11). | A retirement date for old tokens; a breach of plex.tv itself (then everyone signs in again). |
| Jellyfin, Emby | Jellyfin 12 removed the old headers and routes; only endpoints in its OpenAPI spec are kept between majors. Jellyfin 12.0 came out 2026-09-07 and 12.4 on 2026-10-26 (checked 2026-11-11). | A new Jellyfin major; Emby API changes. |
| Stremio | Saves playback every 180 s in newer apps (90 s in older ones); both handled. stremio-core-web 0.65.1 (2026-10-12) says it "pushes library items every 180 seconds"; the constant itself was not read (checked 2026-11-11). | Another change to `PUSH_TO_LIBRARY_EVERY` in stremio-core. |
| Catalog services | The public Jikan API closed on 2026-10-01: `jikan` left the default source lists, and a self-hosted Jikan still works (`jikanBase`). IMDb's suggestion endpoint is unofficial; Wikimedia and AniList rate-limit; TMDB and TVmaze require credit (given). On 2026-10-17 TVmaze's search took 30 to 55 s for any query it had not cached (then cached for an hour; an aborted request does not warm it; shows and episodes answered at once): the program waits for it in the background. And Bulbapedia refused GitHub's runners (HTTP 403), so the canary cannot check it. | A service failing the weekly check (goal 1). |
| Players | VLC 4 still unreleased; Kodi 22 in release candidates (RC2 on 2026-11-10); IINA 1.5 has a new settings window (1.5.1 on 2026-11-06). Newest releases seen 2026-11-11: MPC-HC 2.8.5, MPC-BE 1.9.3, mpv 0.42.0, MPC-QT 26.07. | New majors of any supported player. |
| Discord alternatives | Fluxer, Stoat and Matrix have no presence API for local apps; Vesktop and similar clients offer Discord's local connection. | An official, sign-in-free presence API elsewhere. |

## Sources
- Discord: [developer changelog](https://docs.discord.com/developers/change-log),
  [Rich Presence without sign-in](https://docs.discord.com/developers/discord-social-sdk/development-guides/setting-rich-presence),
  [patch notes 2026-08-04](https://discord.com/blog/discord-patch-notes-august-4-2026),
  [macOS 12 support](https://support.discord.com/hc/da/articles/42846930613783--Known-Issue-Support-for-macOS-12)
- Go: [release history and policy](https://go.dev/doc/devel/release), [Go 1.27 notes](https://go.dev/doc/go1.27),
  [vulnerability list](https://pkg.go.dev/vuln/list)
- Services: [Jikan closing](https://github.com/jikan-me/jikan-rest/issues/613),
  [Stremio 180 s](https://github.com/Stremio/stremio-core/pull/1079),
  [Plex JWT](https://forums.plex.tv/t/jwt-authentication/931646),
  [Jellyfin 12](https://github.com/jellyfin/jellyfin/releases/tag/v12.0),
  [TMDB attribution](https://developer.themoviedb.org/docs/faq),
  [MPC-HC web server note](https://github.com/clsid2/mpc-hc/pull/4053)
- Distribution: [GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases),
  [Flathub requirements](https://docs.flathub.org/docs/for-app-authors/requirements),

# Roadmap

Where MPCvibedRPC is heading, what it deliberately leaves out, and the outside changes it has to watch. It comes from
a research pass over Discord's documentation, the projects the program reads from, similar presence tools and their
users, and distribution channels (last reviewed 2026-10-17, with 0.9.31).

Keep it current: when a change moves a goal, finishes a step, or settles a decision, update this page in the same pull
request. Re-check the watch list about twice a year (with each Go release, in February and August).

## Principles
Every goal has to fit these:
- **One job, done well**: show what you watch on Discord, accurately and nicely.
- **Tiny and self-contained**: one file, the Go standard library only, nothing to install, no server to run.
- **Private by design**: no telemetry, no account of its own, no image uploads, stats that are counts only, and every
  online service switchable off.
- **Honest**: openly made with AI, and exact about what was tested for real.
- **Windows first**: MPC-HC's behaviour never changes when something is added; macOS and Linux follow without
  costing Windows anything.

## Goals

### 1. Keep working when nobody is looking
The services and players the program reads change without notice: in one month a catalog service closed, Stremio
changed how often it saves playback, and Jellyfin, IINA and Kodi shipped new major versions.
- Done when no outside change goes unnoticed for more than a week.
- Steps: a weekly scheduled CI run of the live player and server tests against the latest releases; a small check per
  catalog service (one known title, the answer's shape); the Go vulnerability check (done in 0.9.31); each failure
  opens an issue.

### 2. Own the foundations
- **Discord application**: the default application ID belongs to the project this one started from, which has been
  inactive since 2022. If it is deleted, every install loses its status at once. Register a project-owned application
  and keep the old ID as a fallback. *Needs the owner.*
- **Go toolchain**: move to the newest Go release every February and August; the supported operating systems follow
  Go's (done for Go 1.27 in 0.9.31: macOS 13 or later).
- **Plex sign-in**: Plex is moving to short-lived tokens (7 days, Ed25519 device keys). Old tokens still work and no end
  date is announced. Prepare the new sign-in with the standard library, and ship it when Plex requires it.

### 3. Updates you can trust without a personal signature
The updater checks a checksum from the same release, so anyone able to publish a release could ship to every install.
- Steps: a release manifest signed with an Ed25519 key that the owner keeps offline, whose public key is built into the
  program; GitHub's immutable releases; CI actions pinned by commit and write permissions only where needed; a CI job
  that rebuilds the Windows and Linux files from the tag and compares them byte for byte.
- Not planned: checking GitHub's build attestations inside the program (much security code, a moving format, narrow
  benefit). The attestations stay available to check by hand.

### 4. Reach the people it is for, calmly
Trust on Windows is built per file: SmartScreen reputation starts again with every build, SignPath approves every
release by hand, and winget needs one pull request per version. Frequent releases work against all three.
- Steps: a stable release about once a month, pre-releases in between, and a 1.0 that promises calm maintenance;
  winget (it marks its downloads as trusted), a Scoop bucket and a Homebrew tap of the project's own (Homebrew's
  official cask list does not accept unsigned apps since 2026-09-01); announcements written by hand in the communities
  of the supported players; then SignPath Foundation's free signing once the project is visibly used (it declines
  projects without independent references).

### 5. Private by design, and provably so
- One list of every host the program may contact, used by the Privacy tab and SECURITY.md, and a test that fails when
  the code contacts a host not on it.
- Optionally, a "recent connections" view: host, count and time, never titles.
- MPC-HC's web interface answers any web page once switched on (no password, open to all origins). Say so in the
  Privacy text, and offer MPC-HC's maintainers a fix.
- Possibly, keep sign-ins in the system's key store (Windows DPAPI, the macOS Keychain, the Secret Service on Linux)
  instead of plain text in `config.json`.

### 6. Readable by a human, decisions in the open
- A public record of the product decisions and of the non-goals below, with the reason for each.
- A short note on how AI was used (a file, not a banner) and an AI and pull-request section in CONTRIBUTING.md.
- Size limits: no dependencies, short functions.

### 7. Depth in the core, not breadth
- Fewer, sturdier catalog services rather than more.
- "Why this title?" in Copy diagnostics.
- A title-language choice for anime (the services already return every variant).
- React to player events where players offer them (mpv, Kodi, Linux MPRIS), so pause and seek show at once.
- macOS stays "best effort": the owner has no Mac, CI and testers are the test bed.
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

## Open decisions (the owner's)
1. Release rhythm: monthly stable releases with pre-releases in between?
2. Register a project-owned Discord application.
3. Turn on immutable releases (a repository setting).
4. Keep an offline signing key and sign each stable release (one manual step each).
5. Going public: announcements, asking the original project for a link, package-manager listings.
6. Raise the open MPC-HC web interface with its maintainers.

Settled: the macOS minimum is 13 (0.9.31).

## Watch list
Outside facts the program depends on, with the date they were last checked.

| Topic | State (checked 2026-10-17) | What would change things |
|---|---|---|
| Discord local presence | Documented and supported without sign-in (changelog 2026-04-03; Android since 2026-07-28). Settings group "Activity" renamed "Games & Apps" (2026-08-04). | Any move to signed-in-only presence, or limits on Watching or the activity name for non-games. |
| Go and operating systems | Go 1.27 needs macOS 13; Windows 10 stays supported. Discord dropped macOS 11 and 12 in 2026. | Each Go release's notes (February, August). |
| Plex | New short-lived tokens since 2025-09; old tokens still accepted. | A retirement date for old tokens. |
| Jellyfin, Emby | Jellyfin 12 removed the old headers and routes; only endpoints in its OpenAPI spec are kept between majors. | A new Jellyfin major; Emby API changes. |
| Stremio | Saves playback every 180 s in newer apps (90 s in older ones); both handled since 0.9.31. | Another change to `PUSH_TO_LIBRARY_EVERY` in stremio-core. |
| Catalog services | Jikan closed 2026-10-01 (removed in 0.9.31). IMDb's suggestion endpoint is unofficial; Wikimedia and AniList rate-limit; TMDB and TVmaze require credit (added in 0.9.31). | A service failing the weekly check (goal 1). |
| Players | VLC 4 still unreleased; Kodi 22 in release candidates; IINA 1.5 has a new settings window. | New majors of any supported player. |
| Distribution | Homebrew rejects unsigned casks (2026-09-01); SignPath asks for visible reputation; Flathub requires disclosure of AI-generated code. | Policy changes at these channels. |
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
- Distribution: [SignPath Foundation terms](https://signpath.org/terms.html),
  [Homebrew and unsigned casks](https://github.com/Homebrew/brew/issues/20755),
  [GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases),
  [Flathub requirements](https://docs.flathub.org/docs/for-app-authors/requirements),
  [winget portable installs](https://github.com/microsoft/winget-cli)

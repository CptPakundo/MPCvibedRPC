# Decisions

The product decisions behind the program, each with its reason, so that a change can be weighed against why things
are the way they are. What the program deliberately leaves out is under [Not goals](roadmap.md#not-goals) in the
roadmap; open questions are its [Open decisions](roadmap.md#open-decisions-the-owners).

## Behaviour
- **MPC-HC comes first.** It is the player the program was made for: anything new is added after it and must not
  change how it behaves. *Why:* the people who use it with MPC-HC should never notice that other players arrived.
- **Discord is only contacted once something plays** ("Discord on standby" until then). *Why:* nothing to show means
  nothing to send, and an idle program should not hold a connection.
- **Resting while Discord is closed has no setting.** With Discord closed (and the window closed), no player is asked
  and the account watchers stop. *Why:* the status only reaches your profile through the Discord app, so following
  players without it would do nothing; a switch for it would be meaningless.
- **The status clears itself after 30 minutes of pause** by default (adjustable, 0 = never). *Why:* a forgotten paused
  video should not show for hours, while a short break should not make it vanish.
- **Music is shown as "Listening to"** with its file name, never looked up and not counted. *Why:* the catalogs are for
  films and shows; sending song names to them would be wrong answers and needless data.
- **Many catalogs, on purpose.** Titles are looked up in several services (TMDB with your key, IMDb, Cinemeta,
  TVmaze, AniList, Kitsu, Wikipedia, Bulbapedia), each with its own switch. *Why:* every show, film, special or the
  like should be found, and no single catalog has them all; a slow or closed service must not leave a title without
  its cover. A catalog is only removed when it stops answering for good.
- **The title is shown on a stream by itself** (2026-11-11). Discord names a screen share after the streamer's
  *Playing* status and no other kind, so while Discord shares the screen the status is sent as "Playing <title>", and
  as the usual Watching status the rest of the time. On Windows the program tells that from the threads Discord's app
  runs while it shares, and from Windows' record of screen captures where that can be read; where it cannot tell
  (macOS, Linux) the choice is always or never. *Why:* a switch to remember
  before every stream is friction; the Watching status with its progress bar is what belongs on a profile; showing
  both at once would put two cards there. It is on by default because it changes nothing until a stream starts.
- **One missed answer is not a closed player** (2026-11-11). While a status is up, a check in which no player
  answers is repeated a second later before the status is taken down. *Why:* a player that loads a file or seeks on
  a slow disk misses an answer now and then, and the status should not flicker for it; a second is a small price.
- **A status cleared after a long pause comes back only through the player** (resume, seek, another file), not when
  Discord restarts or a cover arrives late. *Why:* the user left it cleared; Discord restarts for every update of
  its own.
- **Discord's refusals hold for one title.** A title Discord refuses as a Watching status, or with its links, gets
  the simpler form; the next title is tried in full again. *Why:* a refusal can be about one title's text or
  address, and must not cost every later title its look.
- **New behaviour that changes things for existing users is off by default** unless decided otherwise. *Why:* an update
  should not surprise anyone.

## The window and the tray
- **Starting with Windows goes straight to the tray**; opening the program by hand shows the window (unless "Show this
  window when I open the app" is off); the very first start always shows it with a welcome card. *Why:* a start at
  sign-in should be silent, and opening it yourself means you want to see it.
- **Settings save as they change**; Restore defaults asks first. *Why:* no Save button to forget, and only the one
  action that loses settings needs a confirmation.
- **Opening the settings while they are open brings that window to the front** (Windows; also from the tray and
  from starting the program a second time) instead of opening another. On Linux, and on a macOS build without the
  native window, a second window still opens. *Why:* two windows each keep their own copy of the values, and a change
  in one does not show in the other.
- **The window closes the instant the program quits**, and quitting leaves no browser processes behind. *Why:* a
  settings page that outlives its program only confuses.
- **"Start with Windows" follows Windows' own switch too.** An entry turned off in Task Manager > Startup apps shows
  as off, and only turning the setting on in the window turns it back on; the program's own refresh of the entry at
  each start never does. *Why:* the user's choice in Windows is a choice.
- **The window is opened with a one-time code**, not with the key that guards the local API. *Why:* a browser's
  command line can be read by other users of the same computer for as long as it runs; a code is spent by the
  browser's first request.
- **On macOS the program lives in the menu bar and shows its settings in a window of its own**, never in a browser.
  *Why:* a browser window behaved badly for a tester, and a menu bar app is what a Mac user expects.
- **Updates install themselves by default** (2026-10-18; Windows and Linux): checks every six hours, and a found
  update is installed at the next quiet moment (nothing playing, no settings window open), then a notice says so once.
  The tray and menu bar icon show a red dot while an update waits. One switch turns it off. *Why:* users on old
  versions miss fixes; a restart while nothing plays interrupts nobody, and every update is checksummed. The copy that
  installed an update stays until the new version has come up, and puts the old one back if it has not (2026-11-11):
  a version that does not start must not leave anyone without the program, and is not installed by itself again.
- **A found update asks in the open window** (2026-11-11): what is new, Update now or Not now, once for each version;
  the check goes out as the program starts. With no window open nothing is asked: the update installs itself as
  above, though never in the first ten seconds after the start. *Why:* a line at the top and a button on a tab were
  easy to miss; whoever has the window open can decide on the spot, and a program that has only just started cannot
  know yet whether something plays or a window is about to open.

## Privacy
- **Copy diagnostics keeps titles and file names**, and leaves out folder paths, web address details, server names,
  sign-ins, keys and the Never show list. *Why:* a wrong title can only be traced back to the name it was read from;
  everything else that could name a person stays out.
- **Stats are counts and times only, on your computer.** *Why:* a history of titles is exactly what a privacy-minded
  user does not want stored, and nothing about it is ever sent.
- **The cover cache forgets a title 90 days after it was last played.** *Why:* the cache speeds up lookups, but it
  must not turn into a list of everything ever watched.
- **Every online service can be switched off**, and one switch makes the program fully offline. *Why:* each lookup
  sends a title somewhere; the user decides where.

## Code
- **Standard library only, including the Windows plumbing** (reaffirmed 2026-10-25). The tray, window, registry and
  process code in `internal/winsys` is written against Go's `syscall` package by hand rather than the Go team's
  `golang.org/x/sys/windows`. *Why:* "standard library only" is a promise the README makes, and the hand-written code
  was audited correct (layouts, handles, bounds) and is exercised by the Windows smoke test on every run. The question
  comes back only if that code has to grow materially.

## Releases
- **Versions stay 0.9.x, three-part.** *Why:* the updater compares three numbers; a fix takes the next number.
- **The README shows GitHub's own download count** (2026-10-25); the project keeps its exact count of people on the
  side. Since 2026-10-18 the `stats` branch holds that count: program downloads only (not checksums or notices), from a
  reset of GitHub's counts, minus every download the project makes itself (release checks and Claude agents record
  theirs in a ledger there). The program never reports anything; your own copy behaves like everyone's. *Why:* the
  badge every repository shows is GitHub's, and a number of its own is not worth explaining on the front page; the exact
  count stays readable (`summary.json` on the `stats` branch) for when it matters.
- **Frequent releases while the program is young** (2026-10-18): each real improvement ships once its checks pass.
  *Why:* few people use it yet, so quick fixes and feedback count for more than the trust that calmer releases build
  with Windows SmartScreen and package managers. Bundling comes back once it has a wider audience.
- **The release text is the changelog** (2026-10-24): the GitHub release's body is rendered from the in-app changelog's
  newest entry (`tools/relnotes`), plus a hand-written block only when a release needs one (a security note). *Why:*
  the same facts were written twice, in two wordings, for every release, and nothing checked that the second text was
  for the right version.
- **Live tests once per pull request** (2026-10-24): the jobs that install real players and real media servers run
  when a pull request is opened, reopened or marked ready, by hand, and in the weekly canary; every push runs the unit
  tests (with the race detector on Linux), the builds, the smoke tests and the reproducible-build check. *Why:* the
  live jobs were half of every push's CI minutes and ran about three times per branch; once per pull request keeps the
  "live tests before a release" guarantee, since every release goes through a pull request. Commits pushed after the
  pull request was opened are covered by running `ci.yml` by hand (`workflow_dispatch`) on the branch before a
  release, when they touch the engine or a player reader.
- **A release stays a draft until it checks out**: its files, checksums, build records, a rebuild from the tag that
  must match byte for byte, and then a real self-update from the release before. *Why:* nobody's updater should ever
  see a broken release.
- **The macOS minimum is macOS 13.** *Why:* the Go version the program is built with needs it, and so does Discord.
- **Not code-signed for now.** A code-signing certificate names a person or company; the plan is SignPath
  Foundation's free signing for open-source projects once the project qualifies. *Why:* a personal signature is not
  wanted, and the release checks above already let anyone verify a download.
- **Made with AI, and said so once**, near the top of the README (more in [how AI was used](ai.md)). *Why:* honest,
  without a disclaimer on every page.

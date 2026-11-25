# Fix a match: design proposal

A proposal for redesigning the **Fix a match** setting (Titles & art), with the research behind it. It is not built
yet; its [decisions](#decisions) are settled, and the work is listed in the roadmap's [Next up](../roadmap.md#next-up).
The window's look is described in [settings-window.md](settings-window.md): the dialog and the list follow it (neutral
greys, the blurple accent only on the selected row and the main button, no pills, capitals, stripes or shadows, short
help texts).

Mockups, in this folder (open them in a browser; Light, Dark and System buttons at the top; they load nothing from the
web and only draw made-up results): [fix-a-match-dialog.html](fix-a-match-dialog.html) and
[fix-a-match-list.html](fix-a-match-list.html).

## Questions answered

**How does it work today?** A fix is keyed on the *read title*: the title the program reads from the file name
(lowercased and cleaned, `normTitle`), and kept in two maps, `artworkAliases` (search under another name) and
`artworkOverrides` (a pinned cover). One line covers every file read as that title. The window gives no hint of this:
two monospace text boxes, no search, no way to see what the file was read as, and no feedback. A digits-only name, such
as a Blu-ray's `00062`, is marked junk and not looked up, unless a fix names it. Discs are mostly named from the disc
itself, so most need no fix.

**Does the user fix per file name?** By default no: a fix applies to "every file read as *this title*", the right unit
(Plex, Infuse and Jellyfin fix a show once and the episodes follow). The dialog also offers "only this file".

**Does the user contribute to the project's rules?** Never automatically. Decided: an opt-in "Tell the project about
this" that shows a short report, copies it and opens GitHub's issue form; the user posts it. The program sends nothing.

**Auto-complete?** Yes. The search box asks the catalogs in use (only those on under Privacy) after a short pause in
typing and shows cover, year, movie or show, and catalog. Picking one pins that exact entry, so a fix cannot "do nothing".

## Research findings

How other software does it:
- **Plex** (item menu > Fix Match) and **Jellyfin / Emby** (Identify): a name, year or provider ID, then a list of
  results with posters; a Search button, not as you type. The fix is on the movie or the whole show, never a season or
  an episode. Undo is Unmatch or Identify again. Nothing is shared.
- **Kodi** (Refresh, an NFO file) and **Sonarr** (manual import) fix per file or folder.
- **Infuse** (Edit Metadata): suggested matches first, then a search box; one episode fixes the whole show; fixes
  sync across the user's own devices.
- **MusicBrainz Picard** and **Universal Trakt Scrobbler**: sharing exists only with a community database or an
  opt-in server (the Trakt tool shares corrections with its developer only if the user agrees).
- **Discord presence apps** (mpv-discord, mprisence and others): config templates only, no fixing.

What carries over: start from the wrong item; prefill the box with what was read and show suggested matches first;
show cover and year, because names alone are ambiguous; key a fix on the show or title (here, with "only this file" as
the exception); an ID or catalog link is the dependable form; Undo is one step. None of these tools searches as you
type. This program has no community database, and a server is a non-goal, so sharing is the opt-in report only.

Sources: [Plex Fix Match](https://support.plex.tv/articles/201018497-fix-match-match/),
[Jellyfin identifiers](https://jellyfin.org/docs/general/server/metadata/identifiers),
[Infuse: fixing mismatched titles](https://support.firecore.com/hc/articles/4405035033495),
[Universal Trakt Scrobbler listing](https://chromewebstore.google.com/detail/universal-trakt-scrobbler/mbhadeogepkjdjeikcckdkjdjhhkhlid),
[mprisence](https://github.com/lazykern/mprisence).

## Proposed design

**What a fix is attached to.** By default, every file read as one title, which is how it works today: the dialog's
scope choice says "Every file read as **Sample Show**". The second choice, "Only this file", keys the fix on the file's
exact name (base name, never the folder) and wins over a title fix for that file. It stops matching if the file is
renamed. File names repeat across discs (`00062.m2ts`), so the title choice stays the default.

**Where it starts.**
1. The preview's existing link **Wrong title or cover?** opens the **Fix this match** dialog (today it only scrolls
   to Titles & art). It shows whenever a video is playing, matched or not; songs are not looked up, so it stays
   hidden for them.
2. **Titles & art > Fix a match** becomes a list of fixes with **Add a fix** (for a title that is not playing).
3. "Why this title?" in Copy diagnostics points to that link. No tray item.

**Step 1, Fix this match** (mockup 1):
- First line: "From **sample.show.s01e02.1080p.mkv**, read as **Sample Show**, season 1, episode 2. Now matched to
  The Sample Show (1998)." The file name only, never its folder.
- A search box prefilled with the read title. Under it: "Searches IMDb, Cinemeta, TVmaze and AniList (the catalogs on
  under Privacy)." With all of them off: "All catalogs are off (Privacy)." and only the last row below is offered.
- The list opens with the closest results the lookup already saw (`a.closest`). As you type, after a 350 ms pause and
  at least 2 characters, the program asks the catalogs; each row shows the cover, the name, "2019 · Show · IMDb", and
  "Now" on the current match.
- The last row keeps today's behaviour: "Search as "..." instead (no entry pinned)".
- The footer holds the scope choice: "Every file read as Sample Show" (selected) or "Only this file (sample.show...mkv)".
- **Use this** (or Enter, or a double click) saves the fix, as any setting saves.

**Step 2, Fixed:** the picked cover and name; "The card shows it in a moment. Episode titles come from this entry."
- **Cover:** "This entry's cover", then a row of covers of the same title from the *other* catalogs that are on under
  Privacy, then "An image address" (today's pinned cover). The other covers are asked for when this step opens, not
  before: one search per enabled catalog through the usual network path (`a.net`), so each host's request gap, the
  blocked hosts and the Privacy switches apply. At most five covers are shown, cached for a few minutes. A
  chosen cover is saved as its image address.
- **Offset** (folded under "Episodes don't line up?"): two fields, season and episode, both 0 by default and added to
  the numbers read from the file before the catalog is asked. Example: a second cour numbered from 1 in the files but
  from 13 in the catalog needs +12: file episode 2 becomes catalog episode 14.
- Links: **Undo** and **Tell the project about this**; button **Done**.

**Offsets.** The parser (`media`) is untouched, so the frozen golden data stays valid. The offset is applied to the
read season and episode where the catalog is asked (cover lookup by season, episode-title lookup), after
`folderEpisodeNumbers` has chosen the file's number: the folder's count comes first and the offset moves it. The card
then shows the catalog's numbers, so the title and the number agree; Copy diagnostics prints both. A result below 1 is
ignored, with a WARN line. 0 and 0 is today's path.

**The list (mockup 2):** each row has the cover, the read title (or, for a file fix, its file name), and a line saying
what it does: "Sample Show (2019) · Show · IMDb", "Searched as "Sample Movie Remastered"" (an old alias line), "Your
image", "Only this file" and "episodes +12" where they apply. **Edit** reopens the dialog; **Remove** removes the fix
at once and the bar offers Undo. An empty list says where fixes start.

**Tell the project about this** (part of the first version, following the maintainer's choice; opt-in). A dialog with
the editable report text: the file name only (never folders), what it was read as, the automatic match, the fix with
its scope and offsets, the catalogs on, the version. "The program sends nothing: you copy the text, look it over, and
post it on GitHub yourself." **Copy and open GitHub** copies it and opens the issue form at its plain address; the file
name never goes into the web address.

## Mockups
- [fix-a-match-dialog.html](fix-a-match-dialog.html): the status block with a wrong card; Fix this match with made-up
  suggestions, "Now" on the current match and the scope choice; then Fixed with the cover choice (made-up covers from
  other catalogs), the offset fields, Undo and Tell the project.
- [fix-a-match-list.html](fix-a-match-list.html): the list, Edit and Remove, an empty-list toggle, and the Tell the project dialog.
- Not shown yet: the all-catalogs-off state, the card's episode line with an offset, the Undo bar.

## Implementation plan

**Data model.** One new setting, `matchFixes`, a list that replaces both maps. Each fix has: `match` (the read title
as `normTitle`, or, when `file` is true, the exact file name); `file` (true: this file only); `entry` (a pinned catalog
page address, or empty) or `search` (another name to search, or empty); `cover` (an image address, or empty); and
`seasonOffset` and `episodeOffset` (integers, 0). A file fix wins over a title fix. A catalog page address (an IMDb,
TMDB, TVmaze, AniList or Kitsu page) pins the entry.

**Migration: once, on load, and the old keys stay in the file.** When `matchFixes` is absent and `artworkAliases` or
`artworkOverrides` has lines, each title becomes one fix (the alias value as `search`, the override value as `cover`; a
title in both becomes one fix) and the setting is saved. The old keys are left as they are and ignored from then on;
they stay in `core.Config` as documented legacy fields. Why once, and not "keep reading both": two sources would make
Remove ambiguous (an old line would come back) and keep the old code path alive for good. Why leave the old keys:
nothing is lost on a downgrade, and an existing user's lines act exactly as before, so nothing changes by default. A
fix saved by a new version is not seen by an older one; the changelog says so.

**One first version with all of it** (size L: two or three releases' worth of work, built on one branch):
1. `core`/`store`: `matchFixes` with its schema, the migration, the lookup (file before title), the legacy keys.
2. `artwork`: `Suggest(query)` (each enabled provider's `Search` through `a.net`; at most three services per query;
   not Wikipedia, Jikan or Bulbapedia; deduplicated; cached); page addresses resolved by ID so episode titles work from
   the pinned entry; `Covers(entry)`; fixes still read before the `media.Junk` return; the offset applied where season
   and episode reach the catalogs.
3. `cmd/mprp` API: POST `match-search` and `match-covers`, each answer carrying a request number so the window
   drops answers to older keystrokes; the status carries the read title, the file's base name, season, episode and the
   closest results (no folders); the Tell the project text is built from the same data.
4. `internal/assets`: the dialog and the list replace the two text boxes; `why.go` says "Matched as you chose: ...".
5. Docs: `config.md` gets a `matchFixes` row (default `[]`, each field named) and the two old rows say "Read once with
   `matchFixes`; kept for a downgrade", so `TestEverySettingIsDocumented` passes; how-it-works, troubleshooting, the
   changelog.

**Tests** (fakes only, no real catalogs): the migration (both maps, a title in both, idempotent, old keys untouched);
file fix beats title fix; a fix read before the junk return; suggest and cover lookups, including request gaps,
blocked hosts and Privacy switches; the page-address resolver; offsets with and without `folderEpisodeNumbers`, below
1, and 0 and 0 equal to today's output on the golden season cases; the endpoints and their request numbers; a report
text with no path; `TestPageScriptParses`.

**Risks:**
- A file fix breaks when the file is renamed. The list shows the file name, so it stays visible and editable.
- More requests: the covers step asks each enabled catalog once, Suggest up to three per pause. Mitigations: debounce,
  two characters minimum, the cache, the existing gaps; covers are asked only when Step 2 opens.
- Offsets meet folder numbers and the frozen golden data. The parser is not touched, offsets apply after it, and 0 and
  0 is the old path. `pickSeasonEntry`'s rules (the earliest entry wins) stay: an offset moves the number, not the
  entry choice.
- Saving makes the cover and title finder again (`ApplySettings`; `matchFixes` reaches `lookups`) and sends the status
  again, without a restart; check that the card does not blink.
- A pinned page can disappear from its catalog. Fall back to searching the read title, with a WARN line.

## Decisions
Settled by the maintainer on 2026-11-25. Three of them go beyond the first version this design first proposed, so
the first version now includes all four, and Tell the project too (it follows the choices; it is opt-in, and the
program never sends anything itself).
1. **Scope.** "Also offer only this file." A fix applies to every file read as the title, or only to this file.
2. **Sharing.** "Opt-in ability to Tell the project." The user prepares, copies and posts the report.
3. **Covers.** "Other catalogs' covers from the start." The cover step offers them in the first version.
4. **Offsets.** "In the first version." A season and episode offset per fix.

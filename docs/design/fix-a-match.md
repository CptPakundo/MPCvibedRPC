# Fix a match: design proposal

A proposal for redesigning the **Fix a match** setting (Titles & art), with the research behind it. It is not built
yet: it is the first entry of the roadmap's [Next up](../roadmap.md#next-up), after its [open decisions](#open-decisions).
The window's look is described in [settings-window.md](settings-window.md): the dialog and the list follow it (neutral
greys, the blurple accent only on the selected row and the main button, no pills, capitals, stripes or shadows, short
help texts).

Mockups, in this folder (open them in a browser; Light, Dark and System buttons at the top; they load nothing from the
web and only draw made-up results): [fix-a-match-dialog.html](fix-a-match-dialog.html) and
[fix-a-match-list.html](fix-a-match-list.html).

## Current state
- A fix is keyed on the *read title* and kept in two maps, `artworkAliases` (search under another name) and
  `artworkOverrides` (a pinned cover); the window shows two text boxes.
- A digits-only name, such as a Blu-ray's `00062`, is marked junk and not looked up, unless Fix a match names it: then
  the fix is read first and the lookup runs.
- Blu-ray and DVD titles are found from the disc itself (its volume label), so most discs need no fix.

## Questions answered

**How does it work today?** A fix is keyed on the *read title*: the title the program reads from the file name
(lowercased and cleaned, `normTitle`). "Search under another name" swaps the search text for that title;
"Pin a cover image" swaps the cover. So one line already covers every file read as that title (all episodes of a
show), not one file. The window gives no hint of this: two monospace text boxes, no search, no way to see what the
file was read as, and no feedback. A digits-only title (a Blu-ray's `00062`) is looked up when Fix a match names it.

**Does the user fix per file name?** No, and the design should keep it that way: a fix applies to "every file read as
*this title*". That is the right unit (Plex, Infuse and Jellyfin all fix a show once and the episodes follow). The new
dialog says so in one line and shows the read title, so the user knows what they are fixing.

**Does the user contribute to the project's rules?** Not today, and never automatically. Proposed: an opt-in "Tell
the project about this" in the dialog that shows a short report (file name, what it was read as, the automatic match,
the fix), copies it and opens GitHub's issue form. The user reads it and posts it themselves. The program still sends
nothing on its own.

**Auto-complete?** Yes. The search box asks the catalogs already in use (only those switched on under Privacy)
after a short pause in typing, and shows results with cover, year, movie or show, and catalog. Picking one pins that
exact entry, so the fix can no longer "do nothing".

## Research findings

| Software | Entry point | What the user types or picks | As you type? | Attached to | Undo | Shared? |
|---|---|---|---|---|---|---|
| Plex | Item menu > Fix Match (Match for unmatched) | Title, year, language, agent; an ID (`imdb-tt...`) works as the title; list of results with posters | No, a Search button | Movie, or the whole show (not seasons or episodes; Split Apart first) | Unmatch | No |
| Jellyfin | Item menu > Identify | Name, year, or a provider ID (TMDB, TVDB, IMDb); also `[tmdbid-...]` in the folder name | No | Item (movie or show) | Identify again; refresh metadata | No |
| Emby | Item menu > Identify | Name, year, provider IDs; `[imdbid=...]` in file or folder names | No | Item | Identify again | No |
| Kodi | Information > Manage > Refresh | Search box prefilled with the file name, "Manual" for a new term or ID; or a parsing NFO file with the catalog URL next to the file | No | Movie file or show folder (`tvshow.nfo`) | Refresh again, delete the NFO | No (the NFO is a file the user owns) |
| Sonarr | Manual / Interactive Import | Pick series, season and episode per file from lists | Lists, not search | Each file | Re-import | No |
| Infuse | Details > ⋯ > Edit Metadata | Suggested matches first, then a search box | Search box | One episode fixes the whole show | Not documented | Synced across the user's own devices (iCloud) |
| MusicBrainz Picard | Lookup in Browser / Manual Lookup | Searches on the MusicBrainz site, sends the release back; or paste a release link | Web search | The album cluster | Remove, look up again | Users edit the shared MusicBrainz database itself |
| Universal Trakt Scrobbler | Its history page | Correct a wrong match | n/a | The streaming service's item | n/a | Opt-in: corrections can be shared with the developer's server "so that they can be of use to other users" |
| Discord presence apps (mpv-discord, mprisence, discord-rich-presence-plex, jellyfin-rpc) | none found | Config templates only | n/a | n/a | n/a | n/a |

What carries over: (1) start from the wrong item, never from a settings list; (2) the box comes prefilled with what
was read, and suggested matches come first; (3) results show cover and year, because names alone are ambiguous;
(4) a fix is keyed on the show or title, not each file; (5) an ID or catalog link is the dependable form, and every
tool lets you pin one; (6) Undo is one step. None of these tools searches as you type; Infuse comes closest. Shared
fixes appear only where there is a community database (MusicBrainz) or an opt-in server (Trakt Scrobbler); this
program has neither, and a server is a non-goal.

Sources: [Plex Fix Match](https://support.plex.tv/articles/201018497-fix-match-match/),
[Plex: matching by ID](https://forums.plex.tv/t/help-fixing-matches-with-imdb-tvdb-tmdb-numbers/769090),
[Plex: show level only](https://forums.plex.tv/t/cant-fix-match-on-one-season-of-a-show/834918),
[Jellyfin identifiers](https://jellyfin.org/docs/general/server/metadata/identifiers),
[Emby identification](https://emby.media/learn/articles/metadata-discovery/identification/Metadata-Identification.html),
[Emby provider IDs](https://emby.media/learn/articles/metadata-discovery/identification/external-identifiers/Metadata-Provider-IDs.html),
[Kodi parsing NFO](https://kodi.wiki/view/NFO_files/Parsing),
[Kodi Refresh with manual search](https://forum.kodi.tv/showthread.php?tid=350679),
[Sonarr manual import](https://wiki.servarr.com/sonarr/wanted),
[Infuse: fixing mismatched titles](https://support.firecore.com/hc/articles/4405035033495),
[Infuse: fixes stored and synced per device](https://community.firecore.com/t/a-few-metadata-issues-with-corrected-movies/17794),
[Picard Lookup in Browser](https://picard-docs.musicbrainz.org/en/usage/retrieve_browser.html),
[Picard Manual Lookup](https://picard-docs.musicbrainz.org/en/usage/retrieve_manual.html),
[Universal Trakt Scrobbler listing](https://chromewebstore.google.com/detail/universal-trakt-scrobbler/mbhadeogepkjdjeikcckdkjdjhhkhlid),
[mprisence](https://github.com/lazykern/mprisence), [mpv-discord](https://github.com/TheBluuDot/mpv-discord),
[discord-rich-presence-plex](https://github.com/phin05/discord-rich-presence-plex).
Several Plex, Kodi and Infuse details come from user forum posts, not official docs.

## Proposed design

**What a fix is attached to.** Every file read as one title, which is how it works today. The dialog's footer says
"For every file read as **Sample Show**." Not per file: file names repeat across discs and folders (`00062.m2ts`), and
the disc's own name is read from the disc. Seasons and episode offsets come later (below).

**Where it starts.**
1. The preview's existing link **Wrong title or cover?** opens the **Fix this match** dialog (today it only scrolls
   to Titles & art). It shows whenever a video is playing, matched or not; songs are not looked up, so it stays
   hidden for them.
2. **Titles & art > Fix a match** becomes a list of fixes with **Add a fix** (for a title that is not playing).
3. "Why this title?" in Copy diagnostics gets one line: "To fix it: Wrong title or cover? under the preview." No tray
   item: the tray stays as short as it is.

**Step 1, Fix this match** (mockup 1):
- First line: "From **sample.show.s01e02.1080p.mkv**, read as **Sample Show**, season 1, episode 2. Now matched to
  The Sample Show (1998)." The file name only, never its folder (the same rule as Copy diagnostics).
- A search box prefilled with the read title. Under it: "Searches IMDb, Cinemeta, TVmaze and AniList (the catalogs on
  under Privacy)." It names only the services that are on. With all of them off: "All catalogs are off (Privacy)."
  and only the last row below is offered.
- The list opens with the closest results the lookup already saw (`a.closest`), so it often needs no typing. As
  you type, after a 350 ms pause and at least 2 characters, the program asks the catalogs; each row shows the cover,
  the name, "2019 · Show · IMDb", and "Now" on the current match. The accent is used only for the selected row
  (soft) and the Use this button.
- The last row keeps today's behaviour: "Search as "…" instead (no entry pinned)".
- **Use this** (or Enter, or a double click) saves the fix, as any setting saves.

**Step 2, Fixed:** the picked cover and name; "The card shows it in a moment. Episode titles come from this entry."
Cover: "This entry's cover" (with the other catalogs' covers of the same title to choose from) or "An image address"
(today's pinned cover). Links: **Undo** and **Tell the project about this**; button **Done**. Later: a folded
"Episodes don't line up?" for a season and episode offset.

**The list (mockup 2):** each row has the cover, the read title, and a line saying what it does: "Sample Show (2019) ·
Show · IMDb", "Searched as "Sample Movie Remastered"" (an old alias line), or "Your image" (an old pinned cover).
**Edit** reopens the dialog; **Remove** removes the fix at once and the bar offers Undo. When the list is empty: "No
fixes yet. When the card shows the wrong title or cover, choose Wrong title or cover? under it."

**Tell the project about this:** a dialog with the editable report text, "The program sends nothing: you copy the
text, look it over, and post it on GitHub yourself", and **Copy and open GitHub**. This is the Copy diagnostics
pattern. The file name never goes into the web address.

## Mockups
- [fix-a-match-dialog.html](fix-a-match-dialog.html): the status block with a wrong card. Fix this match opens with suggestions, the search
  filters made-up results as you type, and the current match says "Now". After Use this, the Fixed step
  with cover choice, the folded offset (a later step), Undo and Tell the project. The preview updates.
- [fix-a-match-list.html](fix-a-match-list.html): Titles & art with the list (a pinned entry, an old alias, a pinned cover, both), Edit and
  Remove, an empty-list toggle, and the Tell the project dialog.

## Implementation plan

**Data model, with no migration.** Keep both keys. `artworkAliases` values may now also be a catalog page address
(`https://www.imdb.com/title/tt0000001/`, a TMDB, TVmaze, AniList or Kitsu page): that pins the entry. Any other value
is searched as before. `artworkOverrides` stays the pinned cover. The window reads both maps and joins them by
`normTitle`, so lines users already have show up as rows. An older version given a page address only searches for
it as text and finds nothing, which does no harm.

**Minimal version (M; roughly one release's worth of work):**
1. `artwork`: `Suggest(query)`: each enabled provider's existing `Search` (movie and series) through `a.net`,
   so request gaps, blocked hosts and Privacy switches apply. At most three services per query: IMDb suggestions
   (keyless, built for type-ahead, returns covers), Cinemeta or TMDB with a key, TVmaze for shows, and AniList
   instead of TVmaze when the title is read as anime. Not asked: Wikipedia (exact titles only), Jikan (400 ms gap),
   Bulbapedia. Results are deduplicated by IMDb ID or by name and year, and cached per query for a few minutes.
2. `artwork`: resolve a page address by ID (Cinemeta `/meta/{type}/{imdb}.json` is already used; TMDB `/movie|tv/{id}`,
   TVmaze `/shows/{id}`, AniList and Kitsu by ID), fill `IDs`, so episode titles work from the pinned entry.
3. Fixes are read before the `media.Junk` return (done; keep it so when the lookup changes).
4. `server`/`engine`: POST `match-search` {q} → [{name, year, kind, poster, catalog, page}]; a request number lets
   the window drop answers to older keystrokes. The status the window gets carries the read title, the file's base
   name, the season and episode, and the closest results (it is local only; folders stay out).
5. `ui.*`: the dialog and the list replace the two textareas (the schema keeps both keys, with a custom renderer).
6. `why.go`: "Matched as you chose: …" for a pinned entry.
7. Tests: suggest and resolve against fakes (no real catalogs), the junk-title case, joining the two maps, the server
   endpoint, `TestPageScriptParses`, `TestEverySettingIsDocumented` (the keys are unchanged; update the docs rows).
   Docs: config.md (an alias value can be a page address), how-it-works, troubleshooting, a decisions.md line,
   changelog.

**Later:** other catalogs' covers in Step 2; Tell the project; a season or episode offset per fix, which needs a
new field (one `matchFixes` list replacing both maps, migrated when loaded); fixes in Copy diagnostics, which it
already lists.

**Risks:**
- Type-ahead load on unofficial endpoints (IMDb suggestions). Mitigations: debounce, two characters minimum,
  three services, the cache, the existing gaps.
- A parser change in a later version can change the read title and orphan a fix. Golden tests cover most names; the
  list shows the read title, so an orphan stays visible and can be edited.
- Saving restarts the engine (`ApplySettings` stops and starts it). That already refreshes the card. Check that
  the restart does not blink the Discord card; if it does, swap the finder and resend without a restart.
- A pinned page can disappear from its catalog. Fall back to searching the read title, and add a WARN line to the log.

## Open decisions
The maintainer's to settle before the first version is built:
1. **Scope:** only "every file read as this title" (recommended), or also an "only this file" choice?
2. **Sharing:** "Tell the project about this" as copy-and-post (recommended, in the later step), or nothing at all?
3. **Covers in the first version:** the entry's cover plus "An image address" only (recommended), or the other
   catalogs' covers as well from the start?
4. **Season and episode offset:** build it later (recommended, after the first version is in use), or leave it out?

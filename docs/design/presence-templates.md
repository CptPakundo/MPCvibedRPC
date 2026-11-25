# Text lines: design proposal

A proposal for "Presence templates: a user-written layout of the status lines", with the research behind it. The
first version is built (version 0.9.60), with the differences listed under [As built](#as-built); its [decisions](#decisions) are settled. The window's look follows
[settings-window.md](settings-window.md). In the window and the docs the feature is called **Text lines**, because
"template" and "presence" are not words the window uses.

Mockup, in this folder (open it in a browser; Light, Dark and System buttons at the top; it loads nothing from the web
and draws only made-up titles): [presence-templates-look.html](presence-templates-look.html).

## Questions answered

**Which parts of the status can a user shape?** Three: the first text line (`details`), the second line while the
video plays (`state`), and, for films only, the cover's hover text (`large_text`). Everything else stays as it is:
- **The name and type** ("Watching Sample Show", "Listening to", "Playing"). They come from Look > Style, "Show the
  title as the activity name" and the stream form, and Discord names streams after them.
- **An episode's cover text.** It is "Season 1, Episode 2", which Discord turns into the S1E2 badge; a free text
  would break the badge, so episodes keep it fixed. A film's cover text ("Sample Movie (2020)") has no badge and can
  be shaped by Custom. The player logo's hover text stays the player's name.
- **The progress bar and the pause line.** While paused, the second line is the pause bar (`⏸ ▰▰▱▱ 12:00 / 45:00`) as
  today, and the timestamps are not touched.
- **Privacy always wins.** Hide what I'm watching sends "Watching a video" and the don't-show list sends nothing; text
  lines never apply there, and no placeholder can reach a folder, a path or the file name.
- **The stream form, music and the basic fallback.** The stream form keeps its own layout (the text bar needs the
  second line); a song stays "Listening to <file name>"; the basic fallback joins name and first line as today, so it
  follows a custom first line by itself.
- **Discord's limits and the program's trimming.** Each line is cut to 128 characters (125 and "...") and padded to two,
  as `clip` does now; an empty line is left out.

**Which placeholders exist?** Every value below is already computed in `internal/core/activity.go`; the list is what
the code has, nothing new is looked up. Values carry today's formatting, so a line built from them reads like today's.

| Placeholder | Value (example) | From |
|---|---|---|
| `{title}` | Sample Show | `chooseTitle` (catalog spelling, anime title choice) |
| `{year}` | 2020 | the file's year, else a film's catalog year (`movieDetails`) |
| `{genres}` | Drama, Comedy | at most two (`infoParts`); empty with Genres, rating and director off |
| `{rating}` | ★ 7.1 | `infoParts`; same switch |
| `{director}` | Dir. Sample Name | films only (`infoParts`); same switch |
| `{player}` | VLC media player | the application name (`appNameFor`), as on the logo |
| `{length}` | 45:00 | the video's length (`fmtTime`); empty when unknown |
| `{code}` | S01E02 | the episode label, or the file's own words ("OVA 3") (`episodeDetails`) |
| `{season}`, `{episode}` | 1, 2 | `effectiveSeason`, the episode number (the badge's number) |
| `{episodeTitle}` | Sample Episode | file, catalog or continued title (`episodeDetails`) |
| `{seasonName}` | Sample Arc | a named season not already in the title (`episodeDetails`) |

Not offered: the position (Discord's own bar shows it, and a running text would need an update every second, past
Discord's limit of five per 20 seconds), the file name and folder (privacy), and the catalog's name (the button has it).

## Research findings

| Software | What can be shaped | Syntax | Missing values | Presets |
|---|---|---|---|---|
| mprisence (Linux, MPRIS) | details, state, large and small text | Handlebars `{{{title}}}` | `{{#if album}}…{{/if}}`, `{{else}}` | One default per field |
| discord-rich-presence-plex | "all fields … for each media type" | template strings, documented in its web interface | not documented | per media type |
| foo_discord_rich (foobar2000) | top, middle and bottom text | foobar2000 title formatting | `[...]` drops a section when a field in it is empty | defaults |
| foobar2000 itself | any display text | `%artist%`, `$if(...)` | `[...]` as above; a missing title falls back to the file name | none |
| mpv | OSD and script texts | `${media-title}` | `${name:fallback}`, `${?name:text}`, `${!name:text}` | none |
| PreMiD presences | per presence, text settings with `%song%` style variables | `%name%`, percent signs on both sides | each presence's own code | switches and choices first, text fields second |
| Music Discord Presence (MusicBee) | the two lines | an ini file | not documented | ini defaults |

What carries over: (1) every tool that offers free text shapes the same fields, `details`, `state` and sometimes the
hover texts, and keeps the name and type fixed; (2) single-word placeholders in one kind of bracket are enough;
(3) the problem every syntax solves is the missing value, either with a conditional (Handlebars, mpv) or with a section
that drops out (foobar2000's `[...]`), which is far simpler to write; (4) the gentler tools (PreMiD) lead with choices
and keep free text second; (5) per media type matters: Plex's tool shapes episodes and films apart.

Sources: [mprisence](https://github.com/lazykern/mprisence) and its
[example config](https://github.com/lazykern/mprisence/blob/main/config/config.example.toml),
[discord-rich-presence-plex](https://github.com/phin05/discord-rich-presence-plex),
[foo_discord_rich](https://www.foobar2000.org/components/view/foo_discord_rich),
[foobar2000 title formatting](https://wiki.hydrogenaud.io/index.php?title=Foobar2000:Title_Formatting_Reference),
[mpv property expansion](https://mpv.io/manual/stable/#property-expansion),
[PreMiD settings](https://docs.premid.app/v1/guide/settings.html) and
[guidelines](https://docs.premid.app/dev/presence/guidelines),
[Music Discord Presence](https://github.com/jojo2357/Music-Discord-Presence). Several details come from READMEs and
forum posts rather than full references (checked 2026-11-25).

## Proposed design

**One row on the Look tab**, "Text lines", a choice: **Standard** (today's lines, the default), **Short**, **Detailed**
and **Custom**. A choice comes first because most people want a different look, not a language.

| Choice | Episode, first / second line | Film, first / second line |
|---|---|---|
| Standard | `Sample Episode` / `Sample Arc · Drama, Comedy · ★ 8.3` | `2020 · Drama, Comedy` / `★ 7.1 · Dir. Sample Name` |
| Short | `S01E02 · Sample Episode` / nothing | `2020` / `Dir. Sample Name` |
| Detailed | `S01E02 · Sample Episode` / `Drama, Comedy · ★ 8.3 · on VLC media player` | `2020 · Drama, Comedy · 1:52:10` / `★ 7.1 · Dir. Sample Name · on VLC media player` |

**Custom** opens five fields under the row, in two groups: Episodes (a first and a second line) and Films (a first line,
a second line and the cover text).
They start filled with the lines of the choice that was on before. The cover text of every choice is today's, `{title}[ ({year})]`; an empty cover text falls back to it. Under the fields:
- **Placeholder chips**, one per placeholder that fits the group (films get no `{code}`; the cover text takes the film's placeholders); a click inserts it at the
  cursor with a ` · ` before it when the field is not empty.
- **A sample** of both lines for "Sample Show S01E02" and "Sample Movie (2020)", drawn like the preview, updated as
  you type. While a video plays, the status block's preview at the top shows the real card once the change is saved.
- **One help line:** "Parts in [ ] drop out when a value is missing. Discord shows up to 128 characters a line."
  An unknown name such as `{titel}` stays in the text as typed, and the field shows "Unknown: {titel}".

**How a line is filled**, in this order:
1. `{name}` becomes its value; an unknown name stays as typed.
2. A part in `[ ]` is removed whole when any placeholder in it is empty: `[on {player}]`, `[Season {season}]`.
3. The line is split at ` · `; empty parts are dropped, so `{seasonName} · {genres} · {rating}` with no season name reads
   `Drama · ★ 8.3`, never `· Drama · ★ 8.3`.
4. Spaces are trimmed, the line is cut to 128 and padded to two; an empty line is left out.

**Saving.** As everywhere, the change saves itself, but a text field saves when it loses focus or after a pause of a
second, not on each key: each save sends the status again (see Risks).

**What it touches.** Genres, rating and director still empties `{genres}`, `{rating}` and `{director}` when off. "Also
show S01E02 in the title line" only applies to Standard and hides itself for the others, because the other choices
say where the code goes. "Show the title as the activity name" off keeps today's lines (the title is then the first
line), and the Playing style follows the same choice as Watching.

## Implementation plan

**Data model.** Two settings, both new, with defaults that change nothing:
- `textLines`: `"standard"` (default), `"short"`, `"detailed"` or `"custom"`.
- `customLines`: a map with the keys `episodeDetails`, `episodeState`, `movieDetails`, `movieState` and `movieCover`
  (default empty; a missing line key is an empty line, a missing `movieCover` is today's text).

Both go into `core.Config` and `DefaultConfig`, a choice field and a custom renderer in `internal/store/schema.go`
(the same way the `map` type is rendered today), rows in `docs/config.md` (Look), so `TestEverySettingIsDocumented`
passes. An older version ignores both keys.

**Where it applies.** A new `internal/core/lines.go` holds `Expand(tpl string, vars map[string]string) string` and the
presets. In `BuildActivity`, only in the branch with the title in the name and not the stream form: `episodeDetails`
and `movieDetails` also return their parts (they already compute them), and when `textLines` is not `"standard"` the
two lines come from `Expand`, and a film's large image text too (episodes keep "Season N, Episode M"); the second goes to `stateAndTimes` in place of the season and info lines, so the pause
line still wins. **Standard is not a template**: it stays today's code path, so the recorded golden data in
`activity.json.gz` (whose configs carry no `textLines`) is reproduced byte for byte. Standard's fallbacks (the season
name moving up when there is no episode title, a special's own words) are not template rules, and need not become
some. `HiddenActivity` and the music path set `textLines` to `"standard"` in their copies of the settings, so privacy
and songs cannot be reached by a template.

**The window.** The choice row, the five fields, chips and the sample in `ui.html`/`ui.js`. The sample expands in the
page with the same four rules (a few lines of script); a Go test runs a shared table of templates, values and results
through `Expand`, and the same table is embedded in the page's test so the two cannot drift.

**Tests.** `Expand`: each rule, unknown names, brackets around empty values, separators at the ends, 128 cut, UTF-16
counting of `★` and emoji. `BuildActivity`: each preset for an episode, a special, a film, a film without catalog data; a film's cover text (custom, empty, no year) and an episode's unchanged;
the pause line, the stream form, Hide what I'm watching and a song unchanged with `"custom"`; the golden test unchanged.
`TestPageScriptParses`, `TestEverySettingIsDocumented`.

**Docs.** `docs/config.md` (two rows), `docs/how-it-works.md` (What shows on Discord: one bullet), the schema's help
texts, the README's Look line if it lists the switches, a `docs/decisions.md` line (what stays fixed, and why), the
roadmap (Next up), the changelog.

**Size.** Small to medium: about 170 lines of Go and 230 of tests, 130 of page script and markup, the docs.

**First version:** the three choices and Custom with its five fields, chips and the sample (a film's cover text
included). **Later:** a music line (`{title}` only, for "Listening to"); text lines for the stream form's first line;
with "Show the title as the activity name" off.

**Risks.**
- Each save sends the status again (no restart: `ApplySettings` redoes what a setting reaches), and Discord takes about
  five changes in 20 seconds. The save on blur or pause limits it.
- A custom second line can hide what the badge shows or repeat it: the help says so through the sample, not a warning.
- A later change to Standard does not move custom lines with it: custom lines are the user's.
- Users can write odd lines. That is the feature; the 128 cut and the empty-line rule keep Discord from refusing them.

## Decisions
Settled by the maintainer on 2026-11-25; the first version is part of version 0.9.60.
1. **Scope.** "Choices plus Custom": the three choices and Custom in the first version.
2. **Fixed parts.** "Also a film's cover text": Custom may shape a film's cover hover text. The name, the pause line,
   the stream form and an episode's cover text (it makes Discord's S1E2 badge) stay fixed.
3. **Missing values.** "[ ] sections plus dropped separators."
4. **Value format.** "With today's marks": `★ 7.1`, `Dir. Sample Name`.
5. **Presets.** "Standard, Short, Detailed."

## As built
Where the first version (0.9.60) differs from the plan above:
- **The Playing style keeps today's lines.** Without the stream form the Playing style has no title in its name (it is
  "Playing Media Player Classic" with the title as its first line), so it is the same case as "Show the title as the
  activity name" off: text lines would remove the title.
- **Custom starts from the choice before** only while its lines are empty; lines written earlier are kept.
- **The shared table** is `internal/core/testdata/lines.json`; the page's test (`TestPageBehaviour`) runs it through the
  page's own code.

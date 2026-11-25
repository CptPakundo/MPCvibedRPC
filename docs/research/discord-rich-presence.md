# Discord Rich Presence: what the card can show and what MPCvibedRPC sends

A research report, written 2026-11-21 and checked against the code on 2026-11-25. Its sources are Discord's public
developer documentation, what was seen on real profiles and streams, and this repository. Nothing here comes from
reading Discord's own application code. The decisions that follow from it are in
[decisions.md](../decisions.md#behaviour); the outside facts to watch are in the roadmap's
[watch list](../roadmap.md#watch-list).

Evidence tags: **[doc]** in Discord's documentation (links below), **[observed]** seen on a real profile, card or
stream, **[repo]** read in this repository, **unconfirmed** a belief or a report that has not been tried (the open
questions at the end say how to settle them).

Documentation used (all read 2026-11-25):
- D1 <https://docs.discord.com/developers/events/gateway-events> (the Activity object)
- D2 <https://docs.discord.com/developers/rich-presence/using-with-the-embedded-app-sdk>
- D3 <https://docs.discord.com/developers/topics/rpc> (SET_ACTIVITY)
- D4 <https://docs.discord.com/developers/change-log>

## What the program does today
Built: **both buttons** (a second page of the title, such as Letterboxd or IMDb); **`small_url`** (the player's logo
opens the player's site); **the player logos as external image addresses** (the `docs/players` files, served from the
repository's raw address, `core.PlayerIconBase`) **[repo]**; **the 300-character guard** (`core/activity.go`: a cover
address over 300 characters is left out, because the whole status would otherwise be at risk, see below); and **a text
progress bar on the stream form** (a Playing status has no bar of its own). Not done: the unconfirmed ideas at the end.

## What was observed
- **An image key the application does not hold is dropped, and the update still succeeds.** The default application
  holds only `default`, `pause_small`, `play_small`, `stop_small` and `mpcbe_logo` (its asset list is public; checked
  2026-11-21), so the key `mpc-hc` showed no image at all. That is why the program sends the player logos as external
  addresses instead of keys. **[observed]**
- **A screen share is named after the streamer's first *Playing* activity**, never a Watching one. **[observed]**
- **A profile shows one card per application, and Playing is listed before Watching.** **[observed]**
- **A Playing status has one timestamp that counts:** with only an end it shows time left, with only a start it shows
  elapsed time. **[observed]**
- **Only start and end together make a progress bar** (Watching and Listening); a start or an end alone shows no bar.
  **[observed]** The documentation agrees that both are needed for the time bar [doc D1].
- **Your own buttons are not shown to you**, only to other people. Reported widely; not retried here. Unconfirmed.

## Field table
Fields and limits as Discord's documentation gives them [doc D1, D3], and what the program sends today **[repo]**.
The documentation states no length limits for names, texts, timestamps, images or the party; the 128-character cut
the program applies to texts (padded to at least 2) is its own choice. How each field is drawn on a card is mostly
unconfirmed (see the open questions).

| Field | Documented | What the program sends now |
|---|---|---|
| `type` | SET_ACTIVITY allows Playing (0), Listening (2), Watching (3) and Competing (5) [doc D3] | 3, or 0 (Playing; left out of the payload) |
| `name` | string | the title only when "title as the name" or the stream form is on |
| `details`, `state` | strings | the episode or movie line and the season or info line; the file name in the basic form; progress text while paused or in the stream form |
| `details_url`, `state_url` | strings [doc D1] | the title's catalog page |
| `timestamps.start`, `.end` | Unix milliseconds; both give a time bar for Listening and Watching [doc D1] | Watching: both while playing, none while paused. Playing: the end if `showRemainingTime`, else the start |
| `assets.large_image`, `.small_image` | an asset key, or an external image address [doc D2] | the cover address (left out over 300 characters), else an asset key; the player's logo address |
| `assets.large_text`, `.small_text` | strings | "Season n, Episode m" or the title (year); the player's name |
| `assets.large_url`, `.small_url` | strings [doc D1] | the catalog page; the player's site (`core.PlayerSite`) |
| `assets.invite_cover_image` | string [doc D1] | no |
| `buttons` | at most 2; label 1-32 characters, URL 1-512 characters; a bot sees only the labels [doc D1] | up to 2 ("View on <site>" and a second page of the title; label cut to 32, URL up to 512, http or https) |
| `party` | `id`, and `size` as two integers [doc D1] | no |
| `secrets` | join, spectate and match secrets [doc D1] | no |
| `instance` | boolean [doc D1] | `false` always |
| `status_display_type` | 0 name, 1 state, 2 details [doc D1] | Watching: 0 (title in the name) or 2 (title in the details). Playing: not sent |
| `emoji`, `flags`, `url`, `application_id`, `created_at` | part of the Activity object [doc D1]; not set by a program through SET_ACTIVITY | no |

Update rate: "Clients may only update their game status 5 times per 20 seconds" [doc D1]. The engine collapses bursts of
changes and the latest state wins **[repo]**, so a seek or pause within a burst can show up to 20 seconds late
(unconfirmed on a real card).

External images: an address may be used instead of an asset key [doc D2]. The program keeps cover addresses under 300
characters and falls back, in order, to the status without links, without the button, and the basic form when Discord
refuses a status (`sendActivity`) **[repo]**. Whether one over-long address makes Discord refuse the whole status is
unconfirmed; the guard exists so that it can never matter.

## What the program decided because of this
1. **The small player icon.** The logos are external https addresses (`docs/players`, `core.PlayerIcon`), so no asset
   upload is needed. Their addresses never change ([the rule](../../CONTRIBUTING.md#ground-rules)).
2. **Both buttons.** The second button is another page of the title (`core.SecondLink`). A refusal falls back to the
   existing chain.
3. **`small_url`.** The logo is clickable; it is dropped with the other links by the `WithoutLinks` fallback.
4. **The 300-character guard** on cover addresses.

## Ideas not done (all unconfirmed until tried)
- Moving "Season n, Episode m" into `state` for Watching, if a real card shows the state line there (Q1).
- Sending `status_display_type` for a Playing status, which would only change the one-line text in member lists; it
  would change the look for existing users, so it would stay opt-in (Q4).
- Using `party` for "episode n of m": not worth trying until Q3 shows what it does to the card.
- Avoid `secrets` (they create join UI), `instance`, `emoji` and the Streaming type, which SET_ACTIVITY does not allow
  [doc D3].

## Open questions only a real Discord test settles
Test with an account in a private test server and a throwaway data folder
([Running the program safely](../development.md#running-the-program-safely)).

- **Q1** Does a Watching card show `state` as a line under `details`, and is `large_text` visible anywhere except as
  hover text?
- **Q2** Does an external https address as `small_image` (the player logos) render on the Playing card in the popout
  and in the full profile?
- **Q3** Playing with a `party`: does it change or hide the state line, and is "(n of m)" shown anywhere?
- **Q4** Does `status_display_type` work for a Playing status, and what does the member list show with 1 and 2?
- **Q5** How long can `details` and `state` be before they are cut off, in the popout and in the full profile?
- **Q6** Is a 2:3 poster letter-boxed or cropped in the card's image box?
- **Q7** Do `details_url`, `state_url`, `large_url` and `small_url` work on both kinds of status in the popout?
- **Q8** When Discord cannot fetch an external image, is the image dropped quietly and tried again on the next update?
- **Q9** How do Watching cards, buttons and links look on mobile?
- **Q10** Do two buttons render in the popout as well as in the full profile?
- **Q11** Does one cover address over 300 characters make Discord refuse the whole status?

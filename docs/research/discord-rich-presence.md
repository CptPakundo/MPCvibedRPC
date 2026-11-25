# Discord Rich Presence: what the card can show and what MPCvibedRPC sends

A research report, read 2026-11-21 and checked against the code on 2026-11-25. It is read-only research: sources are
Discord web client build **677695** (main bundle `web.f53eaa9877e51f2f.js`), Discord's developer docs and this
repository. No Discord app was contacted. The decisions that follow from it are in [decisions.md](../decisions.md#behaviour);
the outside facts to watch are in the roadmap's [watch list](../roadmap.md#watch-list).

Evidence tags: **[code]** read in the client bundle (module / function named), **[doc]** documented (URL), **[repo]**
read in this repository, **[established]** seen on a real card in an earlier real test, **[inferred]** a conclusion from
the above, to be settled by a real test (see the open questions at the end).

## What the program does today
From the ranked list below, these are built: **both buttons** (a second page of the title, such as Letterboxd or IMDb),
**`small_url`** (the player's logo opens the player's site), **the player logos as external image addresses** (the
`docs/players` files, served from the repository's raw address, `core.PlayerIconBase`; the default Discord application
holds no `mpc-hc` image), **the 300-character guard** (`core/activity.go`: a cover address over 300 characters is left
out, because Discord refuses the whole status for it), and **a text progress bar on the stream form** (a Playing status
has no bar of its own). Not done: items 5 to 11 of the ranked list, and the open questions need a real Discord test.

Module ids are those of the minified bundle and change with every build; the names below are descriptive.

Doc URLs used:
- D1 https://docs.discord.com/developers/events/gateway-events (Activity object)
- D2 https://docs.discord.com/developers/rich-presence/using-with-the-embedded-app-sdk
- D3 https://docs.discord.com/developers/topics/rpc (SET_ACTIVITY)
- D4 https://docs.discord.com/developers/change-log

## 0. Where the card differs from what one would expect

1. **State line on the Watching profile card.** The card component (`UserProfileActivityCard`, function `e0`, module 802880) has three text slots: `ee()` heading, `et()` second line, `en()` third line. `en()` starts with `if(r.type===WATCHING)return null`, so only the **third** slot is dropped for Watching. For Watching the slots are: heading = `details` (link `details_url`), second line = **`state`** (link `state_url`), third = nothing. For Playing: heading = `name`, second line = `details`, third = `state` (see 1.4 for the party catch). **[code]** So `state` probably does show on a Watching card, directly under `details`. An earlier reading that a Watching card shows no state line probably saw only the `en()` line. **[inferred]**: needs a real screenshot (open question Q1). The text that Watching loses for sure is `large_text` and the party "(n of m)" (third slot).
2. **A small player icon may not show.** The card does render a small image (`smallImage` passed to the card image component; the Active Now panel also draws it as a 64 px overlay) **[code]**. The only suppression is for application id 981509069309354054 (Crunchyroll's app; module "isCrunchyroll") **[code]**. The likely cause (it explained a missing icon in a real test) is the asset resolution step: before sending, the client resolves `large_image`/`small_image` through `resolveAssets` (module 913659, function `RG`). A key that is not among the *application's* uploaded assets is silently **deleted** (`null!=t?p.small_image=t:delete p.small_image`) and the update still succeeds **[code]**. So `small_image = "mpc-hc"` shows only if the application behind `clientId` really has an asset named `mpc-hc` (lowercase; keys are lower-cased **[doc D2]**). The same rule applies to `large_image` keys. The default application's assets are public (`https://discord.com/api/v10/oauth2/applications/<id>/assets`); on 2026-11-21 it held `default`, `pause_small`, `play_small`, `stop_small` and `mpcbe_logo`, so the program sends the player logos as external addresses instead of keys.
3. The progress bar and the name display match the code (progress bar needs Listening/Watching plus start and end: module 53257; status_display_type consumer: module 566903).

## (a) Field table

"Accepted" is the SET_ACTIVITY IPC schema (Joi, bundle map `[SET_ACTIVITY]: { request: ... }`, near the RPC command tables) **[code]**. The schema ends `.unknown(true)` on the activity and on the sub-objects, so unknown keys are not rejected; the handler (`yZ[SET_ACTIVITY]`) passes the activity on. Minimum lengths matter: Discord counts **characters** here (Joi string length), and our `clip()` already pads to 2 and cuts at 128 (UTF-16 units; fine).

| Field | Accepted over IPC? | Limit | Watching (type 3) renders | Playing (type 0) renders | We send now |
|---|---|---|---|---|---|
| `type` | yes; only 0, 2, 3, 5 valid, default 0 **[code]** [doc D3] | enum | Header "Watching <name>" | Header "Playing <name>" | 3 or 0 (0 omitted) |
| `name` | yes **[code]** | 1..128 | Header text; member-list text when `status_display_type` is 0/absent; the *application name* is used when absent (`name: u.name ?? application.name`) **[code]** | Same; also the name a **stream** is labelled with **[established]** | the title, only when title-as-name or stream form |
| `details` | yes | 2..128 | Profile card heading (bold, 1st line) **[code]**; Active Now heading (falls back to `name` if empty) **[code]** | Profile card 2nd line **[code]** | episode/movie line, or file name in basic form |
| `details_url` | yes (uri) | 1..256 | Heading becomes a link **[code]** | 2nd line becomes a link **[code]** | catalog page (`AddLinks`) |
| `state` | yes | 2..128 | Card 2nd line (see section 0) **[code]**, Active Now line under details **[code]** | Card 3rd line, **only if no `party` object** (1.4) **[code]**; Active Now line **[code]** | season/info line, "Playing" in Playing style; progress text while paused / in stream form |
| `state_url` | yes (uri) | 1..256 | Link on the state line **[code]** | Link on the state line **[code]** | catalog page |
| `timestamps.start` / `.end` | yes; numbers >= 1; seconds auto-converted to ms when more than 2 digits shorter than `Date.now()` **[code]** | min 1 | start+end: **progress bar** with elapsed / total on the card **[code]** (module 753713, condition module 53257); elapsed is shown rounded down to 10 s steps **[code]**; only start or only end: no bar (member list/Active Now timer only) **[established]** | One number: end wins ("left"), else start ("elapsed") **[established]** | Watching: start+end while playing, none while paused. Playing: end if `showRemainingTime`, else start |
| `assets.large_image` | yes | 1..300 chars | Card image (60/100 px) **[code]**; falls back to the app icon if empty/unresolvable **[code]** | same | cover URL (left out over 300 characters), else asset key |
| `assets.large_text` | yes | 2..128 | Hover text; the visible 3rd line is *not* rendered for Watching (`en()` returns null) **[code]** | Hover text; the 3rd-line `large_text` fallback is reached only for non-Playing types, so for Playing it is hover only **[code]** | "Season n, Episode m" or title (year) |
| `assets.large_url` | yes (uri) | 1..256 | Click on image/text area opens it **[code]** (the whole text block is a button when set) | same | catalog page |
| `assets.small_image` | yes | 1..300 | Corner overlay on the card image; hidden only for the Crunchyroll app id **[code]** | same | the player's logo address (`core.PlayerIcon`) |
| `assets.small_text` | yes | 2..128 | Hover text | Hover text | `appName` |
| `assets.small_url` | yes (uri) | 1..256 | Link on the small image (docs: `small_url`) [doc D1] | same | the player's site (`core.PlayerSite`) |
| `assets.invite_cover_image` | yes | 1..300 | Game-invite banner only [doc D1] | same | no |
| `party.id` | yes | 2..128 | none visible | none visible; used for join/party logic **[code]** | no |
| `party.size` `[n, m]` | yes; array of exactly 2 numbers >= 0 | length 2 | **Not rendered** (3rd slot null) **[code]** | "(n of m)" appended after state **only if the activity has the EMBEDDED flag**; otherwise a present `party` **hides the state line** on the card **[code]** (see 1.4) | no |
| `party.privacy` | yes; 0 private / 1 public | enum | sets PARTY_PRIVACY flags **[code]** | same | no |
| `secrets.match/join/spectate` | yes | 2..128 each; must be unique, must not equal party id, **cannot be sent with buttons** (error INVALID_ACTIVITY_SECRET) **[code]** | `join` sets the JOIN flag, which creates join / "Ask to join" UI for viewers **[code]** (button module for JOIN) | same | no |
| `buttons[]` | yes | min 1, max **2** items; `label` 1..32 (required), `url` 1..512 (required, uri) **[code]** [doc D1] | Full-width buttons under the card, labels only; URL fetched on click from the server-side metadata **[code]** | same | up to 2 buttons ("View on <site>" plus a second page of the title; label cut to 32, URL <= 512, http(s)) |
| `instance` | yes (boolean) | | sets INSTANCE flag; the viewer-side "notify" branch is dead code (`flag check` returns false for INSTANCE) **[code]** so no visible effect | same | `false` always sent |
| `supported_platforms` | yes; 1..10 items from the platform enum (`ActivityGamePlatforms`) **[code]** | | affects join/platform UI only [inferred] | same | no |
| `status_display_type` | yes; 0 name, 1 state, 2 details **[code]** | enum | One-line text in member list / DM list / friends rows **[code]** (module 566903). Default (absent) = `name` (for Streaming: details) | same function; Playing text then reads "Playing <chosen field>" with tooltip "Playing <chosen>" **[code]** | Watching: 0 (title in name) or 2 (title in details). Playing: not sent |
| `flags` | not accepted from the client as such; the client **computes** it (INSTANCE, JOIN, CONTEXTLESS, EMBEDDED, party privacy bits) **[code]** (module 182892) | | | | no |
| `platform` | set by the client to DESKTOP **[code]** | | | | no |
| `emoji` | not in the SET_ACTIVITY schema; allowed through as an unknown key but only meaningful for type 4 (custom status), which SET_ACTIVITY refuses **[code]** [doc D1/D3] | | none | none | no |
| `url` (stream URL) | only for type 1, not valid over SET_ACTIVITY **[code]** | | | | no |
| `application_id`, `created_at`, `session_id`, `sync_id`, `metadata` | set by the client/server; not ours to set | | | | no |

### 1.x Notes on the table

1. **Buttons.** Two are allowed (the program sends up to two). You never see your own buttons on your profile card: in the button renderer (`eV`) the first branch is `if(isSelf) return null` (or the "add to games" button for Playing in popouts), before the buttons are considered **[code]**. Other people see them. The URL is not in the gateway payload: the client sends `metadata.button_urls` and others fetch it on click **[code]**; bots cannot read button URLs [doc D1]. Buttons cannot be combined with `secrets` **[code]**.
2. **Images.** Two forms **[code]** [doc D1, D2]:
   - Uploaded asset key: must exist among the *application's* assets (up to 300, PNG/JPEG/WebP, no animation); resolved by name, unknown keys are dropped silently.
   - External `http(s)` URL: the client POSTs it to `/applications/<id>/external-assets`, gets an `mp:` path back, and the server serves it from the media proxy. Works for large and small image, accepts GIF, animated WebP, AVIF [doc D2]. The string must be **<= 300 characters** (schema), otherwise the whole SET_ACTIVITY is refused with a payload error **[code]**. The program leaves out a cover address over 300 characters **[repo]**, so one long poster address (some catalog CDNs) cannot push the title into the fallback chain.
   - Hover text: `large_text` / `small_text` (2..128). Image size on the card: 60 px (popout) / 100 px (full profile) **[code]**; aspect "default" is square-ish; no cropping rules were found in code (Q6).
   - The Active Now panel draws large image 160 px and small image 64 px **[code]**; it returns *nothing* for the rich section when the activity has no `assets` at all (`null==t.assets && ... return null`) **[code]**.
3. **Small image on the card**: see section 0, point 2.
4. **Party.** Profile card, `en()` (Playing only) **[code]**:
   `if party is null -> state; else if EMBEDDED flag and size has 2 numbers -> state + "(n of m)" (or "(n)" when m is 0); else -> nothing`.
   So with `party` set and no EMBEDDED flag (the case for ordinary RPC), the Playing card shows **no state line at all**, and no "(n of m)". The documentation says the opposite ("(1 of max_party)" badge when `party` is present) [doc D2], so either the client changed, or EMBEDDED is meant to be set another way. This is the largest "inferred, must test" item (Q3). `party.size` is also what makes a party "full" for join logic (`partySize >= maxPartySize`, module 702631) **[code]**; and `secrets.join` is what creates join buttons. Party without secrets: no join UI is created by this code path **[inferred]**. Episode "n of m" via party is therefore **not a reliable use** unless Q3 shows the badge renders.
5. **Rate limit.** [doc D1]: "5 times per 20 seconds". Code agrees: the presence sender keeps a list of send times, pushes `now + 20000` per send, allows 5, then queues the **latest** update with a timeout until the oldest slot frees up (throttle around `emitPresenceUpdate`, near the gateway `presenceUpdate` sender) **[code]**. So bursts are collapsed, not refused; later updates win. Seeks/pauses within a burst can be delayed by up to 20 s.
6. **Live updates without resending.** The progress bar (start+end) and the elapsed/left timer tick locally in each viewer's client from the timestamps **[code]**; nothing needs resending while play continues at 1x. A speed change, seek or pause needs a new activity (we already do that via `NeedsUpdate`).
7. **Wrapping.** In the popout/mini profile theme every text line is wrapped in a clamp component: **one line with an ellipsis** and a hover tooltip when truncated (`lineClamp` default 1) **[code]** (module 3026). In the full profile modal themes the text is rendered without that wrapper, so long lines are likely to wrap onto several lines **[inferred]** (Q5). Text is `trim()`med, so leading/trailing spaces are lost, and an empty string renders nothing **[code]**. The member list line is single-line text from module 566903 (ellipsis by CSS) **[inferred]**.

## Surface by surface

| Surface | What it shows | Source |
|---|---|---|
| Profile "Activity" card and user popout / mini profile | Same component `UserProfileActivityCard`; popout = one-line clamp, modal = full size. Header (type + name), slots as in the table, image + small image, progress bar (Watching/Listening with start+end) or timer, buttons (others only) | **[code]** |
| Member list line / DM list / friends rows | One line of text chosen by `status_display_type` (default `name`), tooltip "Watching X" / "Playing X". Nothing else from the activity | **[code]** module 566903; DM list use **[inferred]** (same helper) |
| Friends "Active Now" panel (`RichPresenceSection`) | Large image 160 px + small overlay 64 px with hover texts, heading = `details` (or `name` if empty), `state` line, a timer/progress component. No type check, so **state shows for Watching here**. Not shown at all when `assets` is absent | **[code]** |
| Mobile | Not examined in code. Docs say the Social SDK added Android RPC support (2026-07-28) [doc D4]; card rendering on mobile is assumed to follow the same fields, with the same single-line truncation | **[inferred]** |

## (b) Ranked improvements

Items 1 to 4 are done; 5 to 11 are not. Effort: S = a few lines + tests, M = a day, L = larger. "Fallback" = what the engine does today when Discord refuses (`sendActivity`: drop links, then drop the button, then basic mode).

1. **Fix the small player icon. Done.** The logos are sent as external https addresses (`docs/players`, `core.PlayerIcon`), so no asset upload is needed; the address is at most 300 characters and Discord's media proxy must reach it.
2. **Use both buttons. Done.** The second button is another page of the title (`core.SecondLink`: Letterboxd and IMDb, or another catalog); label <= 32, URL <= 512 (`buttonURL` checks). A refusal falls back to the existing chain. (You cannot see your own buttons, so a second one is for viewers only.)
3. **`small_url`. Done.** `Assets.SmallURL` is the player's site (`core.PlayerSite`), so the logo is clickable; it is dropped with the other links by the `WithoutLinks` fallback.
4. **The 300-character limit on image strings. Done for covers.** A `large_image` over 300 characters is left out before sending, so one long poster address does not push the title into the no-links or basic fallbacks (the refusal is whole-payload).
5. **Watching: put the useful line in the heading/second line, knowing the layout (S).** On the card heading = `details`, 2nd line = `state`, 3rd line gone, `large_text` hover only. So for Watching put the episode title in `details` and "Season/Episode · info" in `state` (already roughly the case). Consider moving "Season n, Episode m" from `large_text` (hover only for Watching) into `state` (Q1 must confirm `state` shows). Risk: verify first.
6. **Playing: do not send `party` (S, "negative" finding).** Sending `party` without the EMBEDDED flag appears to hide the state line on the Playing card (1.4). Keep `party` unused, or test first (Q3) before using it for "episode n of m".
7. **`status_display_type` in Playing mode (S).** Currently not sent. Setting it to 2 (details) or 1 (state) changes only the one-line member-list text (e.g. show the episode line rather than the title). Accepted in code for any type **[code]**. Risk: changes the look for existing users; keep opt-in. Stream form: no effect on the stream's name (named after the first Playing activity's `name`, established).
8. **Animated cover (M, both modes, optional).** External URLs may be GIF/animated WebP/AVIF [doc D2]; animated covers/trailers-like posters are possible, but catalog posters are static. Low value.
9. **Timestamp fidelity (S).** Seconds are auto-converted by the client if you send seconds, but we send ms; fine. The bar shows elapsed in 10 s steps **[code]**, so sub-10-second drift is invisible; the engine could resend less often (fewer 5/20 s budget hits) for tiny corrections. Risk: none if thresholds are kept above 10 s.
10. **`supported_platforms` (S, low value).** Not needed; affects join UI only.
11. **Avoid:** `secrets` (creates join/Ask-to-join UI, cannot coexist with buttons), `instance` (no visible effect), `emoji` (custom status only), `party` for episode counts (see 6), type 1/4 (refused).

## (c) Hard limits

- Types over SET_ACTIVITY: only 0, 2, 3, 5. No type 1 (Streaming) or 4 (Custom) [code, D3].
- One activity per application id on profile cards/member list (established); Playing is listed before Watching.
- Progress bar only for type 2/3 with start and end [code]; a Playing timer is one number [established].
- Third text slot is not rendered for Watching [code]: no party "(n of m)", no `large_text` line.
- Strings: name 1..128; details/state/hover texts 2..128; image strings <= 300; link URLs <= 256 (uri); button label <= 32, URL <= 512; max 2 buttons; party size exactly 2 numbers [code].
- Update rate: 5 per 20 s, latest wins [doc D1, code].
- Your own buttons are invisible to you [code]. Buttons cannot be sent with secrets [code].
- App name is the application's, not settable except via `name` (which we use) [code].
- Image formats: assets PNG/JPEG/WebP, max 300 assets; external URLs need a publicly reachable URL [doc D2]. No animation for uploaded assets.
- A stream is named after the streamer's first Playing activity (established); `status_display_type` does not rename it.
- Docs no longer list a length limit for name/details/state [doc D1]; the 128 limits come from the client schema [code] and are not guaranteed by Discord.

## (d) Open questions only a real Discord test settles

Test with an account in a private test server and a throwaway data folder ([Running the program safely](../development.md#running-the-program-safely)).

- **Q1** Does a Watching card show `state` as the second line under `details` (code says yes), and is `large_text` visible anywhere except as hover text?
- **Q2** Does an external https address as `small_image` (the player logos) render on the Playing card in the popout and in the profile modal? (The application's own asset keys are known: section 0.)
- **Q3** Playing + `party {size:[n,m]}` without EMBEDDED: does the state line really disappear, and is "(n of m)" shown anywhere? Same for Watching (code says never).
- **Q4** Does `status_display_type` work for a Playing activity, and what exactly does the member list show with 1 and 2?
- **Q5** Truncation and wrapping in the full-profile modal vs. the popout: how long can details/state be before cut off; does the modal wrap.
- **Q6** Image box shape: does a 2:3 poster get letter-boxed or cropped in the 60 px and 100 px boxes, and in Active Now (160 px)?
- **Q7** Do `details_url` / `state_url` / `large_url` / `small_url` work on both modes in the popout, and is the whole text block (not just the line) clickable (code: whole block is a button when `large_url`-like handler is set)?
- **Q8** Behaviour when an external image URL cannot be fetched by the proxy: is the image dropped quietly (code suggests yes), and is it retried on the next update?
- **Q9** Mobile rendering of Watching cards, buttons and links (no code read).
- **Q10** Whether two buttons render in the popout as well as in the modal (code renders all in both themes).

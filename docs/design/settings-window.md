# The settings window's design

The settings window is a local web page (`internal/assets/ui.html`, `ui.css`, `ui.js`) in an app window of the system
browser, or in a WebKit window on macOS. It looks like a quiet system settings page, not like a dashboard. The
decision and its reason are in [decisions.md](../decisions.md#the-window-and-the-tray); this page is the working
description for anyone who changes the page. The tokens below are the ones `ui.css` defines in `:root` (light) and in
`@media (prefers-color-scheme: dark)`; use them, and add none without a reason.

## Principles
- **Neutral greys, one accent.** Surfaces and strokes are plain greys. The accent (Discord's blurple) is rationed: the
  primary button, the selected tab's underline and the selected row, focus rings, and links.
  Nothing else is coloured except the three states (good, warn, bad) when they mean something.
- **No decoration.** No pills, no capital-letter labels, no coloured stripes on cards, no gradients, no shadows on
  cards (the one shadow, `--shadow`, is for dialogs over a scrim).
- **One status block at the top** with the preview of the Discord card (cover, name, details, progress bar, buttons)
  and the main control. Everything below is settings in tabs: General, Players, Look, Titles & art, Privacy, Advanced,
  Help. Each group of settings is one card; the version and Quit sit in the footer.
- **Every setting is a row:** label and help on the left, the control on the right. Settings save as they change; there
  is no Save button. Restore defaults sits under Advanced > Maintenance and asks first.
- **Short help.** A help text is one short sentence. A longer one shows its first sentence and the rest behind a
  **More** fold (`ui.js`, the help builder). Rarely used actions go under Advanced > Maintenance.
- **One term per thing.** The window says "Show on Discord" and "status", never "presence"; a setting has the same
  name in the window, `docs/config.md` and the README. A recorded store test pins the label "MPC-HC web interface
  port": change only its help text.
- **The main control is a button that says what it will do:** primary (accent) "Show on Discord" while nothing is
  shown, plain "Stop showing" while it is. A switch hides the action; a button names it.

## Tokens
Colours (light / dark):
| Token | Light | Dark | Use |
|---|---|---|---|
| `--bg`, `--surface`, `--surface-2` | #f3f3f3, #fbfbfb, #f0f0f0 | #202020, #2b2b2b, #262626 | page, cards, quiet blocks (notices, log) |
| `--control`, `-hover`, `-active` | #fff, #f6f6f6, #ececec | #353535, #3d3d3d, #313131 | buttons and inputs |
| `--stroke`, `--stroke-control`, `--stroke-strong` | #e5e5e5, #d1d1d1, #868686 | #3a3a3a, #484848, #9a9a9a | card borders, control borders, a switch's outline |
| `--text`, `--text-2`, `--text-3` | #1b1b1b, #5c5c5c, #8a8a8a | #f3f3f3, #c8c8c8, #8f8f8f | text, help text, hints |
| `--accent`, `-hover`, `-active` | #4752c4, #3f49b0, #37409c | #5865f2, #6571f3, #4c58d8 | primary button, selection |
| `--accent-text`, `--accent-soft` | #4752c4, 10 % accent | #a5acf8, 22 % accent | links, the selected row |
| `--good`, `--warn`, `--bad` | #0f7b3f, #8a5a00, #c42b2b | #6ccb8c, #f2c661, #ff9a9a | states; `--warn-bg`, `--bad-fill` back them |

Other tokens:
- **Type:** Segoe UI Variable (then Segoe UI, then the system font), three sizes: 12 px / 16 px (help, hints, buttons in
  bars), 14 px / 20 px (body, group headings in semibold), 20 px / 28 px (dialog titles). Code in Consolas.
- **Corners:** 4 px for controls (`--r-control`), 8 px for cards and dialogs (`--r-card`); round only for dots and the
  switch.
- **Motion:** `--t-fast` 120 ms, `--t-move` 160 ms, `--ease` cubic-bezier(.2, 0, 0, 1). Nothing bounces.
- **Size:** the window is 660 x 800 on Windows (660 x 760 of page, after the title bar and border); cards have 12 px
  vertical and 16 px horizontal padding and 8 px between them.

## Changing the window
- A new setting is a schema entry (`internal/store/schema.go`), not new markup: [A new setting](../development.md#a-new-setting).
- Script changes need the real page opened ([how](../development.md#running-the-program-safely)); the tests only prove
  the script parses.
- Retake the README's screenshots only when the part of the window they show changed
  ([Screenshots](../maintaining.md#screenshots)).
- The Fix a match redesign ([fix-a-match.md](fix-a-match.md)) is the next larger change to this page.

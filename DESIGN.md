# slack-tui design system

This is the visual and interaction spec for slack-tui: a Go app built with Bubble Tea and Lipgloss, drawn on a terminal cell grid. Every view reads from the tokens and rules below. When a view needs something the spec doesn't cover, add it to the spec first and then build it.

---

## 1. Principles

1. **One signal per meaning.** Focus, selection, unread, mention, mode and progress each have exactly one visual treatment, and it's the same everywhere.
2. **Hints live in one place.** Key hints appear only on the right of the status bar. Inputs, footers, overlays and placeholders never carry hints.
3. **Three surfaces, nothing else:** a modal, a pane and the full window (§9).
4. **Colour carries meaning, not decoration.** Accent means focus and links, red means it needs you, amber means it's about you or it matched, green means OK or typing, and orange means a warning or searching.
5. **Never jump.** Reserve space for things that load, such as images, and keep column widths fixed.
6. **Nerd Font first, real Slack emoji everywhere.** Icons are Nerd Font glyphs (§3). Emoji are the real thing, including the workspace's custom ones, drawn as images wherever an emoji can appear (§3.1). Plain Unicode is only a fallback, set with `icons = "unicode"`.
7. **Works on any terminal.** Use xterm-256 values with truecolor hex alongside. Light and dark terminals both work, and there's an `ansi` theme that follows the user's terminal palette.

---

## 2. Tokens

Tokens live in `internal/tui/theme.go`. Each token is a `lipgloss.AdaptiveColor`, picked by the terminal background. Views never call `lipgloss.Color` directly. They use a `Styles` struct built once from the active `Theme`.

| Token | Used for | Dark (256 / hex) | Light (256 / hex) | `ansi` theme |
|---|---|---|---|---|
| `bg` | Terminal background. **Never painted.** | default | default | default |
| `raised` | Status bar, modal background, chips, inline code, completion popup | 234 `#1c1d20` | 255 `#eeeeee` | 0 |
| `sel` | Selection background in an **unfocused** frame | 235 `#26282c` | 254 `#e4e4e4` | 8 |
| `selFocus` | Selection background in the **focused** frame, and your own reactions | 235 `#1b2a3e` | 189 `#d7e7fb` | 4 + reverse fallback |
| `fg` | Body text, unread rows, hint keys | 254 `#e4e4e4` | 234 `#1c1c1c` | default fg |
| `muted` | Read rows, secondary text, unfocused frame titles | 249 `#b2b2b2` | 238 `#444444` | 7 |
| `faint` | Times, meta, hint labels, placeholders, system lines | 245 `#8a8a8a` | 242 `#6c6c6c` | 8 |
| `rule` | Idle frame borders, separators | 237 `#3a3a3a` | 251 `#c6c6c6` | 8 |
| `accent` | Focused frame border and title, links, selection bar, NORMAL pill, fuzzy matches | 75 `#5fafff` | 26 `#005fd7` | 12 |
| `onAccent` | Text on any solid pill or chip | = `bg` (`#121315`) | 231 `#ffffff` | 0 |
| `mention` | Edge flag, mention counts, "new" rule, errors, offline chip | 203 `#ff5f5f` | 160 `#d70000` | 9 |
| `self` | Messages that mention you, search-match highlight, inline code text | 221 `#ffd75f` | 94 `#875f00` | 11 |
| `ok` | Live, presence dot, INSERT pill and composer border, ✓ | 114 `#87d787` | 28 `#008700` | 10 |
| `warn` | Reconnecting, SEARCH and CONFIRM pills, pinned | 215 `#ffaf5f` | 130 `#af5f00` | 3 |

**Sender colours.** Hash the user ID into a 10-colour list for the current theme. The lists leave out every token hue, so a name can't be mistaken for a link or a mention.

- Dark: 177 `#d787ff`, 79 `#5fd7af`, 211 `#ff87af`, 147 `#afafff`, 180 `#d7af87`, 141 `#af87ff`, 116 `#87d7d7`, 174 `#d78787`, 187 `#d7d7af`, 218 `#ffafd7`
- Light: 91 `#8700af`, 29 `#00875f`, 125 `#af005f`, 54 `#5f0087`, 97 `#875faf`, 30 `#008787`, 58 `#5f5f00`, 60 `#5f5f87`, 23 `#005f5f`, 89 `#87005f`

**Tinted rows.** These are derived tokens:

- `mentionTint`: dark `#2a2618`, light `#fbf3dc`. Used behind messages that mention you.
- `pinTint`: dark `#1d1b14`, light `#fbf0e4`. Used behind pinned messages.

**Contrast.** Any text token on `bg` or `raised` must reach 4.5:1. The values above pass. Check overrides in the same way.

### Config

```toml
[theme]
name = "auto"        # auto | dark | light | ansi
[theme.overrides]    # any token, as hex or a 256 index
accent = "#5fafff"
```

`auto` uses `lipgloss.HasDarkBackground()`. With `NO_COLOR` set, the app drops every colour, keeps bold and reverse, and draws the edge flag as `!`.

---

## 3. Icons and emoji

Nerd Fonts stay. Icons come from Nerd Font v3 and live in one `icons.go` map, so views ask for `icons.Channel` and never hard-code a codepoint. Set `[ui] icons = "nerd"` (the default) or `"unicode"`. The unicode set is the fallback for machines without a Nerd Font. The mockups on the canvas use the unicode column only because the canvas can't load a Nerd Font.

| Use | Nerd Font (codepoint, name) | Unicode fallback |
|---|---|---|
| Public channel | `U+F292` nf-fa-hashtag | `#` |
| Private channel | `U+F023` nf-fa-lock | `◇` |
| Person, active | `U+F007` nf-fa-user, in `ok` | `●` |
| Person, away | `U+F2C0` nf-fa-user_o, in `faint` | `○` |
| Group DM | `U+F0C0` nf-fa-users | `◎` |
| Unreads entry | `U+F01C` nf-fa-inbox | `≡` |
| CHANNELS section | `U+F292` nf-fa-hashtag | none |
| DIRECT MESSAGES section | `U+F086` nf-fa-comments | none |
| EXTERNAL section | `U+F1AD` nf-fa-building | none |
| Custom section | the section's Slack emoji (§3.1) | none |
| Section folded / open | `U+F0DA` / `U+F0D7` nf-fa-caret_right / caret_down | `▸` / `▾` |
| Thread summary | `U+F112` nf-fa-reply | `↳` |
| Pinned | `U+F08D` nf-fa-thumb_tack | `⚑` |
| Bookmark | `U+F02E` nf-fa-bookmark | `⚑` |
| Scheduled | `U+F017` nf-fa-clock_o | `◷` |
| Edited | none (text `(edited)`) | none |
| Bot | `U+F06A9` nf-md-robot, before the `APP` tag | none |
| Search | `U+F002` nf-fa-search | none |
| DND | `U+F186` nf-fa-moon_o | `☾` |
| Offline | `U+F05AA` nf-md-wifi_off | `✕` |
| Image | `U+F1C5` nf-fa-file_image_o | `▣` |
| PDF / doc / sheet / slides | `U+F1C1` / `U+F1C2` / `U+F1C3` / `U+F1C4` | `▣` |
| Archive / code / audio / video | `U+F1C6` / `U+F1C9` / `U+F1C7` / `U+F1C8` | `▣` |
| Other file | `U+F15B` nf-fa-file | `▣` |
| Link | `U+F0C1` nf-fa-link | `↗` |

These stay plain Unicode in both modes, because they're structure rather than icons:

| Use | Glyph |
|---|---|
| Selection bar, edge flag | `▌` (half block) |
| Plain-unread flag | `▏` (faint) |
| Presence in a header or modal | `●` / `○` |
| System message | `→` |
| Spinner | Braille: `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` (Bubble Tea `spinner.Dot` or `MiniDot`) |
| Back affordance in narrow mode | `‹` |
| Breadcrumb separator | `›` |
| Done / failed | `✓` / `✕` |
| Cursor | Reverse-video cell |

Every icon slot is 2 cells wide (glyph plus a space, or a 2-cell emoji), so columns line up whichever set is active.

### 3.1 Slack emoji, including custom ones

Emoji always come from Slack's real data, never from icons or stand-ins. That covers standard Unicode emoji and the workspace's custom emoji, including aliases (`alias:`) and animated ones.

- **Where they appear:** message text, reactions, user status (sidebar status slot, DM header, profile, switcher, completion rows), your own status in the status bar, custom sidebar section icons, the emoji picker, the `:` completion popup, and status templates in the `:status` suggestions.
- **Custom emoji** are kitty images using Unicode placeholders, exactly **2 cells wide and 1 row tall**, so they sit in the same slot as a standard emoji. Animated custom emoji play while visible and pause off screen, on the same tick as GIFs.
- **Standard emoji** render as the terminal's own emoji glyph, also 2 cells wide.
- **Fallback** (no graphics, images off, or a custom emoji still loading): a `:name:` chip in `faint` on `raised`. A reaction chip keeps its count after the fallback (`:partyparrot: 2`). A loading custom emoji shows the fallback and swaps in place without changing width.
- **Emoji picker:** custom emoji get their own CUSTOM group after FREQUENT, use the same 2-cell images and search by name and alias.
- Large single-emoji messages are not enlarged: they stay 2 cells, like Slack's compact mode.

---

## 4. Layout and frames

### Frames

Every pane is a **frame**: a rounded border (`lipgloss.RoundedBorder()`) with its title set into the top border.

```
╭ ● siko  ⌂ Working remotely · until 23:59 ─────────── ◷ 1 scheduled ╮
│                                                                    │
╰────────────────────────────────────────────────────────────────────╯
```

- The **left label** is the title. It's bold, in `accent` when the frame has focus and `muted` otherwise, followed by context in `muted` or `faint`.
- The **right label** holds meta in `faint`: a count, "in siko", a breadcrumb or a position like "2 of 2".
- Labels get 1 cell of padding on each side and start 1 cell in from the corner.
- The border is `accent` when focused and `rule` when idle. **This is the only focus signal.**
- The composer is a separate small frame under its pane. Its border follows the mode (§7).
- Frames sit ½ cell apart horizontally, with 1 row between a pane and its composer frame.

### Breakpoints

Defaults, configurable as `layout.narrow_cols` and `layout.thread_split_cols`:

| Width | Layout |
|---|---|
| ≥ 140 cols | Sidebar (33) │ Chat (flex) │ Thread (56), when a thread is open |
| 100–139 | Sidebar │ Chat. An open thread replaces the chat frame; esc goes back. |
| < 100 | **Stack mode** (below) |

### Stack mode (narrow)

One frame fills the window at a time.

1. **Conversations.** The sidebar list fills the window. ⏎ opens the selected conversation.
2. **Chat.** Fills the window. The title is `‹ ● siko …` and the right label is the breadcrumb `Børsen › siko`. `t` opens the thread under the selected message, and esc goes back to the list.
3. **Thread.** Fills the window until it's closed. The title is `‹ Thread  in siko` and the right label is `siko › Thread`. esc goes back to the chat.

The status bar stays at every level, and its hints always show how to go in and back.

---

## 5. Focus and selection

| State | Treatment |
|---|---|
| Focused frame | `accent` border and `accent` bold title |
| Unfocused frame | `rule` border and `muted` bold title |
| Selected row in the focused frame | `selFocus` background across the full row, plus a `▌` in `accent` in the bar column |
| Selected row in an unfocused frame | `sel` background only, with no bar |

This applies to every list: sidebar, messages, threads, search results, modals, panes, completion popups and tabs. `tab` cycles focus between frames.

---

## 6. Sidebar

### Row anatomy

All columns are fixed width:

```
[flag 1][icon 2][name ………… flex, ellipsis][status 2][count →]
```

| Row state | Flag | Name | Count |
|---|---|---|---|
| Read | none | `muted` | none |
| Unread, no mention | `▏` faint | `fg` bold | none |
| Mention, or DM unread | `▌` `mention` | `fg` bold | `mention` number |
| Muted channel | none | `faint` | none |
| Your own DM | none | `muted` + `(you)` faint | none |

- The icon is the Nerd Font glyph for the row type (§3). For people, presence is shown by the icon itself: a solid user icon in `ok` when active and an outline user icon in `faint` when away.
- The status slot shows the person's real Slack status emoji, custom ones included (§3.1).
- Custom sections show their Slack emoji in the header icon slot.
- Nicknames go after the name in `faint` (`Kim Kyhlensø kiky`).
- **Section headers** are `faint` and bold in caps, with `▾`/`▸` and the total count on the right in `faint`.
- A **folded** section still surfaces what's inside it. With mentions it shows a red `▌` and a red count. With plain unreads it shows a faint `▏` and "3 new" in `muted`.
- The sidebar frame title is the team name (`muted`) and the right label is your handle (`faint`).
- The first row is `≡ Unreads` with its total.

---

## 7. Messages and composer

### Message anatomy

```
[bar ½][gap ½][time 6][body ……………………………]
```

- The **time gutter** is 6 cells and `faint`. It appears on the first line of every message, including follow-ups.
- The **sender line** is the name in its hashed colour, bold, then the handle in `faint`. Messages from the same sender within 5 minutes skip the sender line.
- **Date separator:** `──── Thursday 22 October ────` in `faint`, starting after the gutter.
- **New messages:** a `mention`-coloured rule across the body column, with a bold `new` label at its right end.
- **Links** are `accent` and underlined.
- **Inline code** uses the `raised` background with `self` text. Code blocks use the `raised` background with full-width padding.
- **Reactions** are chips (`raised` bg, `♥ 1`) with 1 cell between them. Reactions you added use the `selFocus` bg with `accent` bold text. Custom emoji are 2-cell images that fall back to `:name:` in `faint` on a `raised` chip.
- **Thread summary:** `↳ 4 replies · last 2h ago` in `accent`.
- **Mentions of you:** `self` bar plus `mentionTint` background. `@mada` in the text is `self` bold.
- **Pinned:** `warn` bar plus `pinTint` background, with `⚑ pinned` in `warn` after the sender.
- **Edited:** `(edited)` in `faint` after the text.
- **System messages:** a single line `→ siko pinned a message`, `faint` and italic, with the time in the gutter.
- **Bots:** the name, then an `APP` tag (`sel` bg, `muted` text).
- **Tall messages:** the sender line stays pinned with `· lines 1–4 of 22` in `faint`. The last visible row reads `▾ 18 more lines` in `accent`.
- **Top of history:** `── older messages above ──` centred in `faint`. The `k` hint goes in the status bar.

### Composer

- The composer is a frame under its pane with 1 line of content that grows to 6.
- **Border by mode:** `rule` when idle in NORMAL, `ok` in INSERT, `warn` in SEARCH.
- **Labels sit in the top border, left side,** in the border's colour and bold: `editing`, `broadcast`, `3 of 10 attachments`.
- The **placeholder** is `Message siko` / `Reply in thread` in `faint`, with no hints.
- The thread composer shows `□ also send to siko` on the right in `faint`. When it's on, it becomes `■ also send to siko` in `warn`.
- **Attachments:** a row of chips above the text, one per pasted image. Each chip is a 1-cell-tall kitty thumbnail 4 cells wide, then the name, on `raised`. The selected chip uses `selFocus` + `accent`. ctrl+←/→ picks a chip and ctrl+x drops the selected one.
- **Completion popup** (`@` `#` `:`): stacked flush on the composer's top edge, aligned to its left edge, on `raised`, up to 6 rows. Rows follow the modal row grammar, with fuzzy-matched characters in `accent` bold.

---

## 8. Status bar

One row on `raised`, always present, including in the image viewer. It has three zones:

```
[mode pill] [connection] [your status] [transient message ………]        [hints →]
```

### Mode pills

Each pill is bold text in `onAccent` on a solid background:

| Pill | Background | When |
|---|---|---|
| `NORMAL` | accent | Default |
| `INSERT` | ok | Typing in the composer |
| `SEARCH` | warn | Search query or results |
| `JUMP` | accent | ctrl+k switcher |
| `REACT`, `PROFILE`, `OPEN` | accent | The matching modal |
| `VIEW` | accent | Image viewer |
| `CONFIRM` | warn | A y/n question |
| `:` | fg | Command line |
| `ERROR` | mention | Fatal screen |

### Hint grammar

Hints follow the pattern `key label   key label   key label`: the key in `fg` bold, the label in `faint`, 3 spaces between groups and **at most 5 groups**, with `? help` last when it fits. Hints change with the mode and with the focused frame.

### States

| State | Middle zone |
|---|---|
| Live | `● live` in ok |
| Your status | `✚ First Responder` in muted, `☾ dnd · 45m` in faint, `○ away` in faint |
| Progress | `⠹ checking unreads  12 of 80 channels`: an accent spinner, `muted` text and a `faint` count. **No progress bars.** |
| Result | `✓ saved to ~/Downloads/plan (1).pdf` in ok. It fades after about 4 s, and its hint is `o open it`. |
| Confirm | `CONFIRM` pill, then the question in `fg` and the consequence in `faint`. Hints: `y delete   n keep`. |
| Reconnecting | `◌ reconnecting…` in warn |
| Offline | A solid `mention` chip `✕ offline`, then `showing cached messages · retry in 24s` in muted, with the hint `R retry now` |
| Command line | The whole bar becomes the input after a `:` pill. Suggestions stack above it using the completion popup. Hints: `tab complete   ↑↓ history   ⏎ run   esc cancel`. |

---

## 9. Surfaces

### Modal: short-lived, pick something and go

- Centred horizontally, with its top at about 18% of the height. Width depends on content, at most 76 cells.
- `raised` background, `accent` rounded border, and the title in the top border (left: bold `accent`; right: context in `faint`).
- Everything behind it is re-rendered in `faint`, which acts as the dim.
- Layout: input row (`› query▏`), then a rule, then rows using the selection grammar. Groups get a `faint` bold caps header. **No footer.**
- The mode pill names the modal, and its hints are in the status bar.

| Modal | Width | Contents |
|---|---|---|
| Switcher (ctrl+k) | 76 | Rows: icon, name with fuzzy matches in accent, and meta on the right in faint (`channel`, `3 new · channel`, `DM · away`, `you`). People you have no DM with appear once you type, with their real name in faint. Title `Jump to`, right label `4 of 186`. |
| Emoji picker (+) | 50 | Title `React`, right `to siko 10:25`. Groups FREQUENT and CUSTOM. Rows: glyph (2 cells), `:name:`, and `✓ added` in ok on the right. ⏎ toggles. |
| Profile (I) | 64 | Title `Profile`, right `@handle`. Line 1: presence dot and bold name, then `display · pronouns` in faint. Line 2: their title. A rule, then fields in a 12-cell label column (Status, Presence, Local time "+1h vs you", Email, Phone, Workspace). Slack Connect gets an `EXTERNAL` tag. A deactivated account gets a red line in place of Presence. |
| Open chooser (o) | 66 | Title `Open`, right `3 items · siko 11:20`. Rows: number 1–9 in faint, kind tag, name, size on the right. Links use kind `URL` with the address in accent. |

### Pane: browse a list and act on it

- **Replaces the chat frame.** It uses the same frame, with the title `Search  permissions  in:@siko` and the right label `2 of 2`.
- The thread frame on the right becomes a **Preview** of the selected item's context (or hides below the split breakpoint).
- Results are grouped under `faint` bold headers, such as the conversation with its count.
- Inputs (search query, filters) live in the composer frame under the pane, using that mode's border colour.
- esc returns to the conversation. Search results are kept, and n/N still step through them.

| Pane | Notes |
|---|---|
| Search (/) | Result block: date in the gutter, sender plus time, up to 3 lines with matches highlighted (`raised` bg, `self` bold), `in thread` in accent, and thumbnails. Completion for `from:` and `in:`. |
| Files & links (F) | Tabs in the title as `Files 14 · Links 40+`, with the active tab in accent bold and the others in muted. File rows use the file line (§10) plus sender and relative time. Link rows: label, URL in accent, sender, `in thread`. |
| Pins & bookmarks (P) | BOOKMARKS group, then PINNED. Bookmarks are edited in the composer frame. |
| Scheduled (:scheduled) | Grouped by conversation. `e` takes one back into the composer and `d` cancels it. |
| Channel browser (:join) | Filter in the composer frame. Row meta: member count, and `joined` in faint. |
| Members (:members) | Presence dot, name, title in faint. ⏎ opens the profile. |
| Unreads | One block per conversation: a header row (icon, name, flag and count), `… N earlier` in faint, then the last 5 messages using normal message anatomy (3 lines max each). |
| Help (?) | Filters by key (`P`) or by words (`pin`). Rows: key in fg bold (8 cells), description, and `:command` in faint. ⏎ runs the command. |

### Full window: media only

- Only the **image viewer** (v). It replaces the sidebar and frames, but **keeps the status bar.**
- The header row has the filename in accent bold, the sender, `in siko · Mon 19 Oct 19:38` in faint, and `← 3 / 12 →  in this conversation` on the right. An accent rule sits under it.
- The image is centred and fitted. Under it, `fit · kitty graphics` is centred in faint.
- A **Details** panel (40 cells, left rule, toggled with `i`) shows From, In, Posted, File and Zoom, plus the next message in the chat.
- Hints: `← → browse   o open in loupe   s save   i details   esc close`.

---

## 10. Media and files

### File line

Every attachment has one, in every state:

```
<nf image icon> image.png  PNG · 214 KB
<nf pdf icon>   plan.pdf   PDF · 1.2 MB
```

- The **icon** is the Nerd Font file-type glyph from §3, in `accent` for images and `muted` for everything else. It's neutral otherwise, with no colour per type.
- The name is in `fg`. The **kind** (the extension in caps) and the size are in `faint`.
- With `icons = "unicode"`, the icon becomes `▣` and the kind is drawn as a tag instead: bold `fg` on `sel`, with 1 cell of padding (`[PDF] plan.pdf  1.2 MB`), as on the canvas.
- This fixes the old gap where non-image files had no treatment.

### Inline images

- The file line comes first, with the thumbnail below it in a rounded frame (`rule` border, `accent` when the message is selected).
- Thumbnails are capped at `image_max_cols × image_max_rows` (default 40×12) and keep their aspect ratio, snapped to cells. They're drawn with kitty Unicode placeholders, so they scroll and clip with the text.

| State | Treatment |
|---|---|
| Loading | `▣ image.png  loading…` with the marker in faint, plus a **dashed** frame reserved at the final cell size from the file metadata. The layout doesn't move when the image arrives. |
| Failed | A single line `▣ image.png  couldn't load · 403`, with the marker and reason in `mention`. No frame. `r retry` in the hints when selected. |
| No graphics or images off | The file line only. `v` still opens the viewer, which falls back to `image_viewer` / xdg-open. |

- **GIFs:** the file line meta reads `GIF · ▶ playing` while on screen and `GIF · ❚❚` when paused off screen. Only visible GIFs tick.
- **Custom emoji:** see §3.1.
- **Save (s)** never overwrites (`plan (1).pdf`). The result goes to the status bar.
- **Copy (c)** uses OSC 52.

---

## 11. Startup and error screens

Both keep the status bar.

- **Loading:** the team name in bold, then a step list. Each step is `✓` (ok) when done, the accent spinner while running, or `·` (faint) when pending: `cache loaded`, `connecting`, `channels & people`. The status bar shows `slack-tui · cache from 09:58` in faint.
- **Fatal error:** `✕ Couldn't start` in mention bold, then the reason in muted, then the config path in faint. `ERROR` pill, with hints `e edit config   q quit`.

---

## 12. Keys introduced by this design

Everything else stays as it is today.

| Key | Where | Action |
|---|---|---|
| `tab` | Normal | Cycle focus between frames |
| `esc` | Stack mode | Back one level (thread → chat → list) |
| `t` | Chat | Open the thread (also enters the thread level in stack mode) |
| `R` | Offline | Retry now |
| `r` | Failed image selected | Retry loading |
| `i` | Image viewer | Toggle the Details panel |
| `ctrl+←/→` | Composer with attachments | Pick an attachment chip |
| `ctrl+x` | Composer | Drop the **selected** attachment (previously all of them) |
| `e` | Fatal error screen | Open the config in `$EDITOR` |
| `/` | Conversation list in stack mode | Filter conversations (optional, if you add sidebar filtering) |

---

## 13. Implementation notes

Gaps found while building this spec, and how they were filled.

- **Light `ok` and `warn` failed 4.5:1 on `raised`** (#008700 was 4.05:1 and #af5f00 was 4.06:1 on #eeeeee). They are now `ok` #007a00 (256: 22) and `warn` #a05500 (256: 130, the nearest; 94 is taken by `self`).
- **Dark `selFocus` is 235 in 256-colour mode**, the same index as `sel`, as specified. On 256-colour-only terminals, focused and unfocused selection then differ only by the `▌` bar. Truecolor terminals get #1b2a3e.
- **`bg` hex values** (dark #121315, light #ffffff) are what the contrast check assumes for the terminal background. They are never painted.
- **`NO_COLOR`** forces lipgloss's ANSI profile with every token unset. lipgloss's own NO_COLOR handling would drop bold and reverse as well.
- **Theme warnings** (an unknown token, an unparsable value, or contrast below 4.5:1) show in the status bar at start.
- **Frames sit 1 cell apart.** A terminal can't draw half a cell, so the gap is one blank column. A pane and its composer frame have 1 blank row between them.
- **Focus under `NO_COLOR`.** With no border colour, the focused frame's title is drawn in reverse video. This is the only place where focus shows without colour.
- **Selection rows** come from `selLine` in `frame.go`: a 1-cell bar column, then the row padded to the frame's width. `paintBg` restores the selection background after every SGR reset inside the row, so chips, links and custom-emoji images keep it.
- **Folded sections** keep only the open conversation as a row. Everything else is summed up in the header's flag and count. Previously, unread rows stayed visible under a folded header.
- **Muted channels** (the `faint` row state) aren't drawn, because the client doesn't load Slack's mute preferences. That would be a data-layer change, which is out of scope here.
- **The open conversation** is drawn in `fg` rather than `muted` when it's read, so it stands out when the cursor is elsewhere. With the cursor on it, it's the selected row.
- **The sidebar count column** is as wide as the widest count among the visible rows, so names, status emoji and counts line up.
- **Section header arrows** (▾/▸) sit in the icon column. The section's own icon (Nerd Font, or the custom section's Slack emoji) goes before its name.
- **People's presence** is drawn with the solid or outline user icon in the sidebar. Headers and modals keep `●` (§3).
- **The time gutter** is `max(6, width of time_format + 1)` cells, so a 12-hour `time_format` still fits.
- **Mentions of you** include `@here`, `@channel` and `@everyone`, as Slack highlights those too. When a message both mentions you and is pinned, the mention tint wins. Selection wins over both.
- **Day labels** are `Today`, `Yesterday`, then `Thursday 22 October` (the year is added when it isn't the current one).
- **Completion popup** is 56 cells wide, at most (or the composer's width when it's narrower). Its bottom row lies on the blank row above the composer, and the rows above it cover the bottom of the pane. Rows that don't fit their meta truncate it with `…`.
- **Attachment chips** without graphics show the image icon in place of the thumbnail. The thumbnail is drawn once, 4×1 cells, and kept with the other images so it survives a tmux pane switch.
- **Composer labels** are joined with ` · ` when there are several (`editing · 2 of 10 attachments`).
- **The design canvas** ("Dark · normal mode", "Light · insert mode") wasn't available while building this, so the chat was checked against this spec's text and the golden renders instead.
- **Pills for panes** (pins, files and links, scheduled, the channel browser, members, help, unreads) are `NORMAL`, since §8 doesn't name one for them.
- **`? help`** is left out of the hints while typing (INSERT, the command line, a filter input, CONFIRM), because `?` is a character there. When room is short, real hints come before `? help`.
- **Status messages** fade after 4 s, all of them. They come in three kinds: results (`✓` in `ok`, from `setResult`), errors (`✕` in `mention`) and notes (`muted`).
- **`R` while offline** retries the start when the cache is showing. When Socket Mode itself has given up, it starts the live connection again. Either way the bar counts down to the next automatic retry.
- **The first connect** shows `◌ reconnecting…` too, because §8 has no separate "connecting" state.
- **The image viewer** now keeps the status bar (its caption line moves up one row). That change comes from §8, ahead of the rest of phase 7.

# Design — Collapse multi-line pastes into an atomic, highlighted chip (turbotui half of gogent #501)

## Summary

When a user pastes text **containing a line break** into a `MultiLineInput` (and, for
parity, a `TextBox`), collapse it in place into a single atomic, highlighted **chip**
rendered as `[pasted N lines]` instead of spilling N real editable lines into the
buffer. The chip is one atomic unit to the caret: arrows jump over it, one Backspace
(caret after) / Delete (caret before) removes the whole block, selection treats it
atomically, copy/cut yields the full original text, and `GetText()` returns the
verbatim original (newlines restored). Single-line pastes (no `\n`) keep today's
literal-insert behaviour. CR is still dropped on ingest (CRLF→LF).

This is the **turbotui half only**. No gogent files are touched. After it merges, the
gogent half wires the two new theme roles into `ui/tui/theme.go` and re-verifies its
input-driven flows; that is dispatched separately.

---

## Approach decision: B (sentinel rune + side store), not A (segment model)

The issue *recommends* Approach A — replace the flat per-line `string` with a
`[]segment` sequence and make the caret coordinate `(line, segment, offset)`. The
issue also explicitly permits Approach B ("marker rune + side store … acceptable …
note the choice in a comment") and asks the design to weigh both. After reading the
widget, **B is the better fit for this codebase** and is what this design adopts. The
deciding factors:

1. **Exported API preservation (gate 4 — the seam).** `MultiLineInput.Lines []string`,
   `CursorX int`, `CursorY int` and `TextBox.Text []rune`, `TextBox.Cursor int` are
   **exported** fields of a published toolkit. Approach A changes their *types*
   (`Lines` → `[][]segment`, the caret → a 3-tuple). That is a breaking change to the
   public surface that gogent (the thin consumer) and any other embedder may bind to.
   Approach B keeps every existing exported field and its type **unchanged**; the only
   additions to the public surface are the two new theme roles. The gogent side stays
   a thin consumer exactly as the orchestration note wants.

2. **Regression surface (gate 3).** The existing tests assert on the *concrete values*
   of these fields — e.g. `input.Lines[0] != "ahi"`, `input.CursorX != 6`,
   `input.selAnchorY, input.selAnchorX = 2, 1`. Approach A invalidates essentially the
   whole `widget_multiline_input_*_test.go` suite and the intricate, perf-tuned,
   allocation-free caret/wrap layer (`cursorVisualPos`, `cursorRowCol`,
   `caretSpanOffsetCol`, `visualPosToCursorFromRows`, `wrappedRows`). Approach B leaves
   the **editing, caret-motion, and selection logic untouched** because a chip is
   represented as exactly **one rune** in the existing line string — naturally atomic
   to a rune-offset caret (arrow = ±1 rune, one Backspace deletes the one rune). The
   surgery is confined to the **visual-width layer** (wrap + draw + display-column
   math), where a chip rune renders as `displayWidth(label)` columns instead of 1.

3. **Collision safety.** A chip is encoded as a unique code point drawn from
   Supplementary Private-Use Area-A (`U+F0000…U+FFFFD`, 65 533 slots), allocated per
   chip; a side map `chips map[rune]string` holds each chip's verbatim original text.
   All ingest paths (typed runes, paste, `SetText`) **strip any pre-existing
   private-use chip rune** so user content can never collide with the marker. This is
   documented in a comment at the marker definition.

The cost of B is the sentinel + ingest sanitisation and a width-aware wrap/draw pass.
That is a smaller, better-isolated change than rewriting the entire caret model, and
it preserves the public API and the test suite. **The choice and its rationale are
recorded in a comment** in `widget_multiline_input.go` (per the issue's instruction).

---

## Files & functions touched (all under `turbotv/`)

### New: `turbotv/paste_chip.go` (shared model — used by both widgets)
Keeps the chip logic in one place so `TextBox` parity is genuinely "share the model".

- `const chipRuneBase = 0xF0000` and `chipRuneEnd = 0xFFFFD` — SPUA-A range; doc
  comment explains the marker scheme and collision-avoidance contract.
- `isChipRune(r rune) bool`.
- `chipLabel(text string) string` — `"[pasted N lines]"`, `N = strings.Count(text,"\n")+1`.
  N is always ≥ 2 for a chip (a chip only exists when `\n` is present), so the label is
  always plural; no `1 line` case.
- `chipLabelFit(text string, width int) []rune` — returns the label truncated to
  `width` display columns with a trailing `…` when it overflows
  (`"[pasted 12 lines…]"`), hard-cutting to `width` when even the ellipsis cannot fit.
  Uses `tui.StringWidth`/`tui.RuneWidth`. Full text is never altered; only the label
  shrinks.
- `sanitizePasteRunes(s string) (clean string, hasNewline bool)` — drops `\r`, drops
  other control runes `< 0x20` except `\n`, **drops any SPUA-A chip rune**, and reports
  whether a `\n` survived. Shared by both widgets' `handlePaste` and `SetText`.
- `chipStore` helper type (or two small methods per widget): `add(text) rune`,
  `text(r) rune→string`, `prune(present func(rune) bool)` to GC entries whose marker
  rune no longer appears after a delete. (A widget owns one `map[rune]string` plus a
  small monotonically-increasing counter for allocation; the counter is per-widget and
  never reset so a deleted+recreated chip can't alias a live one.)
- `expandRunes(runes []rune, store map[rune]string) string` — turns a rune slice
  (possibly containing chip markers) into real text by replacing each marker with its
  stored original. Backs `GetText`, `selectionText`, copy/cut.

### `turbotv/widget_multiline_input.go` (primary)
- **Struct**: add `chips map[rune]string` and `nextChip rune` (private). No change to
  `Lines`, `CursorX`, `CursorY`.
- `GetText()` (~L100): join lines, but expand chip markers per line via `expandRunes`
  before joining with `\n`. For chip-free buffers the output is byte-identical to today.
- `SetText(text)` (~L104): **documented new rule** — if `text` contains `\n`, build a
  single line holding one chip marker for the whole multi-line content (history recall
  of a pasted prompt restores the chip); a no-newline `SetText` stays literal. The
  **constructor `NewMultiLineInput` keeps today's literal `strings.Split`** (initial
  content is not a paste/recall) — this split is deliberate and commented, and is why
  the existing `NewMultiLineInput("abc\ndef\nghi", …)` tests keep their multi-line
  meaning.
- `handlePaste` (~L427): run `sanitizePasteRunes`; if a `\n` survived, `deleteSelection`
  then insert **one** chip marker rune (storing the cleaned verbatim text); otherwise
  fall back to today's literal per-rune insert. So `GetText` after a chip paste equals
  what a literal multi-line insert would have produced — collapsing is purely visual.
- **Editing / caret / selection logic — unchanged in behaviour** because a chip is one
  rune: `insertRune`, `backspace` (~L444), `forwardDelete`, `newLine`, `moveLeft/Right`
  (~L476), `moveUp/Down`, `deleteSelection` (~L337), `isSelected`, `extendOrClear`,
  `selectionOrdered`, `handleClick`. The only edits here are **chip-store GC**: when a
  delete (`deleteSelection`, `backspace`, `forwardDelete`) removes a span, prune any
  chip markers that were in it. (`moveLeft/Right` already step ±1 rune → jump over the
  chip; `backspace`/`forwardDelete` already remove one rune → remove the whole chip.)
- `selectionText` (~L312): after slicing the rune range (same-line and multi-line
  branches, unchanged), pass the result through `expandRunes` so copy/cut yield the
  chip's **full** original text, not the label. `copySelection` is then unchanged.
- **Width-aware visual layer — the real surgery, confined here**:
  - `wrappedRows`/`lineSpans`/`wordWrapSpans` (~L549–630), `rowsForLine` (~L657),
    `totalVisualRows` (~L642): generalise from "1 rune = 1 column" to **token widths**,
    where a normal rune is a width-1 token and a chip marker is an **unbreakable** token
    of width `displayWidth(chipLabelFit(text, contentWidth))`. A chip never splits
    across rows; if it alone exceeds the content width it is truncated to fit. For a
    **chip-free line every token is width 1, so the spans are identical to today** — the
    existing char-wrap (`start += width`) and word-wrap arithmetic is preserved on the
    fast path (guarded by a "line contains a chip?" check) so all existing wrap tests
    pass unchanged.
  - `cursorVisualPos` (~L738), `cursorRowCol` (~L783), `caretSpanOffsetCol` (~L679),
    `CaretRowInLine` (~L698): compute the caret's display column by summing token widths
    up to `CursorX` (chip-free lines keep the existing `CursorX/width`, `CursorX%width`
    arithmetic on the fast path). `CaretRowInLine` therefore counts a chip's single
    visual row correctly (test 9).
  - `draw` (~L119) inner loop: walk a row's runes accumulating **display columns**; a
    normal rune paints one cell (as today), a chip marker paints its `chipLabelFit`
    label across `displayWidth` cells using `PasteChipFG/BG` (and `TextSelectionFG/BG`
    when the chip is within the current selection, since a chip is one selectable unit).
    `handleClick` maps a clicked display column back to a rune offset, landing the caret
    **before or after** the chip — never inside.

### `turbotv/widget_textbox.go` (parity — same model)
- **Struct**: add `chips map[rune]string`, `nextChip rune`.
- `handlePaste` (~L323): currently drops `\n`; change to `sanitizePasteRunes` → if a
  `\n` survived, `deleteSelection` then insert one chip marker; else literal insert
  (unchanged). This makes a multi-line paste collapse to a chip in the single-line box
  too.
- `GetText`/`SetText` (~L50): `GetText` expands chip markers via `expandRunes`;
  `SetText` with `\n` builds a single chip marker (same documented rule as MLI).
- `insertRune`, `backspace`/forward-delete, `moveCursor`, `deleteSelection`,
  `selRange`, `hasSelection`, `handleClick` — **unchanged** (chip = one rune).
  `copySelection`/`cutSelection` (~L290): expand markers in the sliced range so the
  clipboard gets the full original; GC the store on cut/delete.
- `wordBoundaryLeft/Right` + `handleCtrlShortcut` (~L158–240): treat a chip marker as
  its own boundary class (one word-unit) so Ctrl+←/→ and Ctrl+Backspace stop at /
  remove the chip as a whole. Ctrl+A already selects all including the chip (atomic).
- `draw` (~L60) + `cursorPos`/`ensureCursorVisible` (~L92, L334): become
  display-column aware so the chip renders as its themed label and horizontal scroll /
  caret placement account for its width. Chip-free text keeps today's 1-col-per-rune
  path.

### `turbotv/theme.go`
- Add two roles to `Theme`: `PasteChipBG`, `PasteChipFG tui.Color`, documented next to
  `TextSelectionBG/FG` (same "drawn over the focused-input fill, must contrast with
  `InputFocusBG` and be distinct from `TextSelectionBG/FG`" rationale).
- Defaults:
  - `DefaultTheme`: `PasteChipBG = tui.ANSIColor(5)` (magenta), `PasteChipFG =
    tui.ANSIColor(15)` (bright white) — a distinct "tag" colour, clearly separate from
    the cyan `InputFocusBG(6)` and the black/white `TextSelection` pair.
  - `HighContrastTheme`: `PasteChipBG = tui.ANSIColor(11)` (bright yellow), `PasteChipFG
    = tui.ANSIColor(0)` (black) — luminance-driven, colour-blind-safe, matching the
    preset's hue-free philosophy and distinct from the white `InputFocusBG(15)` /
    black `TextSelectionBG(0)`.
  - Exact codes are tunable; the constraint is contrast-with-focus-fill and
    distinct-from-selection.

### `turbotv/desktop.go` — **no change (verified)**
`handlePaste`/`deliverPaste`/`pasteClipboard` (~L1019–1110) only route the paste text
to the focused widget via `BubblePaste`; the chip-creation decision lives in the
widget's `handlePaste`. Confirmed nothing here needs to know about chips. Left
unchanged.

### `turbotv/menu.go` — **not touched** (concurrent task owns it).

### Tests — `turbotv/widget_multiline_input_test.go`, `_wrap_test.go`,
`_caretrow_test.go`, `_clamp_test.go`, `widget_textbox_test.go`, plus a new
`turbotv/widget_paste_chip_test.go`
Existing tests stay green (chip-free paths are behaviourally identical). New cases
cover the issue's list 1–10 (see Tests section).

---

## User-facing behaviour

- **Single-line paste** (no `\n`): unchanged — characters insert literally, caret steps
  rune-by-rune.
- **Multi-line paste**: collapses in place to one highlighted `[pasted N lines]` token.
  The box does **not** grow to N lines. The chip is themed via `PasteChipBG/FG`,
  distinct from the focus fill and from text selection.
- **Caret**: Left/Right jump over the chip; the caret can sit only **before** or
  **after** it, never inside. Up/Down treat the chip's row as one visual row.
- **Delete**: one Backspace with the caret immediately after the chip, or one Delete
  immediately before it, removes the entire pasted block.
- **Selection**: a drag (or Ctrl+A in `TextBox`) that touches the chip includes it
  **whole**; typing or pasting over a selection that contains the chip replaces it
  wholesale.
- **Copy / cut**: yields the chip's **full** original multi-line text (newlines
  intact), never the visible label — clipboard round-trips losslessly.
- **Submit / `GetText()`**: returns the verbatim original with newlines restored, so
  the host (gogent) sees exactly the text the user pasted; submit is unchanged.
- **History recall**: `SetText` of a previously-pasted multi-line prompt restores it as
  a chip (so the prompt box shows the compact token again rather than exploding to N
  lines).
- **Overflow**: a chip wider than the input truncates its label with `…`; the full text
  is retained internally and still copies/submits in full.

---

## Design criteria (the four gates)

**(1) Goal match.** Exactly the issue's ask: multi-line paste → one atomic
`[pasted N lines]` chip; single-line paste unchanged; `GetText()` verbatim; the chip is
atomic to caret / Backspace / Delete / selection / copy / cut; `SetText` with newlines
recreates the chip; CR still dropped. No scope creep — no new widgets, no behaviour
change for chip-free buffers.

**(2) Usability.** The caret can never enter the chip (only before/after); one
keystroke deletes it; selection, typing-over, and word-wise motion behave atomically;
the chip is clearly surfaced as a themed highlighted token (not silent — the user sees
"N lines were pasted") and truncates gracefully when too wide. The verbatim text is
never lost, so submit/recall are faithful to what the user pasted.

**(3) No regressions.** The editing/caret/selection logic and the exported
`Lines/CursorX/CursorY` (and `Text/Cursor`) fields are unchanged; chip-free code paths
are behaviourally identical (the width-aware wrap reduces to today's arithmetic when
every token is width 1, guarded by a per-line "has chip?" check). Existing
`widget_multiline_input_*` and `widget_textbox_test.go` suites stay green. Ingest
sanitisation prevents the marker rune from corrupting user content. `gofmt` / `go vet`
/ `go build` / `golangci-lint` / `go test ./...` are the authoritative local gate
(turbotui has no CI) and must be clean/green before hand-off.

**(4) Holistic across both repos.** The change is confined to `turbotv/`
(`widget_multiline_input.go` primary, `widget_textbox.go` parity, new shared
`paste_chip.go`, `theme.go` roles); `menu.go` untouched; `desktop.go` verified
unchanged; no new external dependencies (stdlib + existing `tui` width helpers only).
The seam to gogent is respected: the public widget API is **unchanged** in shape, the
only additions are the two `PasteChip*` theme roles. The downstream gogent half is then
a thin consumer — it wires `PasteChipBG/FG` into `ui/tui/theme.go` alongside
`InputFocus*`/`TextSelection*` and re-verifies @-mention completer, slash-commands,
history recall, submit/interject, and typing-idle drain. Because `GetText()` still
returns verbatim text and `OnPasteFn` still bubbles through `BubblePaste`, gogent's
typing-awareness and submit paths see no behavioural change beyond the buffer no longer
ballooning on a paste. **No gogent file is touched in this half.**

---

## Regression risks & mitigations

- **Width-aware wrap/draw rewrite** is the highest-risk area (the existing code is
  perf-tuned and assumes 1 rune = 1 column). *Mitigation*: keep a per-line "contains a
  chip?" fast-path that runs the existing arithmetic verbatim for chip-free lines; only
  lines with a chip use the general token-width walk. Existing wrap/caretrow/clamp tests
  exercise the fast path and must stay green.
- **Marker-rune collision** if a user's real content contained a SPUA-A code point.
  *Mitigation*: strip the SPUA-A chip range on every ingest path
  (`sanitizePasteRunes`, typed-rune insert guard, `SetText`); document the reserved
  range. Realistic prompt text never contains these code points, and they are removed
  if it somehow does.
- **`SetText` semantics change** (newline → chip). *Mitigation*: no existing
  `MultiLineInput`/`TextBox` test calls `SetText` with a `\n` (verified — only a
  `TextView` test does, a different out-of-scope widget), so this is purely additive;
  documented in a comment. The **constructor stays literal**, preserving the existing
  `NewMultiLineInput("a\nb\nc", …)` multi-line tests.
- **Exported `Lines` may now contain a marker rune**, which a consumer reading
  `input.Lines` directly (instead of `GetText()`) would see. *Mitigation*: `GetText()`
  is and remains the documented accessor; gogent uses it. Note this in the field doc.
- **Chip-store growth** across repeated paste→delete cycles. *Mitigation*: GC the map
  when a delete removes a marker rune (`deleteSelection`/`backspace`/`forwardDelete`/
  `cutSelection` prune entries no longer present in the buffer).
- **Wide-rune interaction**: the widget already assumes width-1 for normal runes; this
  design does not change that for text, only introduces honest multi-column width for
  the chip token. No new wide-rune obligations are taken on for ordinary text.

---

## Tests (turbotv) — to add/extend

New `widget_paste_chip_test.go` (+ extensions to existing input tests):

1. Paste single-line → literal insert; caret steps rune-by-rune (existing behaviour).
2. Paste `"a\nb\nc"` → one chip `[pasted 3 lines]`; `GetText() == "a\nb\nc"`; buffer
   stays one logical line.
3. Chip at end, one Backspace → chip gone, buffer empty.
4. Chip mid-line: `←` from after lands the caret **before** the chip (never inside);
   `→` from before lands **after**.
5. Select-all then type a rune → chip replaced wholesale.
6. Copy a chip → clipboard / `copySelection` returns the multi-line original.
7. `SetText("x\ny")` → restored as a chip; `GetText() == "x\ny"`.
8. WordWrap=true: a chip is not split across visual rows (`wrappedRows`/`lineSpans`).
9. `CaretRowInLine` accounts for the chip occupying one visual row.
10. `TextBox` parity: multi-line paste collapses to a chip; `GetText()` round-trips;
    Ctrl+←/→ treats the chip as one word-unit; cut yields the full original.

Plus a guard: pasting text that already contains a SPUA-A code point has it stripped
(no collision). Keep `widget_multiline_input_test.go` / `_wrap_test.go` /
`_caretrow_test.go` / `_clamp_test.go` / `widget_textbox_test.go` passing unchanged.

---

## Open questions

1. **Chip styling reuse** — render the chip as a plain reverse-video token
   (`PasteChipBG/FG`) only, or also bracket it with a subtle marker glyph? This design
   uses the literal `[pasted N lines]` text in chip colours (no extra glyph), which is
   unambiguous and width-predictable. Confirm that is the desired visual.
2. **`PasteChip*` default colours** — proposed magenta/bright-white (Default) and
   yellow/black (HighContrast). These are placeholders meeting the contrast constraints;
   a maintainer may prefer different ANSI codes.
3. **Constructor vs `SetText` split** — confirmed intent: `NewMultiLineInput`/
   `NewTextBox` keep literal multi-line behaviour (initial content), only `SetText`
   chip-ifies a newline-bearing value (history recall). Flag if the constructor should
   also chip-ify.
4. **Approach A vs B** — this design deviates from the issue's *recommended* Approach A
   to B, justified by exported-API/test-suite preservation (the issue permits B and
   asks for the choice to be noted). If preserving the public `Lines`/`CursorX` shape is
   *not* a hard requirement (e.g. gogent provably never touches those fields and a
   breaking toolkit bump is acceptable), Approach A's cleaner caret model could be
   reconsidered — but B is the lower-risk, seam-respecting choice and is recommended.

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
input-driven flows; that is dispatched separately. This design **enumerates the exact
gogent seam points the gogent half must handle** (see §Seam), so that half is a thin,
well-scoped consumer rather than a discovery exercise.

---

## Approach decision: B (sentinel rune + side store), not A (segment model)

The issue *recommends* Approach A (replace the flat per-line `string` with a
`[]segment` sequence; caret becomes `(line, segment, offset)`) and *permits* Approach B
("marker rune + side store … acceptable … note the choice in a comment"). This design
adopts **B**. The choice and its rationale are recorded in a comment in
`widget_multiline_input.go`.

The core mechanism of B: a chip is encoded as **exactly one rune** inside the existing
line string — a unique code point from Supplementary Private-Use Area-A
(`U+F0000…U+FFFFD`, allocated per chip) — with a side map `chips map[rune]string`
holding that chip's verbatim original text. Because a chip is one rune, the caret /
Backspace / Delete / selection atomicity is **free**: the existing rune-offset machinery
already steps ±1 rune (jumps the chip), removes one rune (removes the whole chip), and
slices rune ranges (selects it whole). All ingest paths strip any pre-existing SPUA-A
rune so user content can never collide with the marker.

### Honest A-vs-B trade-off (corrected from the prior revision)

The prior revision justified B with the claim *"gogent uses `GetText()`."* **That claim
was false and is retracted.** Verified facts in `github.com/hobbestherat/gogent`:

- `ui/tui/mention_completer.go` reads `line := []rune(in.Lines[in.CursorY])` (L78,
  L270), indexes by `in.CursorX`, and **writes back** `in.Lines[in.CursorY] =
  string(newLine)` and `in.CursorX = …` on accept (L289–290).
- `ui/tui/commands_dialog.go` does `tmplInput.Lines[y] = string(line[:x]) + ph +
  string(line[x:])` and `tmplInput.CursorX = …` (L358–368), with a comment that it
  relies on `Lines`/`CursorX`/`CursorY` being public.

So gogent **reads and mutates the raw exported fields per keystroke**, not just
`GetText()`. The corrected comparison:

- **Approach A** changes the *types* of `Lines`/`CursorX`/`CursorY`. At gogent's
  coordinated `go.mod` bump (it pins `turbotui v0.3.1-0.20260626…-7db1e2f`, no
  `replace`) this surfaces as a **compile error** that forces a full rewrite of the two
  raw-field sites against the segment model — i.e. a *large* gogent migration. That is
  the opposite of "keep the gogent side a thin consumer" (orchestration note).
- **Approach B** keeps the exported field *types* unchanged, so gogent still
  **compiles** and its rune-offset arithmetic stays self-consistent: a chip sentinel in
  `Lines[y]` is just one more rune that gogent's read→edit→write-back round-trips
  intact (it never fabricates a sentinel, and preserving an existing one is automatic).
  The residual cost is a small set of **bounded, enumerated** gogent edge cases (§Seam)
  — exactly the "@-mention completer, slash-commands, history recall" list the
  orchestration note already assigns to the gogent half.

**Net:** B is chosen because it keeps gogent compiling and rune-consistent with a
*small, enumerable* downstream checklist, whereas A forces a *large* downstream rewrite.
The trade-off B accepts is honest and stated plainly: B **silently changes the
value-contract** of `Lines[y]` from "a plain string of display runes" to "may contain
one opaque chip sentinel rune," with no compiler help — so we (a) document the new
contract on the field, and (b) **export a predicate `IsPasteChipRune(r rune) bool`**
from `turbotv` so the gogent half can detect/skip sentinels cleanly instead of
hard-coding the SPUA-A range. If a maintainer decides the silent value-contract change
is unacceptable and prefers compiler-forced migration, Approach A remains the
documented alternative; B is the recommended, lower-total-churn choice.

---

## Files & functions touched (all under `turbotv/`)

### New: `turbotv/paste_chip.go` (shared model — used by both widgets)
- `const chipRuneBase = 0xF0000`, `chipRuneEnd = 0xFFFFD` — SPUA-A range; doc comment
  explains the marker scheme and collision-avoidance contract.
- **`func IsPasteChipRune(r rune) bool`** — **exported** so the gogent half can skip
  sentinels in its own token parsing without hard-coding the range (seam helper).
- `chipLabel(text string) string` → `"[pasted N lines]"`, `N = strings.Count(text,"\n")+1`
  (always ≥ 2 for a chip, so always plural; no `1 line` case).
- `chipLabelFit(text string, width int) []rune` — label truncated to `width` display
  columns with a trailing `…` on overflow (`"[pasted 12 lines…]"`), hard-cut to `width`
  when even the ellipsis cannot fit. Uses `tui.StringWidth`/`tui.RuneWidth`. The stored
  full text is never altered; only the label shrinks.
- `sanitizePaste(s string) (clean string, hasNewline bool)` — drops `\r`, drops other
  control runes `< 0x20` except `\n`, **drops any SPUA-A chip rune**, reports whether a
  `\n` survived. Shared by both widgets' `handlePaste`.
- `expandRunes(runes []rune, store map[rune]string) string` — replaces each chip marker
  in a rune slice with its stored original. Backs `GetText`, `selectionText`, copy/cut.

### `turbotv/widget_multiline_input.go` (primary)
- **Struct**: add private `chips map[rune]string`, `nextChip rune`. `Lines`, `CursorX`,
  `CursorY` unchanged in name and type. **Field doc on `Lines` updated** to state the new
  value-contract: "may contain opaque paste-chip sentinel runes; use `GetText()` for the
  verbatim text and `IsPasteChipRune` to detect them."
- `GetText()` (~L100): expand chip markers per line via `expandRunes`, then join with
  `\n`. Chip-free buffers produce byte-identical output to today. (Verified: paste
  `a\nb\nc` into `X|Y` → line `X<chip>Y` → `GetText()` = `Xa\nb\ncY`, identical to a
  literal multi-line insert.)
- `SetText(text)` (~L104): **literal multi-line, NOT chip-ifying** — `strings.Split` on
  `\n` exactly as today, and **resets `chips` to a fresh empty map** (so a buffer
  replacement cannot leak chip entries — see R3). This reverses the prior revision's
  rule. Rationale under §Chip-on-recall below; the chip-restore requirement is met by a
  separate explicit API so it cannot collapse hand-typed recalls.
- **`SetTextChip(text string)`** — **new exported method**: if `text` contains `\n`,
  build a single line holding one chip marker for the whole content; else literal. This
  is the explicit "restore a paste as a chip" entry point (satisfies issue test 7 and
  the "SetText with newlines recreates a chip" acceptance criterion via an
  intent-revealing call) **without** hijacking the plain `SetText` that gogent uses to
  recall *typed* prompts. Documented with the paste-vs-typed rationale.
- `Clear()` (~L115): unchanged behaviour (`SetText("")`), now also drops the chip store
  via the `SetText` reset.
- `NewMultiLineInput` (~L68): unchanged — literal `strings.Split` (initial content is
  not a paste). Keeps the existing `NewMultiLineInput("abc\ndef\nghi", …)` tests valid.
- `handlePaste` (~L427): `sanitizePaste`; if a `\n` survived, `deleteSelection` then
  insert **one** chip marker (storing the cleaned verbatim text); else fall back to
  today's literal per-rune insert. `GetText` after a chip paste equals a literal
  multi-line insert — collapsing is purely visual.
- **Editing/caret/selection logic — behaviourally unchanged** (chip = one rune):
  `insertRune`, `backspace` (~L444), `forwardDelete`, `newLine`, `moveLeft/Right`
  (~L476), `moveUp/Down`, `deleteSelection` (~L337), `isSelected`, `extendOrClear`,
  `selectionOrdered`. Only added work: **chip-store pruning** — `deleteSelection`,
  `backspace`, `forwardDelete` prune any markers in the removed span.
- `selectionText` (~L312): after slicing the rune range (both branches unchanged), pass
  through `expandRunes` so copy/cut yield the chip's **full** original, not the label.
  `copySelection` (~L395) then unchanged. (MLI has no `CutFn` today; cut routes through
  `deleteSelection` + the desktop copy, which now uses the expanded text.)
- **Width-aware visual layer (the real surgery) — single source of truth to avoid
  caret drift (R2):** introduce **one** primitive,
  `lineCells(lineRunes []rune, width int) []cell`, where a `cell` carries
  `{runeStart, runeLen, dispWidth}`. A normal rune → `{i, 1, 1}`; a chip marker → one
  **unbreakable** cell `{i, 1, displayWidth(chipLabelFit(text,width))}`. **Every**
  display-geometry function derives from this one projection rather than carrying its
  own parallel arithmetic: `wrappedRows`/`lineSpans`/`wordWrapSpans` (~L549–630),
  `rowsForLine`/`totalVisualRows` (~L642–673), `cursorVisualPos` (~L738),
  `cursorRowCol` (~L783), `caretSpanOffsetCol` (~L679), `CaretRowInLine` (~L698),
  `visualPosToCursorFromRows` (~L843), `draw` (~L119), `handleClick` (~L853). For a
  **chip-free line every cell is width 1**, so the projection reduces to today's
  integer arithmetic; a **cross-check test** asserts the new path equals the old
  `CursorX/width` results on chip-free input, locking the two in step (directly
  answering R2's drift concern — the layer that commit `b5a0f5b` fixed stays
  consistent because there is now one source of truth, not a fast/slow fork).
- **Click-into-chip snap (U2):** `handleClick` → `visualPosToCursorFromRows` is
  specified to **snap**: map the clicked display column to a `cell`; if it lands on a
  chip cell, return the chip's **before** rune offset when the click is in the cell's
  left half and the **after** offset otherwise — the caret can never be placed inside a
  chip. (This is the one place the old `target.start + col` formula is wrong for a
  multi-column token; it is replaced by the cell walk, not left implicit.)
- `draw` inner loop: walk a row's cells accumulating display columns; a normal rune
  paints one cell as today; a chip cell paints its `chipLabelFit` label across its
  `dispWidth` cells using `PasteChipFG/BG` (and `TextSelectionFG/BG` when the chip is
  within the selection, since a chip is one selectable unit).

### `turbotv/widget_textbox.go` (parity — same model)
- **Struct**: add `chips map[rune]string`, `nextChip rune`.
- `handlePaste` (~L323): `sanitizePaste`; `\n` survived → `deleteSelection` + one chip
  marker; else literal insert (unchanged). Multi-line paste collapses to a chip here too.
- `GetText` (~L56)/`SetText` (~L50): `GetText` expands markers; `SetText` is **literal**
  and resets the chip store (R3). A `SetTextChip` mirror provides explicit chip restore
  if a host wants it.
- `insertRune`, `backspace`/forward-delete, `moveCursor`, `deleteSelection`, `selRange`,
  `hasSelection`, `handleClick` — unchanged (chip = one rune). `handleClick` gains the
  same left/right-half snap so a click can't land inside the chip.
- `copySelection`/`cutSelection` (~L290): expand markers in the sliced range; cut prunes
  the store.
- **Word motion (U3):** `wordBoundaryLeft`/`wordBoundaryRight` (~L210, L227) gain an
  **`IsPasteChipRune` short-circuit** so a chip is a hard boundary isolated as one
  word-unit — `charClass` (~L200) otherwise buckets the sentinel as punctuation and
  would jump it together with adjacent punctuation. Ctrl+←/→ stops at / Ctrl+Backspace
  removes the chip as a whole; Ctrl+A already selects it (atomic).
- `draw` (~L60) + `cursorPos`/`ensureCursorVisible` (~L92, L334): become
  display-column aware (chip renders as its themed label; horizontal scroll/caret
  account for its width). Chip-free text keeps today's 1-col-per-rune path.

### `turbotv/theme.go`
- Add `PasteChipBG`, `PasteChipFG tui.Color` to `Theme`, documented beside
  `TextSelectionBG/FG` (drawn over the focused-input fill → must contrast with
  `InputFocusBG` and be distinct from `TextSelectionBG/FG`).
- `DefaultTheme`: `PasteChipBG = tui.ANSIColor(5)` (magenta), `PasteChipFG =
  tui.ANSIColor(15)` (bright white) — distinct from cyan `InputFocusBG(6)` and the
  black/white `TextSelection` pair.
- `HighContrastTheme`: `PasteChipBG = tui.ANSIColor(11)` (bright yellow), `PasteChipFG =
  tui.ANSIColor(0)` (black) — luminance-driven, distinct from white `InputFocusBG(15)`
  and black `TextSelectionBG(0)`. Exact codes tunable; constraint is the contrast above.

### `turbotv/desktop.go` — **no change (verified)**
`handlePaste`/`deliverPaste`/`pasteClipboard` (~L1019–1110, `:1038–1052`) only route
paste text to the focused widget via `BubblePaste`; the chip decision lives in the
widget. Left unchanged.

### `turbotv/menu.go` — **not touched** (concurrent task owns it).

---

## Chip-on-recall: resolving the typed-vs-pasted conflict (G1)

The prior revision had `SetText("a\nb") → chip`. That is **wrong** for this codebase:
gogent recalls prompt history via `sw.input.SetText(...)`
(`session_window.go:1408,1477,1492,1496`), and history stores `GetText()` — which is
plain `\n`-joined text for **both** pasted *and* hand-typed multi-line prompts (gogent
allows typed newlines via Shift+Enter). A blanket `SetText(\n)→chip` would collapse a
recalled **hand-typed** multi-line prompt into an uneditable chip — a real usability
regression and the opposite of "restore what I had."

The information needed to distinguish *pasted* from *typed* does not exist at the
`SetText` boundary (both are plain text). So this half:

- Makes plain **`SetText` literal** (no chip-ification) — recall of any multi-line
  prompt stays fully editable, matching today's behaviour. **No regression** to gogent
  recall.
- Adds explicit **`SetTextChip(text)`** for the "restore a paste as a chip" intent.
  This satisfies the issue's test 7 and the "SetText with newlines recreates a chip"
  acceptance criterion *through an intent-revealing call*, and leaves the policy of
  *when* to restore-as-chip to the host that actually knows paste-ness.
- **Hands the recall policy to the gogent half (open question O1):** if the maintainer
  wants pasted prompts to recall as chips, gogent records a paste flag (or the chip
  marker itself, which `GetText` could be made to preserve) in history and calls
  `SetTextChip` for those entries, `SetText` for typed ones. That decision lives in
  gogent and is explicitly flagged, not silently baked into the toolkit.

---

## Seam: exact gogent points the gogent half must handle (H1/H2)

Under Approach B a chip sentinel can appear in `Lines[y]`. gogent's rune-offset
arithmetic stays correct (offsets are self-consistent; read→edit→write-back preserves
the sentinel), but three gogent call sites parse the *content* of the line and need
sentinel-awareness. **None of these are turbotui changes** — they are the verification
checklist for the gogent half, made concrete here so that half is thin:

1. **`slashMatches` (`mention_completer.go:126`)** guards on `line[0] != '/'`. A chip at
   `line[0]` (multi-line paste into an empty input) makes the first rune a sentinel, so
   slash detection correctly returns false — *benign* (you are not typing a slash
   command), but the gogent half should confirm it does not panic or mis-highlight.
2. **`mentionToken` (mention-query walk-back from the cursor)** collects runes between
   `@` and the caret. A chip between `@` and the cursor would fold the sentinel into the
   query string. The gogent half should skip `IsPasteChipRune` runes (or treat a chip as
   a token boundary) so an in-progress `@mention` next to a pasted block behaves.
3. **`mention_completer.go:289` / `commands_dialog.go:367` write-back** rebuild the line
   from `[]rune` and re-assign `Lines[y]`; a sentinel present in that slice round-trips
   intact, so the chip survives an accepted completion. The gogent half should confirm a
   completion accepted on a line that also contains a chip leaves the chip intact (it
   will, by construction) and that `IsPasteChipRune` is used wherever it scans line
   content for `@`/`/`.

The exported `IsPasteChipRune` predicate exists precisely so these three become
one-line filters in the gogent half. `ui/tui/theme.go` does not yet set `PasteChip*`
(verified) — wiring those two roles is the other gogent-half task.

---

## User-facing behaviour

- **Single-line paste** (no `\n`): unchanged — literal insert, caret rune-by-rune.
- **Multi-line paste**: collapses to one highlighted `[pasted N lines]` token; the box
  does **not** grow to N lines; themed via `PasteChipBG/FG`.
- **Caret**: Left/Right jump over the chip; the caret sits only before/after, never
  inside (including a click that lands on the chip — it snaps to the nearer edge).
  Up/Down treat the chip's row as one visual row.
- **Delete**: one Backspace (caret after) or one Delete (caret before) removes the whole
  block.
- **Selection**: a drag (or Ctrl+A in `TextBox`) touching the chip includes it whole;
  typing/pasting over such a selection replaces it wholesale.
- **Copy/cut**: yields the chip's full original multi-line text, never the label.
- **Submit / `GetText()`**: verbatim original with newlines restored — submit unchanged.
- **History recall**: a recalled multi-line prompt stays **fully editable** by default
  (no surprise chip); restore-as-chip is opt-in via `SetTextChip` (gogent policy, O1).
- **Overflow**: an over-wide chip truncates its label with `…`; full text retained and
  copies/submits in full.

---

## Design criteria (the four gates)

**(1) Goal match.** Multi-line paste → one atomic `[pasted N lines]` chip; single-line
unchanged; `GetText()` verbatim; chip atomic to caret/Backspace/Delete/selection/
copy/cut; CR dropped. The "SetText recreates a chip" requirement is met via the
intent-revealing `SetTextChip` (test 7), **without** the over-reach of chip-ifying every
`\n`-bearing `SetText` (which would have mis-collapsed typed recalls — G1 resolved). No
scope creep; chip-free behaviour unchanged.

**(2) Usability.** Caret can never enter the chip — including the click-snap (U2);
one keystroke deletes it; selection/typing-over/word-motion (with the `IsPasteChipRune`
short-circuit, U3) behave atomically; the chip is visibly surfaced (not silent) and
truncates gracefully; verbatim text is never lost; **recall of typed prompts stays
editable** (G1/U1 resolved).

**(3) No regressions.** Editing/caret/selection logic and the exported
`Lines/CursorX/CursorY`, `Text/Cursor` fields keep their types and values; chip-free
paths reduce to today's arithmetic, locked by a cross-check test (R2). Two existing
tests **encode the pre-feature contract and are deliberately rewritten** to assert chip
creation — `TestMultiLineInputPasteSplitsLines` (`widget_multiline_input_test.go:79`,
pastes `"one\r\ntwo\nthree"`, asserted 3 lines) and `TestTextBoxPasteStripsNewlines`
(`widget_textbox_test.go:33`, asserts `"abcdef"`); the latter's CR-stripping intent is
preserved by asserting the chip's stored text has `\r` removed (R1 resolved — not
silently broken). Chip-store leaks are prevented: `SetText`/`Clear` reset the map,
delete paths prune it (R3 resolved). `gofmt`/`go vet`/`go build`/`golangci-lint`/
`go test ./...` are the authoritative local gate and must be clean/green.

**(4) Holistic cross-repo.** Confined to `turbotv/` (primary
`widget_multiline_input.go`, parity `widget_textbox.go`, shared `paste_chip.go`,
`theme.go` roles); `menu.go` untouched; `desktop.go` verified unchanged; no new deps.
The seam to gogent is **accurately** characterised (H1 corrected): gogent reads/mutates
`Lines[y]`/`CursorX`/`CursorY` directly; B keeps those compiling and rune-consistent;
the three content-parsing sites that need sentinel-awareness are enumerated for the
gogent half (H2), and the exported `IsPasteChipRune` predicate makes them one-line
fixes. The new theme roles are additive; gogent wires them in its own half.

---

## Regression risks & mitigations

- **Caret-drift across the visual layer (R2)** — the geometry functions that commit
  `b5a0f5b` aligned must not diverge. *Mitigation*: **one** `lineCells` projection is the
  single source of truth for all display-column math; a cross-check test asserts it
  equals the legacy `CursorX/width` arithmetic on chip-free input. No fast/slow fork to
  drift.
- **Marker-rune collision** — *Mitigation*: strip the SPUA-A range on every ingest path
  (`sanitizePaste`; typed-rune guard rejects SPUA-A); document the reserved range and
  expose `IsPasteChipRune`.
- **`Lines[y]` value-contract change is silent to gogent (H1/H3)** — *Mitigation*:
  document the new contract on the `Lines` field; export `IsPasteChipRune`; enumerate
  the three affected gogent sites (§Seam) for the gogent half; accept the trade-off
  explicitly (B over A) with the maintainer escape hatch noted.
- **gogent `@mention`/slash parsing on a line containing a chip (H2)** — bounded;
  handed to the gogent half with the precise call sites and the `IsPasteChipRune` fix.
- **Existing-test breakage (R1)** — two tests rewritten with documented intent (assert
  chip creation; preserve CR-stripping assertion); all others stay green.
- **Chip-store growth (R3)** — `SetText`/`Clear` reset the map; delete/cut paths prune.
- **Recall of typed multi-line prompts (G1)** — resolved by making `SetText` literal and
  moving chip-restore to explicit `SetTextChip`; recall policy flagged to gogent (O1).

---

## Tests (turbotv) — to add/extend

New `widget_paste_chip_test.go` plus extensions:

1. Paste single-line → literal; caret steps rune-by-rune (unchanged).
2. Paste `"a\nb\nc"` → one chip `[pasted 3 lines]`; `GetText() == "a\nb\nc"`; one logical
   line.
3. Chip at end, one Backspace → chip gone, buffer empty.
4. Chip mid-line: `←` from after → caret before chip (never inside); `→` from before →
   after; **click on the chip body → snaps to nearer edge, never inside** (U2).
5. Select-all then type → chip replaced wholesale.
6. Copy a chip → `copySelection`/clipboard returns the multi-line original.
7. `SetTextChip("x\ny")` → restored as a chip; `GetText() == "x\ny"`. **Plus**
   `SetText("x\ny")` → stays **literal** (two editable lines), guarding the G1 fix.
8. WordWrap=true: a chip is not split across visual rows.
9. `CaretRowInLine` counts the chip as one visual row.
10. `TextBox` parity: multi-line paste → chip; `GetText()` round-trips; Ctrl+←/→ treats
    the chip as one word-unit (U3); cut yields the full original.
11. **Cross-check (R2):** on chip-free input, the `lineCells`-derived geometry equals the
    legacy arithmetic for `cursorVisualPos`/`cursorRowCol`/`wrappedRows`.
12. **Sanitisation:** a paste whose text already contains a SPUA-A code point has it
    stripped (no collision); `IsPasteChipRune` identifies a real chip marker.

Rewritten with documented intent: `TestMultiLineInputPasteSplitsLines` and
`TestTextBoxPasteStripsNewlines` now assert chip creation (and, for the TextBox, that
the chip's stored text is CR-stripped). All other existing tests pass unchanged.

---

## Open questions

1. **O1 — recall policy (for the gogent half).** Should gogent restore *pasted* history
   entries as chips (record paste-ness and call `SetTextChip`), or recall everything
   literally? This half provides both `SetText` (literal) and `SetTextChip` (chip) and
   defers the policy; default if unaddressed is literal recall (no behaviour change).
2. **O2 — chip styling.** Render the literal `[pasted N lines]` text in chip colours
   (this design), or add a bracketing glyph? Width-predictable text is the default.
3. **O3 — `PasteChip*` defaults.** Magenta/white (Default) and yellow/black
   (HighContrast) are placeholders meeting the contrast constraints; maintainer may
   prefer other codes.
4. **O4 — Approach A reconsideration.** If the silent `Lines[y]` value-contract change
   (B) is deemed unacceptable versus a compiler-forced gogent migration (A), A is the
   documented alternative. B is recommended as lower total cross-repo churn.

# Design: fix wide-emoji "dirt" on scroll (hobbestherat/gogent #470)

## Summary

A line ending in a width-2 emoji such as `✅` (U+2705) leaves a stale glyph
half ("dirt") in the column to its right after the view scrolls up and the line
that scrolls into its place is empty.

**Root cause (confirmed): the width table is incomplete, not the renderer.**
`tui.RuneWidth(0x2705)` returns **1** instead of **2** because `wideRanges` in
`width.go` has no entry covering the default‑emoji‑presentation code points in
the `U+2300–U+2BFF` band (and a few supplementary ranges). Every wide-glyph
mechanism in the renderer (continuation cells, orphan clearing, the diff flush)
is already correct — it simply never engages for `✅` because the renderer is
told the glyph is one column wide.

The fix is a **self-contained data correction in `turbotui/width.go`**: extend
`wideRanges` with the missing Unicode `Emoji_Presentation=Yes` ranges. No public
API changes, no renderer changes, no new dependencies.

## Investigation / how the bug produces "dirt"

Files read: `width.go`, `cell.go`, `screen.go`, `lines.go`, `app.go`
(`WriteCell`/`place`/`clearWideAt`/`WriteString`/`Apply`), `width_test.go`,
plus the `turbotv` consumers (`measure.go`, `surface.go`, `widget_textview.go`).

`RuneWidth` (`width.go:16`) routes wide detection through `isWide` →
`wideRanges` (binary search). The table jumps from `{0x2329,0x232A}` straight to
`{0x2E80,0x303E}`, so the whole symbol/emoji band in between is treated as
width 1. Verified empirically — all of these currently report width 1:

| code point | glyph | should be | currently |
|---|---|---|---|
| U+2705 | ✅ | 2 | **1** |
| U+2B50 | ⭐ | 2 | **1** |
| U+231A | ⌚ | 2 | **1** |
| U+2714 | ✔ | 2 | **1** |
| U+1F7E0 | 🟠 | 2 | **1** |

(Existing tests pass only because they sample glyphs that *are* in the table —
`世`, `😀` U+1F600, `🚀` U+1F680 — and never probe the gap.)

### Why the symptom needs "empty line below" + "scroll up"

Trace with the bug present (`✅` modeled as width 1), `✅` at column 6, row `r`,
row `r+1` empty:

1. **Frame A.** `WriteString` lays `✅` in cell 6 only (no continuation cell,
   because width==1). Cell 7 stays a space. `Apply` emits `✅` at col 6, treats
   the cursor as advancing 1 col → its model thinks col 7 is untouched. The
   *real* terminal renders `✅` across **two physical columns (6–7)**, so the
   right half physically occupies col 7, but turbotui's front buffer records
   col 7 as a plain space.
2. **Frame B (scroll up by one).** Row `r+1`'s empty content scrolls into row
   `r`. New back buffer: row `r` is all spaces. The diff:
   - col 6: front `✅` ≠ back space → emit a space (clears only col 6).
   - col 7: front space == back space → **skipped, nothing emitted.**
   The physical right half of `✅` at col 7 is never repainted → **dirt.**

This is exactly why the two qualifiers in the report matter:
- **Empty line below**: the line that scrolls into row `r` has nothing at col 7,
  so the back buffer's col 7 stays a space and the diff finds "no change." With
  *content* below, col 7 would carry a glyph, the diff would repaint it, and the
  dirt would be coincidentally overwritten — hiding the bug.
- **Scroll up**: provides a frame transition where the wide-glyph cell's
  neighbour goes from "physically half-covered" to "should be blank" without the
  model ever recording the second column as occupied.

### Why no renderer change is needed

Once `RuneWidth(✅)==2`, the existing machinery handles everything:

- `place` (`app.go:431`) lays a base cell plus a `cont:true` continuation cell
  at col 7, and `clearWideAt` blanks orphan halves on overwrite.
- `Apply` (`app.go:680`) already syncs/【skips emission for】continuation cells
  and, crucially, on the scroll frame the col-7 cell transitions from
  `{Ch:' ', cont:true}` (front) to `{Ch:' ', cont:false}` (back). Those `Cell`
  values differ (the `cont` field participates in the struct comparison at
  `app.go:708`), so the diff **does** fire, `next.cont` is false, and a space is
  emitted at col 7 — the dirt is cleared.

This path is already exercised for `世` by
`TestApplySkipsContinuationAndIsIdempotent` and
`TestOverwritingWideGlyphClearsOrphanHalf`; it was simply never reached for
`✅`. So the renderer is correct; only the width data is wrong.

## The change

### File: `turbotui/width.go` — extend `wideRanges`

Add the Unicode `Emoji_Presentation=Yes` ranges that are missing, inserted in
sorted, non-overlapping order so the binary search invariant holds. Concretely
(BMP band first, then the supplementary gaps):

```
0x231A,0x231B      ⌚⌛
0x23E9,0x23EC      ⏩–⏬
0x23F0             ⏰
0x23F3             ⏳
0x25FD,0x25FE      ◽◾
0x2614,0x2615      ☔☕
0x2648,0x2653      zodiac
0x267F             ♿
0x2693             ⚓
0x26A1             ⚡
0x26AA,0x26AB      ⚪⚫
0x26BD,0x26BE      ⚽⚾
0x26C4,0x26C5      ⛄⛅
0x26CE             ⛎
0x26D4             ⛔
0x26EA             ⛪
0x26F2,0x26F3      ⛲⛳
0x26F5             ⛵
0x26FA             ⛺
0x26FD             ⛽
0x2705             ✅   ← the reported glyph
0x270A,0x270B      ✊✋
0x2728             ✨
0x274C             ❌
0x274E             ❎
0x2753,0x2755      ❓❔❕
0x2757             ❗
0x2795,0x2797      ➕➖➗
0x27B0             ➰
0x27BF             ➿
0x2B1B,0x2B1C      ⬛⬜
0x2B50             ⭐
0x2B55             ⭕
0x1F1E6,0x1F1FF    regional indicators
0x1F7E0,0x1F7EB    colored circles/squares
0x1F7F0            heavy equals sign
```

**Why the precise emoji-presentation set, not whole blocks.** I deliberately do
*not* widen the entire `U+2600–U+27BF` / `U+2B00–U+2BFF` blocks. Many code
points there default to **text** presentation and are width 1 in terminals —
e.g. `☢` U+2622 (radioactive), `☺` U+263A, `✓` U+2713, and the arrow glyphs.
Coarsely widening the blocks would regress those to width 2 and *introduce* the
mirror-image dirt bug for ASCII-adjacent symbols. The `Emoji_Presentation=Yes`
set is the standard, well-defined boundary (the same boundary wcwidth /
go-runewidth use) and captures `✅` and friends without touching text symbols.
This keeps us stdlib-only and dependency-free, matching the existing
hand-maintained table's intent ("most emoji").

> Minimum viable fix is the single entry `{0x2705,0x2705}`, but that would be a
> band-aid: the issue title is "wide UTF-8 characters are not handled
> correctly," and `⭐`, `⌚`, `❌`, `🟠` are the same defect one code point over.
> Completing the table is the correct, non-speculative fix and is not scope
> creep.

### No other production files change

`turbotv/measure.go`, `surface.go`, `widget_textview.go` all consume
`tui.RuneWidth`/`StringWidth` as the single source of truth, so they inherit the
fix automatically. No changes there.

## Tests to add

### `width_test.go`
- Extend `TestRuneWidth` cases: `✅`(U+2705)==2, `⭐`(U+2B50)==2, `⌚`(U+231A)==2,
  `❌`(U+274C)==2, plus a guard that a *text-presentation* symbol stays width 1
  (`☢` U+2622==1, `✓` U+2713==1) to lock in the precise boundary and catch
  accidental over-widening. Keep an existing CJK case (`世`==2) and combining
  (==0)/ASCII(==1) cases.
- `TestStringWidth`: add `"hello ✅"`==8.

### Renderer/diff regression test (the actual repro)
A focused test that reproduces frame A → scroll → frame B and asserts no stale
glyph remains in the continuation column:

1. `NewWithSize(w, h, &bytes.Buffer{})`; `Clear`; write `"x ✅"` on row `r`
   with row `r+1` non-empty; `Apply`.
2. Assert the back grid models `✅` as base + `cont:true` neighbour (width-2
   accounting), mirroring `TestWriteStringWideGlyphAdvancesTwoColumns` but for
   `✅`.
3. Rewrite the frame so row `r` is now empty (the scrolled-in empty line);
   `Apply`; assert (a) the emitted bytes for that frame blank **both** columns
   of the old `✅`, and (b) `ReadCell`/front grid show plain spaces (no residual
   `cont` cell, no residual glyph) at both columns.
4. Add a CJK twin of the same scenario with `世` to prove the assertion isn't
   emoji-specific.

These live in `width_test.go` (package `tui`), reusing the existing
buffer-backed `App` test pattern.

## Design criteria

**(1) Goal match.** The change fixes exactly the reported defect — wide
emoji ending a line no longer leave dirt on scroll-with-empty-line-below —
by correcting the one wrong input (`RuneWidth(✅)`) that disables the
already-correct wide-glyph renderer. It is a bug fix, not a feature or
refactor. Completing the emoji-presentation ranges (vs. hard-coding only
`0x2705`) fixes the same class without speculative scope: no text-presentation
symbols are touched.

**(2) Usability.** This is a rendering-correctness fix with no interactive
surface, so "dialog share / user-driven input" is N/A. The user-visible
behavior change is purely corrective: wide emoji now occupy and relinquish two
columns exactly as the terminal draws them; cursor placement, truncation and
wrapping math (already routed through `RuneWidth`) now agree with the terminal.
Nothing is silently swallowed — the previously-hidden second column is now
modeled and repainted.

**(3) No regressions.** ASCII/Latin/box-drawing widths are unchanged (still hit
the width-1 fallback). CJK and the already-covered emoji blocks are unchanged.
The only behavioral delta is for code points in the newly added ranges, all of
which were *wrong* before. The precise-set approach specifically avoids
regressing text-presentation symbols (`☢`, `✓`, arrows) that whole-block
widening would have broken — and a new test pins that boundary. All existing
tests continue to pass (they sample only already-correct glyphs). Verification:
`go build ./...`, `go vet ./...`, `go test ./...` (root + `turbotv`).

**(4) Holistic / cross-repo seam.** The fix lives entirely in turbotui, the
correct repo: `RuneWidth`/`StringWidth` is turbotui's self-contained width
authority and gogent/turbotv measure against it (`turbotv/measure.go:12-13`
explicitly documents this dependency). Fixing it in turbotui keeps gogent's
predicted layout in step with what turbotui actually renders — fixing it in
gogent would only re-derive width and drift from the renderer. The public API
(`RuneWidth`, `StringWidth` signatures) is unchanged, so the downstream gogent
update after merge is just:
`go get github.com/hobbestherat/turbotui@<sha> && go mod tidy`, plus updating
any version pin-guard test. No gogent code change is required.

## Regression risks (and mitigations)

- **Over-widening text-presentation symbols.** Mitigated by using the precise
  `Emoji_Presentation=Yes` ranges and adding an explicit "stays width 1" test
  for `☢`/`✓`.
- **Sorted/non-overlap invariant of `wideRanges`.** New entries must be inserted
  in order and not overlap neighbours, or `isWide`'s binary search breaks.
  Mitigation: insert in the documented sorted position; the expanded
  `TestRuneWidth` matrix (covering low/high ends of new ranges) catches a
  mis-sort. (Optional belt-and-suspenders: a test asserting `wideRanges` is
  sorted and disjoint.)
- **Right-edge wide glyph.** Already handled by `place` (`app.go:436`) and
  `TestWideGlyphAtRightEdgeRendersBlank`; now that more glyphs are width 2,
  that path simply applies to them too — no new logic.

## Open questions

1. **Breadth.** I recommend the full `Emoji_Presentation=Yes` set. If the
   maintainer prefers the most conservative possible change, the alternative is
   `{0x2705,0x2705}` alone — but that leaves `⭐`, `⌚`, `❌`, `🟠` broken. Confirm
   the broader (correct) scope is acceptable.
2. **ZWJ / multi-rune emoji sequences** (e.g. 👍🏽, family emoji). These render
   width 2 in most terminals but turbotui measures per-rune and has no grapheme
   clustering; this is a pre-existing limitation, out of scope for #470, and not
   the reported symptom. Flagging only — no change proposed unless requested.
3. **Variation selector U+FE0F.** It is already treated as zero-width
   (`isZeroWidth`), so `2705 FE0F` measures 2 (base) + 0 = 2, which is correct.
   No change needed; noted for completeness.

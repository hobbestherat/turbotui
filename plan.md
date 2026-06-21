# Plan: Drag-to-select + copy-selection in TextView

Port the drag-to-select / copy-selection model that already exists on
`MultiLineInput` into the read-only `TextView` (turbotv/widget_textview.go).

## Selection model

TextView renders a flattened list of **visual rows** (`[]renderRow` from
`layoutRows`, memoised by content version + width + wrap). The natural,
scroll-stable coordinate is therefore **(visual-row index, column)** in display
space — scrolling changes only `scrollY`, never the row slice, so anchor/active
row indices survive scrolling (the required test). This mirrors
MultiLineInput's `(line, col)` model, but uses the visual row instead of the
logical line because TextView has no cursor and already works row-wise in draw.

Fields added to `TextView`:
- `selAnchorRow, selAnchorCol int` — anchor; `selAnchorRow == -1` ⇒ no selection.
- `selActiveRow, selActiveCol int` — moving end of the selection.
- `selecting bool` — a press is in progress (drag may extend the selection).
- `pressRow, pressCol int` — where the button went down; the anchor is only
  committed once the pointer actually drags away (a plain click selects nothing).

Helpers (named/behaving like MultiLineInput's):
- `hasSelection()` — anchor set and anchor != active.
- `selectionOrdered()` — normalised (r0,c0,r1,c1).
- `isSelected(row,col)` — per-visual-row membership test, used by draw.
- `selectionText()` — reconstructs the selected text from the row slices: rows
  of the **same entry** (wrapped continuations) join with no separator; rows of
  **different entries** (true logical lines) join with `\n`. Copy therefore
  matches what is highlighted on screen.
- `clearSelection()` — resets anchor to -1; called from SetText/Clear/fold toggle.

Column convention: one rune == one column (same simplification MultiLineInput
already makes); exact for the ASCII content the tests use.

## Mouse handling (`handleClick`)

Extend the existing handler, preserving scrollbar thumb/arrow and fold-marker
behaviour:
- release (`!Down`): end thumb drag **and** end selecting.
- press on a fold marker (only when not already dragging): toggle as today.
- press elsewhere inside the view: record `pressRow/pressCol`, clear selection,
  set `selecting` — do **not** anchor yet.
- drag (subsequent `Down` while `selecting`): on first move off the press point,
  anchor at the press point, then extend active to the pointer.

## Draw

After the existing base render of each row (plain `WriteString` path *and*
styled `drawStyledRow` path are both covered), overlay selection: for each
column up to the row's text limit, if `isSelected`, `SetCell` the same rune (or
a blank for the tail past end-of-text) with `activeTheme.SelectionFG/BG` — the
same colours MultiLineInput uses. The blank-tail fill makes a multi-row
selection run to the right edge, matching MultiLineInput.

## CopyFn

New `copySelection` becomes the `CopyFn`: returns `selectionText()` when a
selection exists, else falls back to `AllText()` (today's whole-content copy).
`copyAll` is **kept unchanged** (an existing styled test calls it directly).

## Constraints / preserved behaviour

- Foldable entries: selection walks only the visible rows, so collapsed children
  are naturally excluded; fold toggle clears the selection (rows shift).
- ScrollToTop/ScrollToBottom/follow/scrollbar/PageUp-Down: untouched.
- No new deps; gofmt; package style matches MultiLineInput.

## gogent side (separate repo)

Dep bump only — no API change needed; CopyFn now returns the drag-selected
substring automatically. Add a test verifying a transcript TextView copies a
selected range. (Out of scope for this turbotui repo; tests here are written by
the GLM partner.)

## Tests the partner targets (turbotv package, NO -race)

- drag press→drag→release selects a range; `copySelection` returns exactly it.
- no selection ⇒ `copySelection` returns full `AllText()` (unchanged).
- selection survives a scroll (scrollBy / handleScroll) between set and copy.

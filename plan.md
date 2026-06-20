# Per-span styled text rendering + Italic cell attribute

Foundation for gogent Markdown rendering (gogent #184). Two changes, no new deps.

## 1. Italic cell attribute (`cell.go`, `app.go`)

- Add `Italic bool` to `Cell`, next to `Bold`/`Underline`.
- Add `italic bool` to `styleState`.
- `appendStyle`:
  - Full-reset branch (`!cur.valid`): emit `;3` when `cell.Italic`.
  - Differential branch: when `cur.italic != cell.Italic`, emit `3` to set / `23` to
    reset, following the existing bold (`1`/`22`) and underline (`4`/`24`) pattern.
- `app.go` flush: populate `italic: next.Italic` in the per-cell `styleState`.
- Bold/Underline behaviour untouched. SGR 3 = italic, SGR 23 = italic off.

## 2. Styled spans in TextView (`widget_textview.go`)

- New public type `StyledSpan{ Text; FG/HasFG; BG/HasBG; Bold; Underline; Italic }`.
- `TextEntry` gains `spans []StyledSpan`.
- `TextView.AddStyled(spans) *TextEntry`: sets `entry.text` = concat of span texts
  (so `AllText()`/copy stay correct) and stores `spans`. Plain `AddLine`/`AddColored`
  paths are unchanged.
- `renderRow` gains `spans []StyledSpan`.
- `computeRows`: for a styled entry, wrap **span-aware** to `avail` width:
  - flatten spans into tagged runes (`styledRune{r, span}`), expanding tabs (assigning
    inserted spaces to the tab's span, column-accurate);
  - run the same word-wrap algorithm as `wrapText` (tokenize whitespace, place words,
    hard-split over-long tokens, drop whitespace absorbed at a wrap point);
  - regroup each visual row's runes back into `[]StyledSpan` by consecutive span id.
  - indent + ▸/▾ marker handled exactly as the plain path. Wrap off ⇒ one row.
- `draw`: when `row.spans != nil`, paint cell-by-cell with `Surface.SetCell` (per
  `widget_label.go` mnemonic pattern) honouring each span's FG/BG/Bold/Underline/Italic,
  clipped to the same `limit`; else keep the existing single-fg `WriteString` path.

## Helpers (`widget_textview.go`)
- `styledRune`, `styledRunesOf`, `tokenizeStyledWhitespace`, `wrapStyledRunes`,
  `spansOfStyledRow`, `wrapStyledSpans`, `spansText`, `drawStyledRow`.

## Tester targets (GLM writes tests)
- `Cell{Italic:true}` flush emits SGR `3`.
- `AddStyled` paints each span's colour/bold/italic into the right cells.
- span-aware wrap splits spans correctly across visual rows.
- `AddStyled` entry's `AllText()` == concatenated plain text.

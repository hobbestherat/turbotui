# Plan: add (*TextEntry).AddStyled to turbotv

## Problem (gogent #245, turbotui side)
turbotv has `(*TextView).AddStyled` to create a **top-level** styled entry, but no
`(*TextEntry).AddStyled` to create a **styled child**. Because of that asymmetry,
nothing can make a styled entry that is an indented, foldable child of a header — which
is what gogent needs for "assistant answer as a foldable child of its header".

The recursive layout/draw/wrap/marker logic already handles styled entries at any depth.
Only the constructor was missing.

## Design
Add `(*TextEntry).AddStyled(spans []StyledSpan) *TextEntry`, mirroring the existing
`addChild` (which `Add`/`AddColored` use) but carrying `spans` instead of `fg/hasFG`,
exactly like the top-level `(*TextView).AddStyled` builds its entry.

Behaviour, all from existing code paths (unchanged):
- Sets `e.foldable = true` → receiver gets a ▸/▾ fold marker.
- Child gets `parent: e`, `view: e.view` → recursive draw indents it one level and
  hides/shows it with the parent's collapsed state.
- Child carries `spans` and `text: spansText(spans)` → per-span styling renders and
  AllText/copy stay correct.
- Calls `e.view.touch()` to invalidate metric cache, like every other mutator.

No changes to computeRows, draw, drawStyledRow, wrapStyledSpans, or marker/indent logic.

## Files
- `turbotv/widget_textview.go`: add the method (done).
- `turbotv/widget_textview_styled_test.go`: GLM partner adds symmetric tests mirroring
  `TestAddStyledFoldableMarkerAndIndent`:
  1. plain parent (AddColored) + `parent.AddStyled(child)`: parent shows marker, child
     indented one level, child spans keep per-span styling.
  2. styled parent + styled child: both carry spans; marker + indent + styling preserved.
  3. folding the parent hides the styled child; expanding restores it.

## Constraints
turbotv only; no new deps; gofmt clean; `go test ./...` green (no -race). PR to main.

# Plan: turbotv window-tiling primitive (gogent#241)

## Goal
Add a pure, side-effect-free tiling primitive to turbotv that gogent's View menu will
call. All new code lives in a NEW file `turbotv/tiling.go` (+ `tiling_test.go`). Do NOT
touch desktop.go, input handlers, or the redraw path.

## API (matches the issue exactly)
```go
type TileLayout int
const (
    TileRows    TileLayout = iota // stack vertically (full width each)
    TileColumns                   // side-by-side (full height each)
    TileGrid                      // near-square grid
)
func TileRects(layout TileLayout, area Rect, n int) (rects []Rect, cols, rows int)
func TileWindows(layout TileLayout, area Rect, windows []*Window) []Rect
```

## Algorithm
Core helper `splitSpan(start, length, parts)` returns `parts` (offset, size) pairs that
tile `[start, start+length)` exactly: `base = length/parts`, the first `length%parts`
parts get +1 (remainder to the FIRST rows/cols, per the issue). Each size is clamped to
>= 1 so a tiny/negative area never yields a zero/negative dimension (no panic; partition
is only exact when each cell genuinely gets >= 1 cell, which the partition tests use).

- **TileRows**: `splitSpan` over height → n full-width rows; returns (cols, rows) = (1, n).
- **TileColumns**: `splitSpan` over width → n full-height columns; returns (n, 1).
- **TileGrid**: `cols = ceil(sqrt(n))` (integer, no float), `rows = ceil(n/cols)`. Split
  height into `rows` bands; each band splits width among the items it holds. CHOICE:
  the last (partial) row stretches its items across the FULL width. So every band covers
  full width and the bands stack to full height ⇒ exact partition, no gaps/overlaps.
  Returns (cols, rows).

Edge cases: `n <= 0` → `nil, 0, 0`. `n == 1` → one rect == `area` (every layout).

## TileWindows
`rects, _, _ := TileRects(layout, area, len(windows))`, then for each non-nil window:
un-minimize exactly the way `Window.Maximize` does (clear `minimized`, restore
`Content`/`BottomBar` visibility), clear `maximized` (tiling places explicit bounds, so
it is no longer maximized), then `w.Component.SetBounds(rect)` — the same SetBounds
Maximize uses. No callbacks fired (mirrors Maximize's un-minimize path). Returns rects.

## Tests (GLM partner writes them)
`TileRects` for rows/columns/grid at n = 0,1,2,3,4,5,6,9: assert rect count, exact
partition of `area` (no gaps/overlaps, within bounds), and the returned (cols, rows).
`TileWindows` un-minimizes and sets bounds on real `*Window`s. Tiny-area no-panic.

## Constraints
turbotv only; new file + test; no desktop.go changes; no new deps; gofmt clean;
golangci-lint 0; `go test ./...` green. PR to main referencing gogent#241.

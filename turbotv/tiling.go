package tv

// Window tiling: pure geometry for laying several windows out across a work area,
// plus a thin applier that pushes those rects onto real windows. The math is
// side-effect-free and unit-testable without a desktop, mirroring how the toolkit
// already exposes geometry helpers (Rect.Inset, clampIntoView, Window.clampMove).
// gogent calls these from its View menu (gogent#241); which windows to arrange and
// the menu/command wiring stay session concerns on the gogent side.

// TileLayout selects how TileRects/TileWindows arrange n windows in an area.
type TileLayout int

const (
	// TileRows stacks windows vertically: n full-width rows, top to bottom.
	TileRows TileLayout = iota
	// TileColumns places windows side by side: n full-height columns, left to right.
	TileColumns
	// TileGrid arranges windows in a near-square grid (cols = ceil(sqrt(n))),
	// row-major; a partially filled final row stretches its cells across the full
	// width so the tiles always partition the area with no gaps.
	TileGrid
	// TileCascade overlaps the windows in a diagonal stack (Turbo Pascal style):
	// every window shares one size and is offset by a small per-window stagger from
	// the top-left, so all title bars fan out and stay visible while the front
	// window stays large. Unlike Rows/Columns/Grid the rects OVERLAP by design.
	TileCascade
)

// Cascade stagger: the per-window (dx, dy) offset, just wide/tall enough to expose
// a trailing window's left border + title text and one title-bar row, matching the
// classic Turbo Pascal cascade.
const (
	cascadeStepX = 2
	cascadeStepY = 1
)

// TileRects is pure geometry: it returns the rects that tile area for n windows
// using the given layout, along with the (cols, rows) grid it chose. It has no side
// effects.
//
// The rects partition area exactly — no gaps and no overlaps — whenever the area is
// large enough to give every cell at least one cell of width and height; the split
// is as even as possible with any remainder handed to the first rows/columns. For a
// too-small (or degenerate) area each dimension is clamped to at least 1 so callers
// never receive a zero/negative size, at the cost of overlap in that extreme case.
//
// n <= 0 returns no rects and a (0, 0) grid; n == 1 returns a single rect equal to
// area for every layout.
func TileRects(layout TileLayout, area Rect, n int) (rects []Rect, cols, rows int) {
	if n <= 0 {
		return nil, 0, 0
	}

	switch layout {
	case TileCascade:
		// Overlapping diagonal stack. Every window shares one size; window i is
		// offset by i*(stepX, stepY) from the top-left. The total fan offset is
		// capped at half the area on each axis so the shared window stays usable and,
		// combined with clamping each window's offset to that cap, every rect stays
		// fully inside area (no title bar lost off-screen) even for large n — at the
		// cost of the trailing few overlapping once the cap is reached.
		cols, rows = 1, n
		offX := capOffset((n-1)*cascadeStepX, area.W/2)
		offY := capOffset((n-1)*cascadeStepY, area.H/2)
		w := atLeastOne(area.W - offX)
		h := atLeastOne(area.H - offY)
		rects = make([]Rect, n)
		for i := 0; i < n; i++ {
			dx := capOffset(i*cascadeStepX, offX)
			dy := capOffset(i*cascadeStepY, offY)
			rects[i] = Rect{X: area.X + dx, Y: area.Y + dy, W: w, H: h}
		}

	case TileColumns:
		cols, rows = n, 1
		spans := splitSpan(area.X, area.W, n)
		rects = make([]Rect, n)
		for i, s := range spans {
			rects[i] = Rect{X: s.offset, Y: area.Y, W: s.size, H: atLeastOne(area.H)}
		}

	case TileGrid:
		cols = intCeilSqrt(n)
		rows = (n + cols - 1) / cols
		rowSpans := splitSpan(area.Y, area.H, rows)
		rects = make([]Rect, 0, n)
		for r := 0; r < rows; r++ {
			items := cols
			if r == rows-1 {
				items = n - r*cols // the final row may hold fewer cells
			}
			colSpans := splitSpan(area.X, area.W, items)
			for c := 0; c < items; c++ {
				rects = append(rects, Rect{
					X: colSpans[c].offset,
					Y: rowSpans[r].offset,
					W: colSpans[c].size,
					H: rowSpans[r].size,
				})
			}
		}

	default: // TileRows
		cols, rows = 1, n
		spans := splitSpan(area.Y, area.H, n)
		rects = make([]Rect, n)
		for i, s := range spans {
			rects[i] = Rect{X: area.X, Y: s.offset, W: atLeastOne(area.W), H: s.size}
		}
	}

	return rects, cols, rows
}

// TileWindows applies TileRects to windows: it un-minimizes each window the same way
// Window.Maximize does (so a collapsed window participates), clears its maximized
// state (the window is now at explicit tiled bounds, not filling the area), and calls
// SetBounds with its rect. nil entries are skipped but still consume their slot, so
// the surviving windows keep their position in the layout. It returns the rects
// applied (one per entry in windows, in order).
//
// A tiled window ends up neither minimized nor maximized — the same end state as
// Restore — so, like Restore, it fires OnMinimize(w, false) / OnMaximize(w, false)
// for any window that was actually leaving that state. This keeps listeners (a
// "maximized" indicator, a minimized-windows list) in sync after a View→Tile rather
// than going stale. A window that was already in normal state fires nothing.
func TileWindows(layout TileLayout, area Rect, windows []*Window) []Rect {
	rects, _, _ := TileRects(layout, area, len(windows))
	for i, w := range windows {
		if w == nil {
			continue
		}
		wasMinimized := w.minimized
		wasMaximized := w.maximized
		if wasMinimized {
			// Un-collapse exactly as Maximize does: drop the minimized flag and bring
			// the content/bottom bar back before resizing.
			w.minimized = false
			w.Content.Visible = true
			w.BottomBar.Visible = w.bottomWasVisible
		}
		w.maximized = false
		w.Component.SetBounds(rects[i])
		// Notify listeners of the state transitions, after SetBounds, matching the
		// order and the "false" edge Restore uses.
		if wasMinimized && w.OnMinimize != nil {
			w.OnMinimize(w, false)
		}
		if wasMaximized && w.OnMaximize != nil {
			w.OnMaximize(w, false)
		}
	}
	return rects
}

// tileSpan is one cell along a single axis: its start offset and length.
type tileSpan struct {
	offset int
	size   int
}

// splitSpan divides [start, start+length) into parts contiguous spans. Sizes are as
// even as possible with any remainder given to the first spans, so the spans tile the
// range exactly when length >= parts. Each size is clamped to at least 1 so a length
// smaller than parts (or a non-positive length) never produces a zero/negative span.
func splitSpan(start, length, parts int) []tileSpan {
	if parts <= 0 {
		return nil
	}
	base := length / parts
	rem := length % parts
	if rem < 0 {
		rem = 0 // a negative length carries no distributable remainder
	}
	spans := make([]tileSpan, parts)
	pos := start
	for i := 0; i < parts; i++ {
		size := base
		if i < rem {
			size++
		}
		size = atLeastOne(size)
		spans[i] = tileSpan{offset: pos, size: size}
		pos += size
	}
	return spans
}

// intCeilSqrt returns ceil(sqrt(n)) for n >= 0 using integer arithmetic, avoiding any
// floating-point rounding surprises at the perfect-square boundaries.
func intCeilSqrt(n int) int {
	c := 0
	for c*c < n {
		c++
	}
	return c
}

// atLeastOne clamps v up to 1, keeping tiled dimensions strictly positive.
func atLeastOne(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// capOffset clamps a cascade offset into [0, max], so a negative count yields 0 and
// the fan never advances past its cap (keeping every window inside the work area).
func capOffset(v, max int) int {
	if max < 0 {
		max = 0
	}
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

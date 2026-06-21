package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for the pure window-tiling primitive (tiling.go). They live in package tv
// so they can also assert the unexported minimize/maximize fields TileWindows
// touches, and exercise the splitSpan/intCeilSqrt helpers directly.

// --- geometry helpers ------------------------------------------------------

// assertDimsPositive fails if any rect has a zero or negative width/height: the
// tiler must never hand back a collapsed rect, even for a degenerate area.
func assertDimsPositive(t *testing.T, rects []Rect) {
	t.Helper()
	for i, r := range rects {
		if r.W < 1 || r.H < 1 {
			t.Errorf("rect %d %+v has non-positive dimensions", i, r)
		}
	}
}

// assertRectsWithin fails if any rect escapes area (origin or far edge).
func assertRectsWithin(t *testing.T, area Rect, rects []Rect) {
	t.Helper()
	for i, r := range rects {
		if r.X < area.X || r.Y < area.Y || r.X+r.W > area.X+area.W || r.Y+r.H > area.Y+area.H {
			t.Errorf("rect %d %+v escapes area %+v", i, r, area)
		}
	}
}

// assertPartition asserts the rects exactly tile area: every rect is inside area,
// no two rects overlap, and their combined area equals area's area (no gaps). This
// is only meaningful when area is large enough to give each cell >=1 in both dims.
func assertPartition(t *testing.T, area Rect, rects []Rect) {
	t.Helper()
	assertRectsWithin(t, area, rects)
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if inter := rects[i].Intersect(rects[j]); !inter.Empty() {
				t.Errorf("rects %d %+v and %d %+v overlap on %+v", i, rects[i], j, rects[j], inter)
			}
		}
	}
	total := 0
	for _, r := range rects {
		total += r.W * r.H
	}
	if total != area.W*area.H {
		t.Errorf("rects cover %d cells, area is %d cells (gaps remain)", total, area.W*area.H)
	}
}

// --- TileRects: count, grid dims, partition --------------------------------

// For every supported n the row layout yields n full-width rects that partition
// the area exactly, with grid dims (1, n).
func TestTileRectsRowsPartitionAndDims(t *testing.T) {
	area := Rect{X: 2, Y: 3, W: 40, H: 30}
	for _, n := range []int{1, 2, 3, 4, 5, 6, 9} {
		rects, cols, rows := TileRects(TileRows, area, n)
		if len(rects) != n {
			t.Fatalf("n=%d: got %d rects, want %d", n, len(rects), n)
		}
		if cols != 1 || rows != n {
			t.Fatalf("n=%d: grid (%d,%d), want (1,%d)", n, cols, rows, n)
		}
		assertPartition(t, area, rects)
		// Every row is full-width and rooted at area.X.
		for i, r := range rects {
			if r.X != area.X || r.W != area.W {
				t.Errorf("n=%d row %d %+v is not full-width at area.X", n, i, r)
			}
		}
	}
}

// Symmetric to rows: columns yield n full-height rects partitioning the area.
func TestTileRectsColumnsPartitionAndDims(t *testing.T) {
	area := Rect{X: 2, Y: 3, W: 30, H: 40}
	for _, n := range []int{1, 2, 3, 4, 5, 6, 9} {
		rects, cols, rows := TileRects(TileColumns, area, n)
		if len(rects) != n {
			t.Fatalf("n=%d: got %d rects, want %d", n, len(rects), n)
		}
		if cols != n || rows != 1 {
			t.Fatalf("n=%d: grid (%d,%d), want (%d,1)", n, cols, rows, n)
		}
		assertPartition(t, area, rects)
		for i, r := range rects {
			if r.Y != area.Y || r.H != area.H {
				t.Errorf("n=%d col %d %+v is not full-height at area.Y", n, i, r)
			}
		}
	}
}

// The grid picks cols=ceil(sqrt(n)), rows=ceil(n/cols), partitions the area, and
// returns exactly n rects for each n.
func TestTileRectsGridDimsPartitionCount(t *testing.T) {
	area := Rect{X: 5, Y: 6, W: 30, H: 30}
	want := map[int][2]int{
		1: {1, 1}, 2: {2, 1}, 3: {2, 2}, 4: {2, 2},
		5: {3, 2}, 6: {3, 2}, 7: {3, 3}, 8: {3, 3}, 9: {3, 3},
	}
	for n, cr := range want {
		rects, cols, rows := TileRects(TileGrid, area, n)
		if len(rects) != n {
			t.Fatalf("n=%d: got %d rects, want %d", n, len(rects), n)
		}
		if cols != cr[0] || rows != cr[1] {
			t.Fatalf("n=%d: grid (%d,%d), want (%d,%d)", n, cols, rows, cr[0], cr[1])
		}
		assertPartition(t, area, rects)
	}
}

// A non-square area still partitions exactly under the grid layout: the row
// split (of H) and the per-row column split (of W) are independent, so a wide,
// short area tiles with no gaps as long as H >= rows.
func TestTileRectsGridNonSquareAreaPartitions(t *testing.T) {
	area := Rect{X: 1, Y: 1, W: 50, H: 7} // wide and short
	for _, n := range []int{3, 5, 7} {
		rects, cols, rows := TileRects(TileGrid, area, n)
		if len(rects) != n {
			t.Fatalf("n=%d: got %d rects, want %d", n, len(rects), n)
		}
		if rows > area.H {
			t.Fatalf("n=%d: rows %d > area.H %d", n, rows, area.H)
		}
		assertPartition(t, area, rects)
		_ = cols
	}
}

// A partially filled final grid row stretches its cells across the full area
// width, so the tiles always partition with no gap on the last row.
func TestTileRectsGridLastRowStretchesFullWidth(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 12, H: 12}
	for _, n := range []int{3, 5} { // 3 -> last row 1 cell; 5 -> last row 2 cells
		rects, cols, rows := TileRects(TileGrid, area, n)
		lastRowCount := n - (rows-1)*cols
		if lastRowCount >= cols {
			t.Fatalf("n=%d: expected a partially-filled last row, got lastRowCount=%d cols=%d", n, lastRowCount, cols)
		}
		// Sum the widths of the final-row cells: they must cover the whole width.
		widthSum := 0
		lastY := rects[len(rects)-1].Y
		for _, r := range rects {
			if r.Y == lastY {
				widthSum += r.W
			}
		}
		if widthSum != area.W {
			t.Errorf("n=%d: last row widths sum to %d, want full width %d", n, widthSum, area.W)
		}
	}
}

// --- TileRects: exact layouts (locks even-split + remainder-to-front) ------

// TileRows splits the height evenly with the remainder handed to the first rows.
func TestTileRectsRowsExactHeights(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 20, H: 10}
	cases := []struct {
		n        int
		expected []Rect
	}{
		{3, []Rect{{0, 0, 20, 4}, {0, 4, 20, 3}, {0, 7, 20, 3}}},                // 10/3 -> [4,3,3]
		{4, []Rect{{0, 0, 20, 3}, {0, 3, 20, 3}, {0, 6, 20, 2}, {0, 8, 20, 2}}}, // 10/4 -> [3,3,2,2]
	}
	for _, tc := range cases {
		rects, cols, rows := TileRects(TileRows, area, tc.n)
		if cols != 1 || rows != tc.n {
			t.Fatalf("n=%d: grid (%d,%d), want (1,%d)", tc.n, cols, rows, tc.n)
		}
		if !rectsEqual(rects, tc.expected) {
			t.Fatalf("n=%d: got %+v, want %+v", tc.n, rects, tc.expected)
		}
	}
}

// TileColumns splits the width evenly with the remainder handed to the first cols.
func TestTileRectsColumnsExactWidths(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 10, H: 20}
	rects, cols, rows := TileRects(TileColumns, area, 3)
	want := []Rect{{0, 0, 4, 20}, {4, 0, 3, 20}, {7, 0, 3, 20}} // 10/3 -> [4,3,3]
	if cols != 3 || rows != 1 {
		t.Fatalf("grid (%d,%d), want (3,1)", cols, rows)
	}
	if !rectsEqual(rects, want) {
		t.Fatalf("got %+v, want %+v", rects, want)
	}
}

// The grid lays cells out row-major; the partially filled final row stretches.
func TestTileRectsGridExactLayouts(t *testing.T) {
	cases := []struct {
		name     string
		area     Rect
		n        int
		expected []Rect
	}{
		{
			name: "n=2 one row two cols",
			area: Rect{X: 0, Y: 0, W: 10, H: 10},
			n:    2,
			expected: []Rect{
				{0, 0, 5, 10}, {5, 0, 5, 10},
			},
		},
		{
			name: "n=3 last row stretches",
			area: Rect{X: 0, Y: 0, W: 10, H: 10},
			n:    3,
			expected: []Rect{
				{0, 0, 5, 5}, {5, 0, 5, 5},
				{0, 5, 10, 5},
			},
		},
		{
			name: "n=4 uniform 2x2",
			area: Rect{X: 0, Y: 0, W: 8, H: 6},
			n:    4,
			expected: []Rect{
				{0, 0, 4, 3}, {4, 0, 4, 3},
				{0, 3, 4, 3}, {4, 3, 4, 3},
			},
		},
		{
			name: "n=5 last row two wide",
			area: Rect{X: 0, Y: 0, W: 9, H: 9},
			n:    5,
			expected: []Rect{
				{0, 0, 3, 5}, {3, 0, 3, 5}, {6, 0, 3, 5},
				{0, 5, 5, 4}, {5, 5, 4, 4},
			},
		},
		{
			name: "n=9 uniform 3x3",
			area: Rect{X: 0, Y: 0, W: 9, H: 9},
			n:    9,
			expected: []Rect{
				{0, 0, 3, 3}, {3, 0, 3, 3}, {6, 0, 3, 3},
				{0, 3, 3, 3}, {3, 3, 3, 3}, {6, 3, 3, 3},
				{0, 6, 3, 3}, {3, 6, 3, 3}, {6, 6, 3, 3},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rects, _, _ := TileRects(TileGrid, tc.area, tc.n)
			if !rectsEqual(rects, tc.expected) {
				t.Fatalf("got\n%+v\nwant\n%+v", rects, tc.expected)
			}
		})
	}
}

func rectsEqual(a, b []Rect) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- TileRects: boundaries -------------------------------------------------

// n <= 0 returns no rects and a (0,0) grid for every layout (no panic).
func TestTileRectsZeroAndNegativeN(t *testing.T) {
	for _, layout := range []TileLayout{TileRows, TileColumns, TileGrid} {
		for _, n := range []int{0, -1, -5} {
			rects, cols, rows := TileRects(layout, Rect{X: 0, Y: 0, W: 20, H: 20}, n)
			if rects != nil {
				t.Errorf("layout=%v n=%d: rects=%v, want nil", layout, n, rects)
			}
			if cols != 0 || rows != 0 {
				t.Errorf("layout=%v n=%d: grid (%d,%d), want (0,0)", layout, n, cols, rows)
			}
		}
	}
}

// n == 1 yields a single rect equal to the whole area for every layout.
func TestTileRectsSingleWindowIsWholeArea(t *testing.T) {
	area := Rect{X: 4, Y: 5, W: 18, H: 12}
	for _, layout := range []TileLayout{TileRows, TileColumns, TileGrid} {
		rects, cols, rows := TileRects(layout, area, 1)
		if len(rects) != 1 {
			t.Fatalf("layout=%v: got %d rects, want 1", layout, len(rects))
		}
		if rects[0] != area {
			t.Errorf("layout=%v: rect %+v, want whole area %+v", layout, rects[0], area)
		}
		if cols != 1 || rows != 1 {
			t.Errorf("layout=%v: grid (%d,%d), want (1,1)", layout, cols, rows)
		}
	}
}

// An unknown TileLayout value falls back to the rows layout rather than panicking
// or returning nothing (the switch default is TileRows).
func TestTileRectsUnknownLayoutFallsBackToRows(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 20, H: 12}
	unknown := mustTileRects(t, TileLayout(99), area, 3)
	rows := mustTileRects(t, TileRows, area, 3)
	if !rectsEqual(unknown, rows) {
		t.Errorf("unknown layout %+v != rows %+v", unknown, rows)
	}
}

// A pure function is deterministic: the same inputs yield the same rects.
func TestTileRectsDeterministic(t *testing.T) {
	area := Rect{X: 1, Y: 2, W: 33, H: 27}
	for _, layout := range []TileLayout{TileRows, TileColumns, TileGrid} {
		a := mustTileRects(t, layout, area, 7)
		b := mustTileRects(t, layout, area, 7)
		if !rectsEqual(a, b) {
			t.Errorf("layout=%v: non-deterministic output", layout)
		}
	}
}

// A large n still partitions a large area cleanly.
func TestTileRectsLargeN(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 100, H: 100}
	rects, cols, rows := TileRects(TileGrid, area, 100)
	if len(rects) != 100 {
		t.Fatalf("got %d rects, want 100", len(rects))
	}
	if cols != 10 || rows != 10 {
		t.Fatalf("grid (%d,%d), want (10,10)", cols, rows)
	}
	assertPartition(t, area, rects)
}

// --- TileRects: degenerate areas must not panic ----------------------------

// A tiny / zero / negative area must not panic; rects keep >=1 dims. (They may
// overlap or escape the area in this extreme — the >=1 clamp trades exactness
// for safety. We assert only the safety invariants here.)
func TestTileRectsDegenerateAreaNoPanic(t *testing.T) {
	areas := []Rect{
		{X: 0, Y: 0, W: 1, H: 1},
		{X: 0, Y: 0, W: 0, H: 0},
		{X: 0, Y: 0, W: -3, H: -4},
		{X: 2, Y: 2, W: 1, H: 5},
	}
	for _, layout := range []TileLayout{TileRows, TileColumns, TileGrid} {
		for _, area := range areas {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("layout=%v area=%+v n=5 panicked: %v", layout, area, r)
					}
				}()
				rects, _, _ := TileRects(layout, area, 5)
				assertDimsPositive(t, rects)
				if len(rects) != 5 {
					t.Errorf("layout=%v area=%+v: got %d rects, want 5", layout, area, len(rects))
				}
			}()
		}
	}
}

// --- internal helpers ------------------------------------------------------

// intCeilSqrt matches ceil(sqrt(n)) at and around the perfect-square boundaries.
func TestIntCeilSqrt(t *testing.T) {
	cases := map[int]int{
		0: 0, 1: 1, 2: 2, 3: 2, 4: 2,
		5: 3, 8: 3, 9: 3, 10: 4, 15: 4, 16: 4, 17: 5,
	}
	for n, want := range cases {
		if got := intCeilSqrt(n); got != want {
			t.Errorf("intCeilSqrt(%d) = %d, want %d", n, got, want)
		}
	}
}

// splitSpan partitions length into parts contiguous spans with the remainder on
// the first spans; sizes clamp to >=1 so a short length never yields zeros.
func TestSplitSpanEvenSplitAndClamp(t *testing.T) {
	// 10 into 3 -> [4,3,3], contiguous.
	spans := splitSpan(5, 10, 3)
	want := []tileSpan{{5, 4}, {9, 3}, {12, 3}}
	if len(spans) != len(want) {
		t.Fatalf("got %+v, want %+v", spans, want)
	}
	for i, s := range spans {
		if s != want[i] {
			t.Errorf("span %d = %+v, want %+v", i, s, want[i])
		}
	}
	// A length smaller than parts still yields parts spans, each >=1.
	tiny := splitSpan(0, 2, 5)
	if len(tiny) != 5 {
		t.Fatalf("got %d spans, want 5", len(tiny))
	}
	for i, s := range tiny {
		if s.size < 1 {
			t.Errorf("span %d size %d < 1 (must clamp)", i, s.size)
		}
	}
	// Non-positive parts returns nothing.
	if got := splitSpan(0, 10, 0); got != nil {
		t.Errorf("splitSpan parts=0 = %+v, want nil", got)
	}
}

// --- TileWindows: applies bounds -------------------------------------------

func mustTileRects(t *testing.T, layout TileLayout, area Rect, n int) []Rect {
	t.Helper()
	rects, _, _ := TileRects(layout, area, n)
	return rects
}

// newWindows builds n standalone real windows for TileWindows tests.
func newWindows(t *testing.T, n int) []*Window {
	t.Helper()
	ws := make([]*Window, n)
	for i := 0; i < n; i++ {
		ws[i] = NewWindow("w", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	}
	return ws
}

// TileWindows sets each window's bounds to the matching tiled rect and returns
// those rects in order.
func TestTileWindowsSetsBoundsForEachWindow(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 40, H: 30}
	for _, layout := range []TileLayout{TileRows, TileColumns, TileGrid} {
		ws := newWindows(t, 4)
		applied := TileWindows(layout, area, ws)
		expected := mustTileRects(t, layout, area, 4)
		if !rectsEqual(applied, expected) {
			t.Fatalf("layout=%v: applied %+v != TileRects %+v", layout, applied, expected)
		}
		for i, w := range ws {
			if w.Component.Bounds != applied[i] {
				t.Errorf("layout=%v window %d bounds %+v != applied %+v", layout, i, w.Component.Bounds, applied[i])
			}
		}
	}
}

// A minimized window is un-minimized (mirroring Window.Maximize) so it can be
// tiled: the minimized flag clears and its content/bottom-bar visibility is
// restored to what Minimize captured.
func TestTileWindowsUnminimizesMinimizedWindows(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 30, H: 20}
	ws := newWindows(t, 3)

	// Minimize window 0 with a visible bottom bar so bottomWasVisible is captured
	// as true and we can assert it is restored.
	ws[0].BottomBar.Visible = true
	ws[0].Minimize()
	if !ws[0].IsMinimized() {
		t.Fatal("precondition: window 0 should be minimized")
	}
	if ws[0].Content.Visible {
		t.Fatal("precondition: minimized window content should be hidden")
	}

	rects := TileWindows(TileRows, area, ws)

	// Window 0 was un-minimized and resized to its tile, with chrome restored.
	if ws[0].IsMinimized() {
		t.Errorf("window 0 still minimized after tiling")
	}
	if !ws[0].Content.Visible {
		t.Errorf("window 0 content not made visible again")
	}
	if !ws[0].BottomBar.Visible {
		t.Errorf("window 0 bottom bar not restored (bottomWasVisible was true)")
	}
	if ws[0].Component.Bounds != rects[0] {
		t.Errorf("window 0 bounds %+v != rect %+v", ws[0].Component.Bounds, rects[0])
	}
	// The other windows were never minimized; they just got their tiles.
	for i := 1; i < 3; i++ {
		if ws[i].IsMinimized() {
			t.Errorf("window %d unexpectedly minimized", i)
		}
		if ws[i].Component.Bounds != rects[i] {
			t.Errorf("window %d bounds %+v != rect %+v", i, ws[i].Component.Bounds, rects[i])
		}
	}
}

// TileWindows clears the maximized flag (the window is now at explicit tiled
// bounds, not filling its constraint area) and applies the tile.
func TestTileWindowsClearsMaximizedState(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 30, H: 20}
	w := NewWindow("w", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	w.Maximize()
	if !w.IsMaximized() {
		t.Fatal("precondition: window should be maximized")
	}
	rects := TileWindows(TileColumns, area, []*Window{w})
	if w.IsMaximized() {
		t.Errorf("window still maximized after tiling")
	}
	if w.Component.Bounds != rects[0] {
		t.Errorf("window bounds %+v != rect %+v", w.Component.Bounds, rects[0])
	}
}

// The un-minimize TileWindows performs must match Window.Maximize's exactly:
// same minimized/content/bottom-bar outcome. (Maximize then sets maximized=true;
// TileWindows sets it false — that difference is tested separately above.)
func TestTileWindowsUnminimizeMatchesMaximize(t *testing.T) {
	mkMinimized := func() *Window {
		w := NewWindow("w", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
		w.BottomBar.Visible = true
		w.Minimize()
		return w
	}
	maximized := mkMinimized()
	tiled := mkMinimized()

	maximized.Maximize()
	TileWindows(TileRows, Rect{X: 0, Y: 0, W: 20, H: 12}, []*Window{tiled})

	if tiled.minimized != maximized.minimized {
		t.Errorf("minimized: tiled=%v maximized=%v", tiled.minimized, maximized.minimized)
	}
	if tiled.Content.Visible != maximized.Content.Visible {
		t.Errorf("Content.Visible: tiled=%v maximized=%v", tiled.Content.Visible, maximized.Content.Visible)
	}
	if tiled.BottomBar.Visible != maximized.BottomBar.Visible {
		t.Errorf("BottomBar.Visible: tiled=%v maximized=%v", tiled.BottomBar.Visible, maximized.BottomBar.Visible)
	}
	if tiled.bottomWasVisible != maximized.bottomWasVisible {
		t.Errorf("bottomWasVisible: tiled=%v maximized=%v", tiled.bottomWasVisible, maximized.bottomWasVisible)
	}
}

// nil entries are skipped (no panic) but still consume their slot, so the
// surviving windows keep their position; the returned slice has one rect per
// input entry.
func TestTileWindowsNilEntriesConsumeSlot(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 30, H: 12}
	w0 := NewWindow("a", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	w2 := NewWindow("b", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	ws := []*Window{w0, nil, w2}

	var rects []Rect
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked on nil entry: %v", r)
			}
		}()
		rects = TileWindows(TileColumns, area, ws)
	}()

	if len(rects) != 3 {
		t.Fatalf("got %d rects, want 3 (one per slot)", len(rects))
	}
	// w0 and w2 get the slot-0 and slot-2 columns; the nil slot-1 is untouched.
	if w0.Component.Bounds != rects[0] {
		t.Errorf("w0 bounds %+v != rect[0] %+v", w0.Component.Bounds, rects[0])
	}
	if w2.Component.Bounds != rects[2] {
		t.Errorf("w2 bounds %+v != rect[2] %+v", w2.Component.Bounds, rects[2])
	}
	// The slot-0 and slot-2 columns differ (they are not the same rect).
	if rects[0] == rects[2] {
		t.Errorf("slot 0 and slot 2 rects identical: %+v", rects[0])
	}
}

// An empty window slice yields no rects and does not panic.
func TestTileWindowsEmptyNoPanic(t *testing.T) {
	for _, layout := range []TileLayout{TileRows, TileColumns, TileGrid} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("layout=%v empty slice panicked: %v", layout, r)
				}
			}()
			rects := TileWindows(layout, Rect{X: 0, Y: 0, W: 20, H: 20}, nil)
			if len(rects) != 0 {
				t.Errorf("layout=%v: got %d rects, want 0", layout, len(rects))
			}
		}()
	}
}

// A mixed batch (minimized + maximized + nil + normal) composes correctly: every
// non-nil window ends up un-minimized, de-maximized and at its tile.
func TestTileWindowsMixedStates(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 40, H: 30}
	minimized := NewWindow("min", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	minimized.Minimize()
	maximized := NewWindow("max", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	maximized.Maximize()
	plain := NewWindow("plain", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	ws := []*Window{minimized, maximized, nil, plain}

	rects := TileWindows(TileGrid, area, ws)
	if len(rects) != 4 {
		t.Fatalf("got %d rects, want 4", len(rects))
	}
	for i, w := range []*Window{minimized, maximized, plain} {
		// map back to their slots (0,1,3)
		slot := []int{0, 1, 3}[i]
		if w.IsMinimized() {
			t.Errorf("window %d still minimized", slot)
		}
		if w.IsMaximized() {
			t.Errorf("window %d still maximized", slot)
		}
		if w.Component.Bounds != rects[slot] {
			t.Errorf("window %d bounds %+v != rect %+v", slot, w.Component.Bounds, rects[slot])
		}
	}
}

// --- TileWindows: minimize/maximize callbacks ------------------------------

// cbEvent records one OnMinimize/OnMaximize invocation: the window pointer, the
// bool argument, and the window's bounds as observed *inside* the callback (so
// tests can prove SetBounds has already run).
type cbEvent struct {
	window *Window
	flag   bool
	bounds Rect
}

// cbRecorder captures OnMinimize/OnMaximize into per-channel slices.
type cbRecorder struct {
	minCalls []cbEvent
	maxCalls []cbEvent
}

func (r *cbRecorder) attach(w *Window) {
	w.OnMinimize = func(win *Window, minimized bool) {
		r.minCalls = append(r.minCalls, cbEvent{window: win, flag: minimized, bounds: win.Component.Bounds})
	}
	w.OnMaximize = func(win *Window, maximized bool) {
		r.maxCalls = append(r.maxCalls, cbEvent{window: win, flag: maximized, bounds: win.Component.Bounds})
	}
}

// A minimized window fires OnMinimize(w, false) exactly once and does not fire
// OnMaximize (a minimized window is never also maximized).
func TestTileWindowsFiresOnMinimizeForMinimizedWindow(t *testing.T) {
	w := NewWindow("min", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	w.Minimize()
	rec := &cbRecorder{}
	rec.attach(w) // attach after setup so the Minimize() call doesn't pollute counts

	rects := TileWindows(TileRows, Rect{X: 0, Y: 0, W: 30, H: 20}, []*Window{w})

	if len(rec.minCalls) != 1 {
		t.Fatalf("OnMinimize fired %d times, want 1", len(rec.minCalls))
	}
	if rec.minCalls[0].flag != false {
		t.Errorf("OnMinimize arg = %v, want false", rec.minCalls[0].flag)
	}
	if rec.minCalls[0].window != w {
		t.Errorf("OnMinimize passed window %p, want %p", rec.minCalls[0].window, w)
	}
	if rec.minCalls[0].bounds != rects[0] {
		t.Errorf("OnMinimize observed bounds %+v != tiled rect %+v", rec.minCalls[0].bounds, rects[0])
	}
	if len(rec.maxCalls) != 0 {
		t.Errorf("OnMaximize fired %d times on a minimized window, want 0", len(rec.maxCalls))
	}
}

// A maximized window fires OnMaximize(w, false) exactly once and does not fire
// OnMinimize.
func TestTileWindowsFiresOnMaximizeForMaximizedWindow(t *testing.T) {
	w := NewWindow("max", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	w.Maximize()
	rec := &cbRecorder{}
	rec.attach(w)

	rects := TileWindows(TileColumns, Rect{X: 0, Y: 0, W: 30, H: 20}, []*Window{w})

	if len(rec.maxCalls) != 1 {
		t.Fatalf("OnMaximize fired %d times, want 1", len(rec.maxCalls))
	}
	if rec.maxCalls[0].flag != false {
		t.Errorf("OnMaximize arg = %v, want false", rec.maxCalls[0].flag)
	}
	if rec.maxCalls[0].window != w {
		t.Errorf("OnMaximize passed window %p, want %p", rec.maxCalls[0].window, w)
	}
	if rec.maxCalls[0].bounds != rects[0] {
		t.Errorf("OnMaximize observed bounds %+v != tiled rect %+v", rec.maxCalls[0].bounds, rects[0])
	}
	if len(rec.minCalls) != 0 {
		t.Errorf("OnMinimize fired %d times on a maximized window, want 0", len(rec.minCalls))
	}
}

// A window already in normal state fires neither callback.
func TestTileWindowsFiresNoCallbacksForNormalWindow(t *testing.T) {
	w := NewWindow("plain", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	rec := &cbRecorder{}
	rec.attach(w)
	TileWindows(TileRows, Rect{X: 0, Y: 0, W: 30, H: 20}, []*Window{w})
	if len(rec.minCalls) != 0 || len(rec.maxCalls) != 0 {
		t.Errorf("normal window fired callbacks: min=%d max=%d, want 0/0", len(rec.minCalls), len(rec.maxCalls))
	}
}

// Callbacks fire only after SetBounds and after the un-minimize chrome restore,
// so a listener can read the post-state synchronously inside the handler.
func TestTileWindowsCallbacksFireAfterSetBoundsAndUnminimize(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 30, H: 20}
	w := NewWindow("min", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
	w.BottomBar.Visible = true
	w.Minimize()

	var boundsAtCall Rect
	var contentVisibleAtCall, bottomVisibleAtCall, minimizedAtCall bool
	w.OnMinimize = func(win *Window, _ bool) {
		boundsAtCall = win.Component.Bounds
		contentVisibleAtCall = win.Content.Visible
		bottomVisibleAtCall = win.BottomBar.Visible
		minimizedAtCall = win.IsMinimized()
	}
	rects := TileWindows(TileRows, area, []*Window{w})

	if boundsAtCall != rects[0] {
		t.Errorf("bounds inside callback %+v != tiled rect %+v (SetBounds must run first)", boundsAtCall, rects[0])
	}
	if !contentVisibleAtCall {
		t.Errorf("Content not visible inside callback (chrome restore must run first)")
	}
	if !bottomVisibleAtCall {
		t.Errorf("BottomBar not restored inside callback (bottomWasVisible was true)")
	}
	if minimizedAtCall {
		t.Errorf("window still minimized inside callback (flag must be cleared first)")
	}
}

// In a mixed batch each window fires exactly the callback for the state it was
// leaving, with its own pointer; nil and normal slots fire nothing.
func TestTileWindowsCallbacksPerWindowInBatch(t *testing.T) {
	mkMin := func() *Window {
		w := NewWindow("min", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
		w.Minimize()
		return w
	}
	mkMax := func() *Window {
		w := NewWindow("max", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
		w.Maximize()
		return w
	}
	mkPlain := func() *Window { return NewWindow("plain", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle) }

	minW, maxW, plainW := mkMin(), mkMax(), mkPlain()
	minRec, maxRec, plainRec := &cbRecorder{}, &cbRecorder{}, &cbRecorder{}
	minRec.attach(minW)
	maxRec.attach(maxW)
	plainRec.attach(plainW)

	TileWindows(TileGrid, Rect{X: 0, Y: 0, W: 40, H: 30}, []*Window{minW, nil, maxW, plainW})

	if len(minRec.minCalls) != 1 || minRec.minCalls[0].flag != false || len(minRec.maxCalls) != 0 {
		t.Errorf("minimized window: min=%v max=%d, want one min(false)/no max", minRec.minCalls, len(minRec.maxCalls))
	}
	if len(maxRec.maxCalls) != 1 || maxRec.maxCalls[0].flag != false || len(maxRec.minCalls) != 0 {
		t.Errorf("maximized window: max=%v min=%d, want one max(false)/no min", maxRec.maxCalls, len(maxRec.minCalls))
	}
	if len(plainRec.minCalls) != 0 || len(plainRec.maxCalls) != 0 {
		t.Errorf("normal window fired callbacks: min=%d max=%d", len(plainRec.minCalls), len(plainRec.maxCalls))
	}
}

// TileWindows' callback semantics match Window.Restore's exactly (one fire, with
// false) for a window leaving the minimized or maximized state.
func TestTileWindowsCallbackParityWithRestore(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 40, H: 30}

	t.Run("maximized", func(t *testing.T) {
		restored := NewWindow("r", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
		restored.Maximize()
		rr := &cbRecorder{}
		rr.attach(restored)
		restored.Restore()

		tiled := NewWindow("t", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
		tiled.Maximize()
		rt := &cbRecorder{}
		rt.attach(tiled)
		TileWindows(TileGrid, area, []*Window{tiled})

		if len(rr.maxCalls) != len(rt.maxCalls) || len(rt.maxCalls) != 1 {
			t.Fatalf("max calls: restore=%d tile=%d, want 1/1", len(rr.maxCalls), len(rt.maxCalls))
		}
		if rr.maxCalls[0].flag != rt.maxCalls[0].flag || rt.maxCalls[0].flag != false {
			t.Errorf("max flag: restore=%v tile=%v, want false/false", rr.maxCalls[0].flag, rt.maxCalls[0].flag)
		}
	})

	t.Run("minimized", func(t *testing.T) {
		restored := NewWindow("r", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
		restored.Minimize()
		rr := &cbRecorder{}
		rr.attach(restored)
		restored.Restore()

		tiled := NewWindow("t", Rect{X: 0, Y: 0, W: 8, H: 6}, tui.LineSingle)
		tiled.Minimize()
		rt := &cbRecorder{}
		rt.attach(tiled)
		TileWindows(TileGrid, area, []*Window{tiled})

		if len(rr.minCalls) != len(rt.minCalls) || len(rt.minCalls) != 1 {
			t.Fatalf("min calls: restore=%d tile=%d, want 1/1", len(rr.minCalls), len(rt.minCalls))
		}
		if rr.minCalls[0].flag != rt.minCalls[0].flag || rt.minCalls[0].flag != false {
			t.Errorf("min flag: restore=%v tile=%v, want false/false", rr.minCalls[0].flag, rt.minCalls[0].flag)
		}
	})
}

// Tiling into a degenerate area must not panic through the integrated
// SetBounds -> window.layout path (the >=1 clamp yields 1x1 rects, and layout
// guards W/H<2). State transitions and callbacks still behave.
func TestTileWindowsDegenerateAreaNoPanic(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 1, H: 1}
	ws := newWindows(t, 4)
	ws[0].Minimize()
	ws[1].Maximize()
	rec := &cbRecorder{}
	rec.attach(ws[0])
	rec.attach(ws[1])
	rec.attach(ws[2])
	rec.attach(ws[3])

	var rects []Rect
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("TileWindows into 1x1 area panicked: %v", r)
			}
		}()
		rects = TileWindows(TileGrid, area, ws)
	}()

	if len(rects) != 4 {
		t.Fatalf("got %d rects, want 4", len(rects))
	}
	for i, w := range ws {
		if w.Component.Bounds != rects[i] {
			t.Errorf("window %d bounds %+v != rect %+v", i, w.Component.Bounds, rects[i])
		}
		if w.IsMinimized() || w.IsMaximized() {
			t.Errorf("window %d still min/max", i)
		}
	}
	// Callbacks still fire into a tiny area: ws[0] was minimized, ws[1] maximized.
	if len(rec.minCalls) != 1 || len(rec.maxCalls) != 1 {
		t.Errorf("callbacks into tiny area: min=%d max=%d, want 1/1", len(rec.minCalls), len(rec.maxCalls))
	}
}

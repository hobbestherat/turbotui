package tv

import "testing"

// Tests for the TileCascade geometry (gogent#271): an overlapping diagonal stack.
// Unlike Rows/Columns/Grid the rects intentionally overlap, so these assert the
// shared size + per-window stagger + within-area clamping rather than partitioning.

func TestTileCascadeSharedSizeAndStagger(t *testing.T) {
	area := Rect{X: 3, Y: 2, W: 100, H: 40}
	n := 4
	rects, _, _ := TileRects(TileCascade, area, n)
	if len(rects) != n {
		t.Fatalf("got %d rects, want %d", len(rects), n)
	}
	// Offsets are small enough (n-1=3) that no capping kicks in: full stagger.
	offX := (n - 1) * cascadeStepX
	offY := (n - 1) * cascadeStepY
	wantW := area.W - offX
	wantH := area.H - offY
	for i, r := range rects {
		if r.W != wantW || r.H != wantH {
			t.Errorf("rect %d size = %dx%d, want %dx%d (all windows share one size)", i, r.W, r.H, wantW, wantH)
		}
		if r.X != area.X+i*cascadeStepX || r.Y != area.Y+i*cascadeStepY {
			t.Errorf("rect %d origin = (%d,%d), want (%d,%d)", i, r.X, r.Y, area.X+i*cascadeStepX, area.Y+i*cascadeStepY)
		}
	}
	assertDimsPositive(t, rects)
	assertRectsWithin(t, area, rects) // every window fully inside the work area
}

func TestTileCascadeSingleWindowFillsArea(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 80, H: 24}
	rects, _, _ := TileRects(TileCascade, area, 1)
	if len(rects) != 1 || rects[0] != area {
		t.Fatalf("single cascade window = %+v, want the whole area %+v", rects, area)
	}
}

func TestTileCascadeEmpty(t *testing.T) {
	if rects, c, r := TileRects(TileCascade, Rect{W: 10, H: 10}, 0); rects != nil || c != 0 || r != 0 {
		t.Fatalf("n=0 = (%v,%d,%d), want (nil,0,0)", rects, c, r)
	}
}

func TestTileCascadeClampsLargeNWithinArea(t *testing.T) {
	// Many windows: the fan must clamp so every rect stays inside area (no title
	// bar off-screen), even though the trailing ones then overlap at the corner.
	area := Rect{X: 5, Y: 5, W: 60, H: 30}
	rects, _, _ := TileRects(TileCascade, area, 100)
	if len(rects) != 100 {
		t.Fatalf("got %d rects, want 100", len(rects))
	}
	assertDimsPositive(t, rects)
	assertRectsWithin(t, area, rects)
	// Offsets are capped at half the area, so the deepest windows sit at that cap.
	maxX := area.X + area.W/2
	maxY := area.Y + area.H/2
	for i, r := range rects {
		if r.X > maxX || r.Y > maxY {
			t.Errorf("rect %d origin (%d,%d) exceeds the offset cap (%d,%d)", i, r.X, r.Y, maxX, maxY)
		}
	}
}

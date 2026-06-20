package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// shadowCells returns the set of (x,y) cells the shadow painted, identified by
// their foreground colour matching the shadow colour. The element is drawn as a
// solid block first so shadow cells are only those outside it.
func shadowCells(app *tui.App, shadow tui.Color, w int, h int) map[[2]int]bool {
	out := map[[2]int]bool{}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if app.ReadCell(x, y).FG == shadow {
				out[[2]int{x, y}] = true
			}
		}
	}
	return out
}

func TestDrawShadowDefaultGeometry(t *testing.T) {
	const w, h = 12, 8
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	rect := Rect{X: 2, Y: 2, W: 5, H: 3} // Right()=6, Bottom()=4
	surface.DrawShadow(rect, shadow, DefaultShadowStyle)

	got := shadowCells(app, shadow, w, h)

	// Default: 2-column right band (snug at Right()+1, Right()+2), running from
	// OffsetY (Y+1) down to Bottom()+1; 1-row bottom band at Bottom()+1 from
	// OffsetX (X+1) to Right()+2.
	want := map[[2]int]bool{}
	// Right band columns 7,8 for rows 3..5.
	for _, x := range []int{7, 8} {
		for y := 3; y <= 5; y++ {
			want[[2]int{x, y}] = true
		}
	}
	// Bottom band row 5, columns 3..8.
	for x := 3; x <= 8; x++ {
		want[[2]int{x, 5}] = true
	}

	if len(got) != len(want) {
		t.Fatalf("shadow cell count = %d, want %d (got=%v)", len(got), len(want), got)
	}
	for cell := range want {
		if !got[cell] {
			t.Errorf("expected shadow cell at %v, missing", cell)
		}
	}
	// The shadow must hug the frame: no cell two columns past the right edge gap,
	// and nothing directly above the element's top-right corner (notch is open).
	if got[[2]int{7, 2}] {
		t.Errorf("right band should not cover the top row (notch should be open)")
	}
}

func TestDrawShadowConfigurableThickness(t *testing.T) {
	const w, h = 16, 12
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	rect := Rect{X: 1, Y: 1, W: 4, H: 3} // Right()=4, Bottom()=3
	style := ShadowStyle{OffsetX: 1, OffsetY: 1, RightWidth: 3, BottomHeight: 2}
	surface.DrawShadow(rect, shadow, style)

	got := shadowCells(app, shadow, w, h)

	want := map[[2]int]bool{}
	// Right band: columns 5,6,7 (Right()+1..+3) rows 2..5 (Y+OffsetY..Bottom()+BottomHeight).
	for _, x := range []int{5, 6, 7} {
		for y := 2; y <= 5; y++ {
			want[[2]int{x, y}] = true
		}
	}
	// Bottom band: rows 4,5 (Bottom()+1..+2) columns 2..7 (X+OffsetX..Right()+RightWidth).
	for _, y := range []int{4, 5} {
		for x := 2; x <= 7; x++ {
			want[[2]int{x, y}] = true
		}
	}

	if len(got) != len(want) {
		t.Fatalf("shadow cell count = %d, want %d", len(got), len(want))
	}
	for cell := range want {
		if !got[cell] {
			t.Errorf("expected shadow cell at %v, missing", cell)
		}
	}
}

func TestDrawShadowZeroBandsOmitted(t *testing.T) {
	const w, h = 10, 6
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	rect := Rect{X: 1, Y: 1, W: 4, H: 2}
	// Only a right band, no bottom band.
	surface.DrawShadow(rect, shadow, ShadowStyle{OffsetX: 1, OffsetY: 1, RightWidth: 1, BottomHeight: 0})

	got := shadowCells(app, shadow, w, h)
	// Right band: column Right()+1 = 5, rows Y+1..Bottom()+0 => rows 2..2.
	if len(got) != 1 || !got[[2]int{5, 2}] {
		t.Fatalf("expected a single right-band cell at (5,2), got %v", got)
	}
}

func TestDrawShadowOffsetWidensNotch(t *testing.T) {
	const w, h = 16, 12
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	rect := Rect{X: 2, Y: 2, W: 5, H: 4} // Right()=6, Bottom()=5
	style := ShadowStyle{OffsetX: 2, OffsetY: 2, RightWidth: 2, BottomHeight: 1}
	surface.DrawShadow(rect, shadow, style)

	got := shadowCells(app, shadow, w, h)
	// Right band starts at Y+OffsetY = 4, so row 3 (Y+1) must be empty.
	if got[[2]int{7, 3}] {
		t.Errorf("right band should start at row 4 with OffsetY=2, found cell at (7,3)")
	}
	if !got[[2]int{7, 4}] {
		t.Errorf("right band should start at row 4 with OffsetY=2")
	}
	// Bottom band starts at X+OffsetX = 4, so column 3 (X+1) must be empty.
	if got[[2]int{3, 6}] {
		t.Errorf("bottom band should start at column 4 with OffsetX=2, found cell at (3,6)")
	}
	if !got[[2]int{4, 6}] {
		t.Errorf("bottom band should start at column 4 with OffsetX=2")
	}
}

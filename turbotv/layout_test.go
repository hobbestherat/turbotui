package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// relayout re-runs a container's LayoutFn so a test can inspect the resulting
// child bounds without going through Draw.
func relayout(c *VisualComponent) {
	c.SetBounds(c.Bounds)
}

func mkChild(w, h int) *VisualComponent {
	return NewComponent(Rect{X: 0, Y: 0, W: w, H: h})
}

func TestHBoxHomogeneous(t *testing.T) {
	box := NewHBox(Rect{X: 0, Y: 0, W: 30, H: 4})
	box.Homogeneous = true
	a, b, c := mkChild(2, 1), mkChild(2, 1), mkChild(2, 1)
	box.Add(a).Add(b).Add(c)
	relayout(box.Component)

	want := []Rect{
		{X: 0, Y: 0, W: 10, H: 4},
		{X: 10, Y: 0, W: 10, H: 4},
		{X: 20, Y: 0, W: 10, H: 4},
	}
	for i, child := range []*VisualComponent{a, b, c} {
		if child.Bounds != want[i] {
			t.Fatalf("child %d bounds = %+v; want %+v", i, child.Bounds, want[i])
		}
	}
}

func TestHBoxFlexDistributesLeftover(t *testing.T) {
	box := NewHBox(Rect{X: 0, Y: 0, W: 30, H: 1})
	fixed := mkChild(10, 1)
	growA := mkChild(0, 1)
	growB := mkChild(0, 1)
	growA.Flex = 1
	growB.Flex = 2
	box.Add(fixed).Add(growA).Add(growB)
	relayout(box.Component)

	// fixed=10, leftover=20 split 1:2 => 6 and 14 (remainder goes to last flex).
	if fixed.Bounds.X != 0 || fixed.Bounds.W != 10 {
		t.Fatalf("fixed = %+v; want X0 W10", fixed.Bounds)
	}
	if growA.Bounds.X != 10 || growA.Bounds.W != 6 {
		t.Fatalf("growA = %+v; want X10 W6", growA.Bounds)
	}
	if growB.Bounds.X != 16 || growB.Bounds.W != 14 {
		t.Fatalf("growB = %+v; want X16 W14", growB.Bounds)
	}
}

func TestHBoxNaturalPackWithSpacing(t *testing.T) {
	box := NewHBox(Rect{X: 5, Y: 7, W: 100, H: 3})
	box.Spacing = 2
	a, b := mkChild(4, 1), mkChild(6, 1)
	box.Add(a).Add(b)
	relayout(box.Component)

	// Children are box-relative (origin 0), regardless of the box's own X/Y.
	if a.Bounds != (Rect{X: 0, Y: 0, W: 4, H: 3}) {
		t.Fatalf("a = %+v", a.Bounds)
	}
	if b.Bounds != (Rect{X: 6, Y: 0, W: 6, H: 3}) {
		t.Fatalf("b = %+v; want X6 (0+4+2)", b.Bounds)
	}
}

func TestHBoxCrossAlignment(t *testing.T) {
	// Child natural cross size = 2 inside a box of cross size 6; positions are
	// relative to the box (the box's own X/Y must not shift them).
	cases := []struct {
		name  string
		align Align
		wantY int
		wantH int
	}{
		{"stretch", AlignStretch, 0, 6},
		{"start", AlignStart, 0, 2},
		{"center", AlignCenter, 2, 2}, // (6-2)/2
		{"end", AlignEnd, 4, 2},       // 6 - 2
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			box := NewHBox(Rect{X: 3, Y: 9, W: 20, H: 6})
			box.Align = tc.align
			child := mkChild(4, 2)
			box.Add(child)
			relayout(box.Component)
			if child.Bounds.Y != tc.wantY || child.Bounds.H != tc.wantH {
				t.Fatalf("cross = Y%d H%d; want Y%d H%d", child.Bounds.Y, child.Bounds.H, tc.wantY, tc.wantH)
			}
		})
	}
}

func TestVBoxPacksVertically(t *testing.T) {
	box := NewVBox(Rect{X: 0, Y: 0, W: 8, H: 20})
	box.Homogeneous = true
	a, b := mkChild(1, 1), mkChild(1, 1)
	box.Add(a).Add(b)
	relayout(box.Component)

	if a.Bounds != (Rect{X: 0, Y: 0, W: 8, H: 10}) {
		t.Fatalf("a = %+v", a.Bounds)
	}
	if b.Bounds != (Rect{X: 0, Y: 10, W: 8, H: 10}) {
		t.Fatalf("b = %+v", b.Bounds)
	}
}

func TestGridLaysOutCells(t *testing.T) {
	grid := NewGrid(Rect{X: 0, Y: 0, W: 20, H: 10}, 2, 2)
	grid.Padding = 0
	grid.Spacing = 0
	children := make([]*VisualComponent, 4)
	for i := range children {
		children[i] = mkChild(1, 1)
		grid.Add(children[i])
	}
	relayout(grid.Component)

	want := []Rect{
		{X: 0, Y: 0, W: 10, H: 5},
		{X: 10, Y: 0, W: 10, H: 5},
		{X: 0, Y: 5, W: 10, H: 5},
		{X: 10, Y: 5, W: 10, H: 5},
	}
	for i, child := range children {
		if child.Bounds != want[i] {
			t.Fatalf("cell %d = %+v; want %+v", i, child.Bounds, want[i])
		}
	}
}

func TestGridPaddingAndSpacing(t *testing.T) {
	grid := NewGrid(Rect{X: 0, Y: 0, W: 24, H: 14}, 2, 2)
	grid.Padding = 2
	grid.Spacing = 2
	// inner after padding = {2,2,20,10}; cells = (20-2)/2=9 wide, (10-2)/2=4 tall.
	a := mkChild(1, 1)
	b := mkChild(1, 1)
	grid.Add(a).Add(b)
	relayout(grid.Component)

	if a.Bounds != (Rect{X: 2, Y: 2, W: 9, H: 4}) {
		t.Fatalf("a = %+v", a.Bounds)
	}
	if b.Bounds != (Rect{X: 13, Y: 2, W: 9, H: 4}) { // 2 + 9 + 2
		t.Fatalf("b = %+v", b.Bounds)
	}
}

// TestBoxRerunsLayoutOnResize verifies the container reacts to SetBounds, which
// is how it stays correct when its parent re-layouts it (e.g. a window resize).
func TestBoxRerunsLayoutOnResize(t *testing.T) {
	box := NewHBox(Rect{X: 0, Y: 0, W: 30, H: 1})
	box.Homogeneous = true
	a, b := mkChild(1, 1), mkChild(1, 1)
	box.Add(a).Add(b)
	relayout(box.Component)
	if a.Bounds.W != 15 {
		t.Fatalf("initial width = %d; want 15", a.Bounds.W)
	}
	box.Component.SetBounds(Rect{X: 0, Y: 0, W: 60, H: 1})
	if a.Bounds.W != 30 || b.Bounds.X != 30 {
		t.Fatalf("after resize a=%+v b=%+v; want a.W30 b.X30", a.Bounds, b.Bounds)
	}
}

// TestHBoxInDialogButtons sanity-checks that HBox-laid-out children render into a
// buffer-backed app (the LayoutFn runs during Draw).
func TestHBoxRendersChildren(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(40, 3, &output)
	desktop := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 3})
	box := NewHBox(Rect{X: 0, Y: 1, W: 40, H: 1})
	box.Homogeneous = true
	mkLabel := func(text string) *VisualComponent {
		lbl := NewLabel(text, Rect{})
		return lbl.Component
	}
	box.Add(mkLabel("AA")).Add(mkLabel("BB"))
	root.AddChild(box)
	desktop.AddLayer(NewLayer("root", root, true, false))
	desktop.Redraw()

	// First label "AA" is laid out at the left cell's origin (0,1); second "BB"
	// starts at column 20 (the second homogeneous half of a 40-wide row).
	if got := app.ReadCell(0, 1).Ch; got != 'A' {
		t.Fatalf("expected first label 'A' at (0,1), got %q", got)
	}
	if got := app.ReadCell(20, 1).Ch; got != 'B' {
		t.Fatalf("expected second label 'B' at (20,1), got %q", got)
	}
}

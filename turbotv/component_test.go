package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestAbsoluteBoundsChainsThroughParents(t *testing.T) {
	root := NewComponent(Rect{X: 10, Y: 20, W: 100, H: 100})
	mid := NewComponent(Rect{X: 1, Y: 2, W: 50, H: 50})
	leaf := NewComponent(Rect{X: 3, Y: 4, W: 5, H: 6})
	root.AddChild(mid)
	mid.AddChild(leaf)

	got := leaf.AbsoluteBounds()
	want := Rect{X: 14, Y: 26, W: 5, H: 6}
	if got != want {
		t.Fatalf("leaf AbsoluteBounds = %+v; want %+v", got, want)
	}
}

// TestAbsoluteBoundsCache proves the result is memoized: a direct mutation of
// Bounds (which bypasses SetBounds) is NOT reflected until SetBounds invalidates
// the cache.
func TestAbsoluteBoundsCache(t *testing.T) {
	parent := NewComponent(Rect{X: 10, Y: 10, W: 20, H: 20})
	child := NewComponent(Rect{X: 1, Y: 1, W: 5, H: 5})
	parent.AddChild(child)

	if got := child.AbsoluteBounds(); got != (Rect{X: 11, Y: 11, W: 5, H: 5}) {
		t.Fatalf("initial abs = %+v", got)
	}
	// Bypass SetBounds: the cache must still report the old position.
	child.Bounds.X = 2
	if got := child.AbsoluteBounds(); got.X != 11 {
		t.Fatalf("cached abs should be stale (X=%d), got X=%d", 11, got.X)
	}
	// SetBounds invalidates the cache, so the new position is reflected.
	child.SetBounds(Rect{X: 2, Y: 1, W: 5, H: 5})
	if got := child.AbsoluteBounds(); got != (Rect{X: 12, Y: 11, W: 5, H: 5}) {
		t.Fatalf("post-SetBounds abs = %+v; want X12 Y11", got)
	}
}

// TestAbsoluteBoundsInvalidatesSubtree verifies that moving a parent repositions
// the whole subtree (the cache is cleared recursively on SetBounds).
func TestAbsoluteBoundsInvalidatesSubtree(t *testing.T) {
	parent := NewComponent(Rect{X: 0, Y: 0, W: 30, H: 30})
	child := NewComponent(Rect{X: 5, Y: 5, W: 5, H: 5})
	parent.AddChild(child)

	// Prime the child cache while the parent is at the origin.
	if got := child.AbsoluteBounds(); got != (Rect{X: 5, Y: 5, W: 5, H: 5}) {
		t.Fatalf("primed abs = %+v", got)
	}
	parent.SetBounds(Rect{X: 100, Y: 100, W: 30, H: 30})
	if got := child.AbsoluteBounds(); got != (Rect{X: 105, Y: 105, W: 5, H: 5}) {
		t.Fatalf("after parent move, child abs = %+v; want X105 Y105", got)
	}
}

// TestAbsoluteBoundsAfterReparent verifies RemoveChild/AddChild clear the cache.
func TestAbsoluteBoundsAfterReparent(t *testing.T) {
	a := NewComponent(Rect{X: 10, Y: 10, W: 20, H: 20})
	b := NewComponent(Rect{X: 50, Y: 50, W: 20, H: 20})
	mover := NewComponent(Rect{X: 0, Y: 0, W: 3, H: 3})
	a.AddChild(mover)
	if got := mover.AbsoluteBounds(); got != (Rect{X: 10, Y: 10, W: 3, H: 3}) {
		t.Fatalf("under a: abs = %+v", got)
	}
	a.RemoveChild(mover)
	b.AddChild(mover)
	if got := mover.AbsoluteBounds(); got != (Rect{X: 50, Y: 50, W: 3, H: 3}) {
		t.Fatalf("under b: abs = %+v; want X50 Y50 (recomputed, not stale)", got)
	}
}

// TestAbsoluteBoundsCachedDuringDraw is an integration check that a deep widget
// tree renders correctly (and thus that the per-frame cache composes with Draw).
func TestAbsoluteBoundsCachedDuringDraw(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(40, 10, &output)
	desktop := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 10})
	parent := NewComponent(Rect{X: 2, Y: 1, W: 20, H: 5})
	child := NewComponent(Rect{X: 3, Y: 1, W: 10, H: 1})
	child.DrawFn = func(component *VisualComponent, surface Surface) {
		abs := component.AbsoluteBounds()
		// parent(2,1) + child(3,1) = absolute (5,2)
		surface.WriteString(abs.X, abs.Y, "Z", tui.Cell{FG: tui.ANSIColor(15)})
	}
	root.AddChild(parent)
	parent.AddChild(child)
	desktop.AddLayer(NewLayer("root", root, false, false))
	desktop.Redraw()

	if app.ReadCell(5, 2).Ch != 'Z' {
		t.Fatalf("expected 'Z' at (5,2), got %q", app.ReadCell(5, 2).Ch)
	}
}

package tv

import (
	"fmt"
	tui "github.com/hobbestherat/turbotui"
	"testing"
)

// A plain click (press+release, no drag) must not create a selection, so the
// first typed character is not treated as selected and overwritten by the next.
func TestMultiLineClickThenTypeKeepsFirstChar(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 20, H: 5})
	m.Component.OnClickFn(m.Component, tui.ClickEvent{X: 0, Y: 0, Down: true})
	m.Component.OnClickFn(m.Component, tui.ClickEvent{X: 0, Y: 0, Down: false})
	if m.hasSelection() {
		t.Fatal("a plain click must not create a selection")
	}
	m.Component.OnTypeFn(m.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'})
	m.Component.OnTypeFn(m.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'b'})
	if got := m.GetText(); got != "ab" {
		t.Fatalf("expected %q, got %q", "ab", got)
	}
}

// A mouse drag (press, then move to a different cell) still creates a selection.
func TestMultiLineDragCreatesSelection(t *testing.T) {
	m := NewMultiLineInput("hello", Rect{X: 0, Y: 0, W: 20, H: 3})
	m.Component.OnClickFn(m.Component, tui.ClickEvent{X: 0, Y: 0, Down: true})
	m.Component.OnClickFn(m.Component, tui.ClickEvent{X: 3, Y: 0, Down: true}) // drag
	if !m.hasSelection() {
		t.Fatal("dragging should create a selection")
	}
	if got := m.selectionText(); got != "hel" {
		t.Fatalf("expected selection %q, got %q", "hel", got)
	}
}

// Wheel scrolling the tree must persist instead of being snapped back to the
// selection on the next frame.
func TestTreeWheelScrollPersists(t *testing.T) {
	tr := NewTree(Rect{X: 0, Y: 0, W: 20, H: 3})
	for i := 0; i < 10; i++ {
		tr.AddRoot(NewTreeNode(fmt.Sprintf("n%d", i)))
	}
	tr.handleScroll(tr.Component, tui.ScrollEvent{Delta: -1}) // wheel down
	tr.handleScroll(tr.Component, tui.ScrollEvent{Delta: -1})
	if tr.offset != 2 {
		t.Fatalf("expected offset 2 after two wheel-down events, got %d", tr.offset)
	}
}
func TestScrollbarOffsetForY(t *testing.T) {
	track := Rect{X: 9, Y: 0, W: 1, H: 10} // arrows at y=0 and y=9
	total, visible := 30, 10               // span = 20
	if off, ok := scrollbarOffsetForY(track, total, visible, 5, track.Y); !ok || off != 4 {
		t.Fatalf("top arrow should decrement: got %d ok=%v", off, ok)
	}
	if off, ok := scrollbarOffsetForY(track, total, visible, 5, track.Bottom()); !ok || off != 6 {
		t.Fatalf("bottom arrow should increment: got %d ok=%v", off, ok)
	}
	// Dragging to the very bottom of the inner track maps to the max offset.
	if off, ok := scrollbarOffsetForY(track, total, visible, 0, track.Bottom()-1); !ok || off != 20 {
		t.Fatalf("bottom of track should map to max offset 20: got %d ok=%v", off, ok)
	}
	// Nothing to scroll when content fits.
	if _, ok := scrollbarOffsetForY(track, 5, 10, 0, 3); ok {
		t.Fatal("scrollbar should report no-scroll when content fits")
	}
}

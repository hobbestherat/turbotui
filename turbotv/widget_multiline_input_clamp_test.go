package tv

import "testing"

// Reproduces the panic "slice bounds out of range [8:7]": a selection whose
// columns outlive the (shorter) line they referenced must not crash on delete.
func TestMultiLineDeleteSelectionClampsStaleColumns(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 20, H: 5})
	m.Lines = []string{"short", "line2"}
	// Stale cursor column 8 on the 5-rune "short" line, with an anchor below.
	m.selAnchorY = 1
	m.selAnchorX = 2
	m.CursorY = 0
	m.CursorX = 8
	if !m.deleteSelection() {
		t.Fatal("expected deletion to occur")
	}
	if got := m.GetText(); got != "shortne2" {
		t.Fatalf("unexpected merged text %q", got)
	}
}
func TestMultiLineSelectionTextClampsStaleColumns(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 20, H: 5})
	m.Lines = []string{"abc"}
	m.selAnchorY = 0
	m.selAnchorX = 0
	m.CursorY = 0
	m.CursorX = 99 // far past the line
	// Must not panic; returns the whole (clamped) line.
	if got := m.selectionText(); got != "abc" {
		t.Fatalf("unexpected selection text %q", got)
	}
}

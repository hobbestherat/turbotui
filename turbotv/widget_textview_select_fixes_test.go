package tv

import (
	"fmt"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// This file holds regression tests for the "fixes round 1" changes to TextView
// drag-to-select: each test pins one fix so it cannot silently regress. The
// helpers (sendTextViewClick, dragSelectTextView, newSelectTextView, drawTextView)
// live in widget_textview_select_test.go.

// --- fix: selection highlight preserves styled-span attributes (#1) ---------
// The original drawSelection overwrote each selected cell with a bare Cell, which
// dropped Bold/Italic/Underline on styled spans. The fix reads the cell back and
// swaps only FG/BG, so a styled run keeps its attributes under the highlight.

// Selecting part of a styled line keeps Bold on the selected cells (and applies
// the selection colours), while the unselected spans keep their own attributes.
func TestTextViewSelectionKeepsStyledAttributes(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 3})
	view.AddStyled([]StyledSpan{
		{Text: "AB", Bold: true, FG: tui.ANSIColor(7), HasFG: true},
		{Text: "CD", Italic: true, FG: tui.ANSIColor(7), HasFG: true},
		{Text: "EF", Underline: true, FG: tui.ANSIColor(7), HasFG: true},
	})
	view.ScrollToTop()
	dragSelectTextView(view, 0, 0, 2, 0) // select "AB"
	app := drawTextView(view, 20, 3)

	// Selected styled cells: keep Bold, gain selection FG/BG.
	a := app.ReadCell(0, 0)
	if a.Ch != 'A' || !a.Bold || a.FG != activeTheme.SelectionFG || a.BG != activeTheme.SelectionBG {
		t.Fatalf("selected 'A' = %q bold=%v fg%v bg%v, want bold preserved + selection colours",
			a.Ch, a.Bold, a.FG, a.BG)
	}
	b := app.ReadCell(1, 0)
	if b.Ch != 'B' || !b.Bold || b.BG != activeTheme.SelectionBG {
		t.Fatalf("selected 'B' = %q bold=%v bg%v, want bold preserved + selection bg", b.Ch, b.Bold, b.BG)
	}
	// Unselected styled cells: keep their own attribute and the span/view colours.
	c := app.ReadCell(2, 0)
	if c.Ch != 'C' || !c.Italic || c.FG != tui.ANSIColor(7) || c.BG != view.BG {
		t.Fatalf("unselected 'C' = %q italic=%v fg%v bg%v, want italic + span fg/view bg",
			c.Ch, c.Italic, c.FG, c.BG)
	}
	e := app.ReadCell(4, 0)
	if e.Ch != 'E' || !e.Underline {
		t.Fatalf("unselected 'E' = %q underline=%v, want underline preserved", e.Ch, e.Underline)
	}
}

// A full-row selection paints every span's text in the selection colours while
// keeping each span's Bold/Italic/Underline.
func TestTextViewFullRowStyledSelectionKeepsAllAttributes(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 3})
	view.AddStyled([]StyledSpan{
		{Text: "B", Bold: true, FG: tui.ANSIColor(7), HasFG: true},
		{Text: "I", Italic: true, FG: tui.ANSIColor(7), HasFG: true},
		{Text: "U", Underline: true, FG: tui.ANSIColor(7), HasFG: true},
		{Text: "P", FG: tui.ANSIColor(7), HasFG: true},
	})
	view.ScrollToTop()
	dragSelectTextView(view, 0, 0, 4, 0) // whole row
	app := drawTextView(view, 20, 3)

	want := []struct {
		x              int
		ch             rune
		bold, ul, ital bool
	}{
		{0, 'B', true, false, false},
		{1, 'I', false, false, true},
		{2, 'U', false, true, false},
		{3, 'P', false, false, false},
	}
	for _, w := range want {
		c := app.ReadCell(w.x, 0)
		if c.Ch != w.ch || c.Bold != w.bold || c.Underline != w.ul || c.Italic != w.ital {
			t.Errorf("cell(%d,0) = %q bold=%v ul=%v ital=%v, want %q bold=%v ul=%v ital=%v",
				w.x, c.Ch, c.Bold, c.Underline, c.Italic, w.ch, w.bold, w.ul, w.ital)
		}
		if c.BG != activeTheme.SelectionBG {
			t.Errorf("cell(%d,0) bg = %v, want selection bg (whole row selected)", w.x, c.BG)
		}
	}
}

// Copying a styled selection returns the concatenated span text.
func TestTextViewSelectionCopiesStyledSpanText(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 3})
	view.AddStyled([]StyledSpan{
		{Text: "Hello ", Bold: true, FG: tui.ANSIColor(1), HasFG: true},
		{Text: "world", Italic: true, FG: tui.ANSIColor(2), HasFG: true},
	})
	view.ScrollToTop()
	dragSelectTextView(view, 0, 0, 11, 0) // whole "Hello world"
	if got, _ := view.Component.Copy(); got != "Hello world" {
		t.Fatalf("styled selection copy = %q, want %q", got, "Hello world")
	}
}

// --- fix: scrollbar does not hijack a mid-drag selection (#3) ---------------
// The scrollbar branch now runs only when a drag is not in progress, so a motion
// event that wanders onto the track keeps extending the selection.

func TestTextViewScrollbarDoesNotHijackDrag(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%02d", i)
	}
	view := newSelectTextView(strings.Join(lines, "\n"), 20, 5) // overflows → scrollbar at col 19
	if view.Component.AbsoluteBounds().Right() != 19 {
		t.Fatal("precondition: expected the scrollbar column at x=19")
	}

	// Start a drag in the text area, then move onto the scrollbar track mid-drag.
	sendTextViewClick(view, 0, 0, true)
	sendTextViewClick(view, 19, 2, true)
	if view.draggingThumb {
		t.Fatal("mid-drag motion onto the scrollbar grabbed the thumb; it should extend the selection")
	}
	if !view.hasSelection() {
		t.Fatal("mid-drag motion onto the scrollbar should extend the selection")
	}
	sendTextViewClick(view, 19, 2, false) // release

	// A fresh press on the track (no drag in progress) must still grab the thumb.
	sendTextViewClick(view, 19, 2, true)
	if !view.draggingThumb {
		t.Fatal("a fresh press on the scrollbar track should grab the thumb")
	}
}

// --- fix: selection cleared when the content width changes (#4) -------------
// The selection's row indices are resolved against a specific content width; if a
// later draw is at a different width the content has re-wrapped, so the selection
// is dropped rather than mis-mapped. selWidth is recorded on press.

func TestTextViewSelectionClearedOnWidthChange(t *testing.T) {
	view := newSelectTextView("hello world foo", 20, 5) // fits at width 20
	dragSelectTextView(view, 0, 0, 3, 0)                // "hel", selWidth recorded
	if !view.hasSelection() {
		t.Fatal("precondition: expected a selection")
	}
	// Widen the view and redraw: the selection must be dropped.
	view.Component.SetBounds(Rect{X: 0, Y: 0, W: 30, H: 5})
	drawTextView(view, 30, 5)
	if view.hasSelection() {
		t.Fatal("selection should be cleared after the content width changed")
	}
}

// A redraw at the same content width keeps the selection (the width guard is a
// precise mismatch check, not a blanket clear).
func TestTextViewSelectionSurvivesSameWidthRedraw(t *testing.T) {
	view := newSelectTextView("hello world foo", 20, 5)
	dragSelectTextView(view, 0, 0, 3, 0)
	view.Component.SetBounds(Rect{X: 0, Y: 0, W: 20, H: 5}) // same width
	drawTextView(view, 20, 5)
	if !view.hasSelection() {
		t.Fatal("selection should survive a redraw at the same content width")
	}
	if got, _ := view.Component.Copy(); got != "hel" {
		t.Fatalf("CopyFn = %q, want %q", got, "hel")
	}
}

// Narrowing so a scrollbar appears (textWidth W → W-1) also clears the selection.
func TestTextViewSelectionClearedWhenScrollbarAppears(t *testing.T) {
	// 6 short rows in a 6-tall view fit (no bar, textWidth=W). Select, then shrink
	// the height so it overflows and a scrollbar reserves a column (textWidth=W-1).
	view := newSelectTextView("a\nb\nc\nd\ne\nf", 20, 6) // 6 rows == H, no overflow
	dragSelectTextView(view, 0, 0, 1, 0)
	if !view.hasSelection() {
		t.Fatal("precondition: expected a selection")
	}
	view.Component.SetBounds(Rect{X: 0, Y: 0, W: 20, H: 3}) // now overflows → scrollbar
	drawTextView(view, 20, 3)
	if view.hasSelection() {
		t.Fatal("selection should be cleared when a scrollbar narrows the content width")
	}
}

// --- fix: focus loss aborts an in-progress drag but keeps the selection (#6) --

func TestTextViewFocusLossAbortsDragKeepsSelection(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	sendTextViewClick(view, 0, 0, true) // press
	sendTextViewClick(view, 3, 0, true) // drag → "hel", selecting still true
	if !view.hasSelection() {
		t.Fatal("precondition: expected a selection")
	}
	view.handleFocus(view.Component, false)
	if view.selecting {
		t.Fatal("focus loss should abort the in-progress drag (selecting must become false)")
	}
	if !view.hasSelection() {
		t.Fatal("focus loss should keep the committed selection intact")
	}
	if got, _ := view.Component.Copy(); got != "hel" {
		t.Fatalf("CopyFn after focus loss = %q, want %q", got, "hel")
	}
}

// After focus loss, the next press starts a fresh selection instead of being
// misread as a drag continuation of the stale one.
func TestTextViewPressAfterFocusLossStartsFresh(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	sendTextViewClick(view, 0, 0, true)
	sendTextViewClick(view, 3, 0, true) // "hel"
	view.handleFocus(view.Component, false)

	// A new press further right, then a drag, anchors a fresh selection there.
	sendTextViewClick(view, 2, 0, true)
	sendTextViewClick(view, 4, 0, true) // "ll"
	if got, _ := view.Component.Copy(); got != "ll" {
		t.Fatalf("post-focus press should start a fresh selection = %q, want %q (got %q)", "ll", "ll", got)
	}
}

// --- fix: a click before the first draw lands on the right row (#5) ---------
// A fresh view follows the bottom (scrollY is a huge sentinel until clamped). The
// click handler now clamps before mapping the pointer, so a press at the top row
// hits row 0 instead of being clamped to the last row.

func TestTextViewClickBeforeFirstDrawLandsOnRightRow(t *testing.T) {
	// Built directly: no ScrollToTop, no draw → scrollY is still the sentinel.
	view := NewTextView("ab\ncd", Rect{X: 0, Y: 0, W: 20, H: 5})
	view.Wrap = false
	dragSelectTextView(view, 0, 0, 1, 1)
	// With clamp-before-map: scrollY→0, press@row0, drag@row1col1 → "ab\nc".
	// Without it: scrollY=1<<30 clamps the press to the last row, selecting only "c".
	if got, _ := view.Component.Copy(); got != "ab\nc" {
		t.Fatalf("click before first draw = %q, want %q (press must land on row 0)", got, "ab\\nc")
	}
}

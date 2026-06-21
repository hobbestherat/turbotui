package tv

import (
	"fmt"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// --- helpers ----------------------------------------------------------------

// sendTextViewClick dispatches a mouse click event to view's OnClickFn. Down is
// true for a press and for the motion ("drag") reports that follow while the
// button is held, false for the release. The handler tells a fresh press apart
// from continued drag via its own selecting flag, so a plain {X,Y,Down} event is
// sufficient to drive the gesture — the same way the existing MultiLineInput
// selection tests drive its handler.
func sendTextViewClick(view *TextView, x, y int, down bool) {
	view.Component.OnClickFn(view.Component, tui.ClickEvent{X: x, Y: y, Down: down})
}

// dragSelectTextView presses at (x0,y0), drags straight to (x1,y1) and releases:
// the minimal gesture that anchors a selection at the press point and extends it
// to the end point. Intermediate cells are not visited, which still exercises the
// "first motion off the press point anchors" rule because the end point differs
// from the press point.
func dragSelectTextView(view *TextView, x0, y0, x1, y1 int) {
	sendTextViewClick(view, x0, y0, true)  // press
	sendTextViewClick(view, x1, y1, true)  // drag motion to the end point
	sendTextViewClick(view, x1, y1, false) // release
}

// newSelectTextView builds a top-anchored (scrollY=0, follow off) TextView so the
// first content row sits at screen row 0 — required because freshly-built views
// follow the bottom (scrollY is a huge sentinel until a draw clamps it), which
// would otherwise make a click at Y=0 map to the last visual row.
func newSelectTextView(text string, w, h int) *TextView {
	view := NewTextView(text, Rect{X: 0, Y: 0, W: w, H: h})
	view.Wrap = false
	view.ScrollToTop()
	return view
}

// --- copy / selection model -------------------------------------------------

// Issue #174 Phase B: a drag press→drag→release selects a range and CopyFn
// returns exactly that range.
func TestTextViewDragSelectCopiesExactRange(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 0, 0, 3, 0) // h(0) … between l(2) and l(3) → "hel"

	if !view.hasSelection() {
		t.Fatal("a drag should leave a selection active")
	}
	if got := view.selectionText(); got != "hel" {
		t.Fatalf("selectionText = %q, want %q", got, "hel")
	}
	got, ok := view.Component.Copy()
	if !ok {
		t.Fatal("CopyFn ok = false with an active selection, want true")
	}
	if got != "hel" {
		t.Fatalf("CopyFn = %q, want %q", got, "hel")
	}
}

// A single rune can be selected and copied.
func TestTextViewDragSelectSingleRune(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 2, 0, 3, 0) // [2,3) → "l"
	if got, _ := view.Component.Copy(); got != "l" {
		t.Fatalf("single-rune selection = %q, want %q", got, "l")
	}
}

// With no selection, CopyFn falls back to the whole content (unchanged behaviour).
func TestTextViewNoSelectionCopiesAllContent(t *testing.T) {
	view := newSelectTextView("alpha\nbeta\ngamma", 20, 5)
	if view.hasSelection() {
		t.Fatal("a fresh view must have no selection")
	}
	got, ok := view.Component.Copy()
	if !ok {
		t.Fatal("CopyFn ok = false for non-empty content with no selection")
	}
	if got != view.AllText() {
		t.Fatalf("CopyFn = %q, want AllText %q", got, view.AllText())
	}
	if got != "alpha\nbeta\ngamma" {
		t.Fatalf("CopyFn = %q, want %q", got, "alpha\nbeta\\ngamma")
	}
}

// copyAll must keep returning the whole content even when a selection is active —
// an existing styled test calls it directly, so it must not gain selection logic.
func TestTextViewCopyAllIgnoresSelection(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 0, 0, 3, 0) // selection "hel"
	if !view.hasSelection() {
		t.Fatal("precondition: expected an active selection")
	}
	got, ok := view.copyAll(view.Component)
	if !ok || got != "hello" {
		t.Fatalf("copyAll = %q ok=%v, want %q (copyAll ignores selection)", got, ok, "hello")
	}
}

// An empty view with no selection copies nothing (ok=false).
func TestTextViewCopyEmptyViewReturnsFalse(t *testing.T) {
	view := newSelectTextView("", 10, 3)
	got, ok := view.Component.Copy()
	if ok {
		t.Fatalf("empty view CopyFn ok = true, want false (got %q)", got)
	}
	if got != "" {
		t.Fatalf("empty view CopyFn = %q, want %q", got, "")
	}
}

// A selection whose reconstructed text is empty reports ok=false so the copy is a
// no-op. The empty-text case is reachable when a selection has anchor != active on
// a row with no text: selectionText slices to the empty range, and copySelection's
// `text != ""` guard then reports nothing to copy.
func TestTextViewCopyEmptySelectionTextReturnsFalse(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 5})
	view.Wrap = false
	view.AddLine("") // one entry whose row text is ""
	view.ScrollToTop()
	view.selAnchorRow, view.selAnchorCol = 0, 0
	view.selActiveRow, view.selActiveCol = 0, 1 // hasSelection, but the row has no text
	if !view.hasSelection() {
		t.Fatal("precondition: expected an active selection")
	}
	if view.selectionText() != "" {
		t.Fatalf("selectionText = %q, want %q", view.selectionText(), "")
	}
	got, ok := view.Component.Copy()
	if ok {
		t.Fatalf("empty-text selection CopyFn ok = true, want false (got %q)", got)
	}
	if got != "" {
		t.Fatalf("empty-text selection CopyFn = %q, want %q", got, "")
	}
}

// Selecting across two empty entries reconstructs only the inter-entry newline
// (each row contributes no text), so the copy carries that single separator. This
// pins the eager newline-join as known behaviour.
func TestTextViewSelectionAcrossEmptyRowsIsNewlineOnly(t *testing.T) {
	view := newSelectTextView("\n\n", 20, 5) // three empty entries
	dragSelectTextView(view, 0, 0, 0, 1)     // row0 col0 … row1 col0
	if !view.hasSelection() {
		t.Fatal("expected a selection across two rows")
	}
	if got, _ := view.Component.Copy(); got != "\n" {
		t.Fatalf("across-empty-rows selection = %q, want %q", got, "\n")
	}
}

// --- drag geometry ----------------------------------------------------------

// A backward drag (right→left on one row) selects the same range as a forward one
// thanks to selectionOrdered normalisation.
func TestTextViewBackwardDragOnRowSelectsRange(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 3, 0, 1, 0) // from col3 back to col1 → [1,3) = "el"
	if got := view.selectionText(); got != "el" {
		t.Fatalf("backward drag = %q, want %q", got, "el")
	}
}

// A backward multi-row drag (bottom→top) selects the forward-ordered text.
func TestTextViewBackwardDragAcrossRowsSelectsOrdered(t *testing.T) {
	view := newSelectTextView("alpha\nbeta\ngamma", 20, 5)
	// "gamma" g0 a1 m2 m3 a4 — press at col3; drag up to "alpha" col1.
	dragSelectTextView(view, 3, 2, 1, 0)
	want := "lpha\nbeta\ngam" // alpha[1:]="lpha", beta full, gamma[:3]="gam"
	if got, _ := view.Component.Copy(); got != want {
		t.Fatalf("backward multi-row drag = %q, want %q", got, want)
	}
}

// A forward multi-entry selection joins the entries with a newline, matching
// AllText's one-line-per-entry shape.
func TestTextViewMultiEntrySelectionJoinsWithNewline(t *testing.T) {
	view := newSelectTextView("alpha\nbeta\ngamma", 20, 5)
	dragSelectTextView(view, 1, 0, 3, 2) // alpha col1 … gamma col3
	want := "lpha\nbeta\ngam"
	if got := view.selectionText(); got != want {
		t.Fatalf("multi-entry selection = %q, want %q", got, want)
	}
}

// Rows that are wrapped continuations of one logical line (the same entry) join
// with no separator, so a wrapped line copies as one run.
func TestTextViewWrappedSameEntrySelectionHasNoSeparator(t *testing.T) {
	view := NewTextView("ABCDEFG", Rect{X: 0, Y: 0, W: 4, H: 5}) // Wrap=true default
	view.ScrollToTop()
	// At width 4 "ABCDEFG" wraps to ["ABCD","EFG"], both the same entry.
	rows, _, _ := view.metrics(view.Component.AbsoluteBounds())
	if len(rows) != 2 || rows[0].text != "ABCD" || rows[1].text != "EFG" {
		t.Fatalf("layout precondition: want [ABCD|EFG], got %v", []string{rows[0].text, rows[1].text})
	}
	dragSelectTextView(view, 0, 0, 2, 1) // row0 col0 … row1 col2 → ABCD + EF
	if got := view.selectionText(); got != "ABCDEF" {
		t.Fatalf("wrapped same-entry selection = %q, want %q", got, "ABCDEF")
	}
}

// Selection across a foldable parent and its child: the parent's text starts after
// its 2-column marker while the child's starts after its 2-column indent, so both
// have textX=2 — verify the per-row column mapping and the cross-entry newline.
func TestTextViewSelectionAcrossFoldableParentAndChild(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 5})
	view.Wrap = false
	parent := view.AddLine("PARENT") // marker ▾ at cols 0..1, text from col 2
	parent.Add("child")              // indent 2, text from col 2
	view.ScrollToTop()

	rows, _, _ := view.metrics(view.Component.AbsoluteBounds())
	if len(rows) != 2 {
		t.Fatalf("layout precondition: want parent+child visible (2 rows), got %d", len(rows))
	}
	// Press parent text col2 (event.X=4), drag to child col3 (event.X=5).
	dragSelectTextView(view, 4, 0, 5, 1)
	want := "RENT\nchi" // PARENT[2:]="RENT", newline (different entry), child[:3]="chi"
	if got, _ := view.Component.Copy(); got != want {
		t.Fatalf("parent/child selection = %q, want %q", got, want)
	}
}

// --- plain click vs drag ----------------------------------------------------

// A plain click (press + release at the same cell, no motion) must not anchor a
// selection — copy then returns the whole content.
func TestTextViewPlainClickDoesNotSelect(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	sendTextViewClick(view, 2, 0, true)
	sendTextViewClick(view, 2, 0, false)
	if view.hasSelection() {
		t.Fatal("a plain click with no motion must not create a selection")
	}
	got, _ := view.Component.Copy()
	if got != "hello" {
		t.Fatalf("after a plain click CopyFn = %q, want whole content %q", got, "hello")
	}
}

// A release at a different cell than the press, with no drag motion in between,
// must still not select (the anchor is only committed on the first motion off the
// press point).
func TestTextViewReleaseAtDifferentCellWithoutDragDoesNotSelect(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	sendTextViewClick(view, 0, 0, true)
	sendTextViewClick(view, 3, 0, false) // release elsewhere, no motion event
	if view.hasSelection() {
		t.Fatal("release without a prior drag motion must not create a selection")
	}
	if got, _ := view.Component.Copy(); got != "hello" {
		t.Fatalf("CopyFn = %q, want %q", got, "hello")
	}
}

// A new drag replaces the previous selection rather than merging with it.
func TestTextViewNewSelectionReplacesOld(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 0, 0, 3, 0) // "hel"
	if got, _ := view.Component.Copy(); got != "hel" {
		t.Fatalf("first selection = %q, want %q", got, "hel")
	}
	dragSelectTextView(view, 2, 0, 4, 0) // "ll"
	if got, _ := view.Component.Copy(); got != "ll" {
		t.Fatalf("second selection = %q, want %q (old selection should be replaced)", got, "ll")
	}
}

// --- selection survives scrolling (required test) ---------------------------

// Issue #174 Phase B: a selection made at one scroll offset must still copy the
// same text after the view is scrolled — the anchor/active are visual-row indices
// into the (scroll-stable) layout, not screen rows.
func TestTextViewSelectionSurvivesScrolling(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%02d", i)
	}
	view := newSelectTextView(strings.Join(lines, "\n"), 20, 5)

	// Select from row0 col0 to row1 col2: "L00" + "\n" + "L01"[:2] = "L00\nL0".
	dragSelectTextView(view, 0, 0, 2, 1)
	want := "L00\nL0"
	if got, _ := view.Component.Copy(); got != want {
		t.Fatalf("selection before scroll = %q, want %q", got, want)
	}
	if view.scrollY != 0 {
		t.Fatalf("precondition: expected scrollY=0 at the top, got %d", view.scrollY)
	}

	// Scroll down five rows (Delta is negated: -5 → scrollBy(+5)); the selected
	// rows (0..1) leave the viewport but the selection state must persist.
	view.Component.OnScrollFn(view.Component, tui.ScrollEvent{Delta: -5})
	if view.scrollY != 5 {
		t.Fatalf("expected scrollY=5 after scrolling down, got %d", view.scrollY)
	}
	if !view.hasSelection() {
		t.Fatal("scrolling must not clear the selection")
	}
	if got, _ := view.Component.Copy(); got != want {
		t.Fatalf("selection after scroll down = %q, want unchanged %q", got, want)
	}

	// Scroll back to the top: the same selection still copies the same text.
	view.Component.OnScrollFn(view.Component, tui.ScrollEvent{Delta: 5})
	if view.scrollY != 0 {
		t.Fatalf("expected scrollY=0 after scrolling back up, got %d", view.scrollY)
	}
	if got, _ := view.Component.Copy(); got != want {
		t.Fatalf("selection after scroll back = %q, want unchanged %q", got, want)
	}
}

// --- highlight / draw -------------------------------------------------------

// Selected cells are repainted with the selection colours (SelectionFG/BG), and
// the cell just past the selection keeps the view's normal colours.
func TestTextViewSelectionHighlightUsesSelectionColors(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 0, 0, 2, 0) // select [0,2): 'h','e'
	app := drawTextView(view, 20, 3)

	selFG, selBG := activeTheme.SelectionFG, activeTheme.SelectionBG
	for x := 0; x < 2; x++ {
		c := app.ReadCell(x, 0)
		if c.FG != selFG || c.BG != selBG {
			t.Fatalf("cell(%d,0) = fg%v bg%v, want selection fg%v bg%v", x, c.FG, c.BG, selFG, selBG)
		}
	}
	// The cell at the selection end ('l') keeps the view's foreground/background.
	end := app.ReadCell(2, 0)
	if end.Ch != 'l' || end.BG == selBG {
		t.Fatalf("cell(2,0) = %q bg%v, want 'l' with non-selection background", end.Ch, end.BG)
	}
	if end.FG != view.FG || end.BG != view.BG {
		t.Fatalf("cell(2,0) = fg%v bg%v, want view fg%v bg%v", end.FG, end.BG, view.FG, view.BG)
	}
}

// A multi-row selection fills each fully-covered row out to the right edge with
// the selection background (blank tail painted as spaces), matching MultiLineInput.
func TestTextViewMultiRowSelectionFillsBlankTailToRightEdge(t *testing.T) {
	view := newSelectTextView("abc\ndef", 20, 3)
	dragSelectTextView(view, 0, 0, 1, 1) // row0 all … row1 col1
	app := drawTextView(view, 20, 3)

	selBG := activeTheme.SelectionBG
	// Row 0 is the first (anchor) row of a multi-row selection: every column is
	// selected, including the blank tail past 'c'.
	tail := app.ReadCell(10, 0)
	if tail.Ch != ' ' || tail.BG != selBG {
		t.Fatalf("blank-tail cell(10,0) = %q bg%v, want ' ' on selection bg", tail.Ch, tail.BG)
	}
	// Row 1 is the last (active) row: only col < c1=1 is selected, so col 1 ('e')
	// keeps the normal background.
	last := app.ReadCell(1, 1)
	if last.Ch != 'e' || last.BG == selBG {
		t.Fatalf("last-row cell(1,1) = %q bg%v, want 'e' with non-selection background", last.Ch, last.BG)
	}
}

// A single-row selection must NOT bleed the selection background into the blank
// tail past its end column.
func TestTextViewSingleRowSelectionDoesNotFillBlankTail(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 0, 0, 2, 0) // [0,2) on one row
	app := drawTextView(view, 20, 3)
	// Col 5 is past 'o' (index 4); it must keep the view background.
	c := app.ReadCell(5, 0)
	if c.BG == activeTheme.SelectionBG {
		t.Fatalf("cell(5,0) bg = selection, want normal: single-row selection leaked into blank tail")
	}
	if c.BG != view.BG {
		t.Fatalf("cell(5,0) bg = %v, want view bg %v", c.BG, view.BG)
	}
}

// The fold marker is not part of the text and must keep its own chrome colour even
// when the text beside it is selected.
func TestTextViewSelectionHighlightLeavesFoldMarkerUnpainted(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 5})
	view.Wrap = false
	parent := view.AddLine("PARENT") // marker ▾ at cols 0..1
	parent.Add("child")
	view.ScrollToTop()
	dragSelectTextView(view, 2, 0, 5, 0) // select text cols 0..2 → "PAR"
	app := drawTextView(view, 20, 3)

	// Marker cell keeps the marker chrome, not the selection background.
	marker := app.ReadCell(0, 0)
	if marker.Ch != '▾' {
		t.Fatalf("marker cell(0,0) = %q, want '▾'", marker.Ch)
	}
	if marker.BG == activeTheme.SelectionBG {
		t.Fatalf("marker cell painted with selection background; it should keep its own chrome")
	}
	// The selected text cell ('P' at col 2) does carry the selection background.
	text := app.ReadCell(2, 0)
	if text.Ch != 'P' || text.BG != activeTheme.SelectionBG {
		t.Fatalf("selected text cell(2,0) = %q bg%v, want 'P' on selection bg", text.Ch, text.BG)
	}
}

// --- fold interaction -------------------------------------------------------

// A click on a fold marker toggles the entry and does not start a selection.
func TestTextViewFoldMarkerClickTogglesWithoutSelecting(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 5})
	view.Wrap = false
	parent := view.AddLine("PARENT")
	parent.Add("child")
	view.ScrollToTop()

	rows, _, _ := view.metrics(view.Component.AbsoluteBounds())
	if len(rows) != 2 {
		t.Fatalf("precondition: want parent+child expanded (2 rows), got %d", len(rows))
	}
	// Press on the marker cell (col 0).
	sendTextViewClick(view, 0, 0, true)
	sendTextViewClick(view, 0, 0, false)
	if view.hasSelection() {
		t.Fatal("clicking a fold marker must not start a selection")
	}
	rows, _, _ = view.metrics(view.Component.AbsoluteBounds())
	if len(rows) != 1 {
		t.Fatalf("clicking the marker should collapse to 1 row, got %d", len(rows))
	}
}

// Dragging across a fold marker (motion events landing on the marker cell while a
// drag is in progress) must not toggle the entry — the marker check only runs on a
// fresh press.
func TestTextViewDragAcrossFoldMarkerDoesNotToggle(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 5})
	view.Wrap = false
	parent := view.AddLine("PARENT")
	parent.Add("child")
	view.ScrollToTop()

	// Press in the parent's text (event.X=4 → text col 2), then drag a motion
	// event onto the marker cell (event.X=1).
	sendTextViewClick(view, 4, 0, true) // press
	sendTextViewClick(view, 1, 0, true) // drag motion over the marker
	sendTextViewClick(view, 1, 0, false)

	rows, _, _ := view.metrics(view.Component.AbsoluteBounds())
	if len(rows) != 2 {
		t.Fatalf("drag across marker toggled fold (rows=%d); want still expanded (2)", len(rows))
	}
	if !view.hasSelection() {
		t.Fatal("drag over a marker should still have made a selection")
	}
}

// --- selection cleared on content/layout mutation ---------------------------

func TestTextViewToggleClearsSelection(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 0, 0, 2, 0)
	if !view.hasSelection() {
		t.Fatal("precondition: expected a selection")
	}
	parent := view.AddLine("p")
	parent.Add("c")
	parent.Toggle() // fold layout change must drop the selection
	if view.hasSelection() {
		t.Fatal("Toggle must clear the selection")
	}
}

func TestTextViewSetCollapsedClearsSelection(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 5})
	view.Wrap = false
	parent := view.AddLine("hello")
	parent.Add("world") // parent becomes foldable: marker ▾ at cols 0..1, text from col 2
	view.ScrollToTop()
	// Press in the parent's text area (col 2+), not on its marker.
	dragSelectTextView(view, 2, 0, 4, 0) // select "he"
	if !view.hasSelection() {
		t.Fatal("precondition: expected a selection")
	}
	parent.SetCollapsed(true)
	if view.hasSelection() {
		t.Fatal("SetCollapsed must clear the selection")
	}
}

func TestTextViewSetTextClearsSelection(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 0, 0, 2, 0)
	if !view.hasSelection() {
		t.Fatal("precondition: expected a selection")
	}
	view.SetText("world")
	if view.hasSelection() {
		t.Fatal("SetText must clear the selection")
	}
	if got, _ := view.Component.Copy(); got != "world" {
		t.Fatalf("after SetText CopyFn = %q, want %q", got, "world")
	}
}

func TestTextViewClearClearsSelection(t *testing.T) {
	view := newSelectTextView("hello", 20, 3)
	dragSelectTextView(view, 0, 0, 2, 0)
	if !view.hasSelection() {
		t.Fatal("precondition: expected a selection")
	}
	view.Clear()
	if view.hasSelection() {
		t.Fatal("Clear must clear the selection")
	}
	if got, ok := view.Component.Copy(); ok || got != "" {
		t.Fatalf("after Clear CopyFn = %q ok=%v, want %q ok=false", got, ok, "")
	}
}

// --- isSelected geometry (direct) -------------------------------------------

// isSelected is the heart of the highlight: a table check of which cells fall in a
// known (row0,col1)→(row2,col2) selection, independent of the click→column mapping.
func TestTextViewIsSelectedGeometry(t *testing.T) {
	view := newSelectTextView("aaaa\nbbbb\ncccc", 20, 5)
	view.selAnchorRow, view.selAnchorCol = 0, 1
	view.selActiveRow, view.selActiveCol = 2, 2
	// Selection: row0 [1,∞), row1 all, row2 [0,2).
	cases := []struct {
		row, col int
		want     bool
	}{
		{0, 0, false}, {0, 1, true}, {0, 3, true}, {0, 99, true}, // anchor row: from c0 to the right edge
		{1, 0, true}, {1, 99, true}, // fully covered middle row
		{2, 0, true}, {2, 1, true}, {2, 2, false}, {2, 99, false}, // active row: up to (not incl) c1
		{3, 0, false}, {3, 99, false}, // outside the row range
	}
	for _, c := range cases {
		if got := view.isSelected(c.row, c.col); got != c.want {
			t.Errorf("isSelected(%d,%d) = %v, want %v", c.row, c.col, got, c.want)
		}
	}
}

// selectionOrdered must normalise an anchor/active pair regardless of direction.
func TestTextViewSelectionOrderedNormalises(t *testing.T) {
	view := newSelectTextView("abc\ndef\nghi", 20, 5)
	view.selAnchorRow, view.selAnchorCol = 2, 2 // active end is "earlier"
	view.selActiveRow, view.selActiveCol = 0, 1
	r0, c0, r1, c1 := view.selectionOrdered()
	if r0 != 0 || c0 != 1 || r1 != 2 || c1 != 2 {
		t.Fatalf("selectionOrdered = (%d,%d)-(%d,%d), want (0,1)-(2,2)", r0, c0, r1, c1)
	}
}

// --- defensive / edge cases -------------------------------------------------

// Clicking an empty view must not panic and must not anchor a selection.
func TestTextViewClickEmptyViewNoSelectionNoPanic(t *testing.T) {
	view := newSelectTextView("", 10, 3)
	sendTextViewClick(view, 0, 0, true)
	sendTextViewClick(view, 0, 0, false)
	if view.hasSelection() {
		t.Fatal("clicking an empty view must not create a selection")
	}
}

// A press outside the view bounds must not start a selection.
func TestTextViewPressOutsideBoundsDoesNotSelect(t *testing.T) {
	view := newSelectTextView("hello", 10, 3)
	sendTextViewClick(view, 50, 50, true) // far outside
	sendTextViewClick(view, 50, 50, false)
	if view.hasSelection() {
		t.Fatal("a press outside the view must not start a selection")
	}
	if got, _ := view.Component.Copy(); got != "hello" {
		t.Fatalf("CopyFn = %q, want whole content %q", got, "hello")
	}
}

// selectionText must not panic when the selection references rows past the end of
// the layout (e.g. layout shrank after the selection was made) — it clamps to the
// available rows.
func TestTextViewSelectionTextClampsOutOfRangeRows(t *testing.T) {
	view := newSelectTextView("ab\ncd", 20, 5)
	view.selAnchorRow, view.selAnchorCol = 0, 0
	view.selActiveRow, view.selActiveCol = 100, 50 // far past the last row
	if !view.hasSelection() {
		t.Fatal("precondition: expected a selection")
	}
	got := view.selectionText() // must not panic
	// r1 clamps to the last row (1); col clamps to len → "ab" + "\n" + "cd".
	if got != "ab\ncd" {
		t.Fatalf("out-of-range selectionText = %q, want %q", got, "ab\\ncd")
	}
}

// A drag whose active end lands below the last row clamps to the final row, so the
// selection still copies text rather than panicking.
func TestTextViewDragBelowLastRowClamps(t *testing.T) {
	view := newSelectTextView("aaa\nbbb", 20, 8)
	// Press row0; drag to a Y well past the content (row index clamps to the last
	// row) and an X at the end of that row (column clamps to its length).
	dragSelectTextView(view, 0, 0, 3, 6)
	if !view.hasSelection() {
		t.Fatal("expected a clamped selection")
	}
	got, _ := view.Component.Copy()
	if got != "aaa\nbbb" {
		t.Fatalf("drag-below-last-row selection = %q, want %q", got, "aaa\\nbbb")
	}
}

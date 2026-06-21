package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for (*TextEntry).AddStyled — the foldable-tree counterpart of
// (*TextView).AddStyled. They mirror TestAddStyledFoldableMarkerAndIndent (which
// exercises a styled PARENT made foldable by a plain Add child) and add the
// symmetric cases: a styled CHILD created via parent.AddStyled. The shared
// drawTextView / spanRowText helpers live in widget_textview_styled_test.go.

// --- structural wiring -----------------------------------------------------

// AddStyled must return a non-nil child wired to the receiver exactly like
// addChild does, but carrying spans. Because these tests live in package tv they
// can inspect the unexported fields directly, pinning the constructor's contract.
func TestTextEntryAddStyledWiring(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 5})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	spans := []StyledSpan{
		{Text: "AB", FG: tui.ANSIColor(1), HasFG: true, Bold: true},
		{Text: "CD", FG: tui.ANSIColor(2), HasFG: true, Italic: true},
	}
	parent := view.AddColored("parent", tui.ANSIColor(3))
	parent.foldable = false // prove AddStyled flips it on, regardless of prior state

	prevVersion := view.layoutVersion
	child := parent.AddStyled(spans)

	if child == nil {
		t.Fatal("AddStyled returned nil")
	}
	// Receiver becomes foldable, so it will gain a ▾/▸ marker.
	if !parent.foldable {
		t.Fatal("parent.foldable = false, want true (AddStyled must mark the receiver foldable)")
	}
	// Child is wired as a styled leaf of the parent.
	if child.parent != parent {
		t.Fatalf("child.parent = %p, want %p", child.parent, parent)
	}
	if child.view != view {
		t.Fatalf("child.view = %p, want %p", child.view, view)
	}
	if child.text != "ABCD" {
		t.Fatalf("child.text = %q, want %q (spansText concatenation)", child.text, "ABCD")
	}
	if len(child.spans) != 2 || child.spans[0].Text != "AB" || child.spans[1].Text != "CD" {
		t.Fatalf("child.spans = %+v, want the two spans passed in", child.spans)
	}
	// A freshly-added styled child is a leaf: not foldable, no children of its own.
	if child.foldable {
		t.Fatal("child.foldable = true, want false (a new styled child is a leaf)")
	}
	if len(child.children) != 0 {
		t.Fatalf("child has %d children, want 0", len(child.children))
	}
	// The per-entry fg path is unused for a styled entry (each span owns its colour).
	if child.hasFG {
		t.Fatal("child.hasFG = true, want false (styled entries do not use entry-level fg)")
	}
	// The child is appended as the receiver's last child.
	if n := len(parent.children); n != 1 || parent.children[n-1] != child {
		t.Fatalf("parent.children = %+v, want [%p]", parent.children, child)
	}
	// touch() was called, invalidating the layout cache.
	if view.layoutVersion == prevVersion {
		t.Fatal("layoutVersion unchanged: AddStyled did not call view.touch()")
	}
}

// AddStyled must append rather than replace, preserving sibling order — the same
// contract Add/AddColored already honour.
func TestTextEntryAddStyledAppendsPreservingOrder(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 24, H: 6})
	parent := view.AddColored("parent", tui.ANSIColor(3))
	first := parent.AddStyled([]StyledSpan{{Text: "first", FG: tui.ANSIColor(1), HasFG: true}})
	parent.Add("plain kid")
	second := parent.AddStyled([]StyledSpan{{Text: "second", FG: tui.ANSIColor(2), HasFG: true}})

	want := []*TextEntry{first, parent.children[1], second}
	if len(parent.children) != 3 {
		t.Fatalf("len(parent.children) = %d, want 3", len(parent.children))
	}
	for i, c := range want {
		if parent.children[i] != c {
			t.Fatalf("parent.children[%d] = %p, want %p (order not preserved)", i, parent.children[i], c)
		}
	}
}

// --- symmetric foldable marker + indent (the core ask) ---------------------

// Case 1: a PLAIN parent (AddColored) with a STYLED child (parent.AddStyled).
// The parent shows a fold marker, the child is indented one level, and the
// child's spans keep their per-span styling.
func TestTextEntryAddStyledPlainParentStyledChild(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 6})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	red := tui.ANSIColor(3)
	green := tui.ANSIColor(2)
	blue := tui.ANSIColor(4)

	parent := view.AddColored("parent", red)
	parent.AddStyled([]StyledSpan{
		{Text: "AB", FG: green, HasFG: true, Bold: true},
		{Text: "CD", FG: blue, HasFG: true, Italic: true},
	})
	app := drawTextView(view, 20, 6)

	// Parent: fold marker at column 0 (bold), content shifted to column 2 with the
	// entry colour.
	marker := app.ReadCell(0, 0)
	if marker.Ch != '▾' {
		t.Fatalf("parent marker = %q, want ▾", marker.Ch)
	}
	if !marker.Bold {
		t.Fatalf("parent marker should be bold, got %+v", marker)
	}
	if got := app.ReadCell(2, 0); got.Ch != 'p' || got.FG != red {
		t.Fatalf("parent content (2,0) = %q fg %v, want 'p' fg %v", got.Ch, got.FG, red)
	}
	if got := app.ReadCell(1, 0).Ch; got != ' ' {
		t.Fatalf("marker gap (1,0) = %q, want blank", got)
	}

	// Child: indented one level (depth*2 = 2 columns), no marker of its own, each
	// span keeping its colour and attributes.
	if got := app.ReadCell(2, 1); got.Ch != 'A' || got.FG != green || !got.Bold {
		t.Fatalf("child span0 (2,1) = %q fg %v bold %v, want 'A' fg %v bold true", got.Ch, got.FG, got.Bold, green)
	}
	if got := app.ReadCell(3, 1); got.Ch != 'B' || got.FG != green || !got.Bold {
		t.Fatalf("child span0 (3,1) = %q fg %v bold %v, want 'B' fg %v bold true", got.Ch, got.FG, got.Bold, green)
	}
	if got := app.ReadCell(4, 1); got.Ch != 'C' || got.FG != blue || !got.Italic {
		t.Fatalf("child span1 (4,1) = %q fg %v italic %v, want 'C' fg %v italic true", got.Ch, got.FG, got.Italic, blue)
	}
	if got := app.ReadCell(5, 1); got.Ch != 'D' || got.FG != blue || !got.Italic {
		t.Fatalf("child span1 (5,1) = %q fg %v italic %v, want 'D' fg %v italic true", got.Ch, got.FG, got.Italic, blue)
	}
}

// Case 2: a STYLED parent with a STYLED child. Both carry spans; the marker,
// indentation and per-span styling are all preserved on both rows.
func TestTextEntryAddStyledStyledParentStyledChild(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 6})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	red := tui.ANSIColor(1)
	yellow := tui.ANSIColor(3)
	green := tui.ANSIColor(2)

	parent := view.AddStyled([]StyledSpan{
		{Text: "PA", FG: red, HasFG: true, Bold: true},
		{Text: "PB", FG: yellow, HasFG: true},
	})
	parent.AddStyled([]StyledSpan{
		{Text: "CA", FG: green, HasFG: true, Italic: true},
	})
	app := drawTextView(view, 20, 6)

	// Parent row: marker then two styled spans.
	if got := app.ReadCell(0, 0).Ch; got != '▾' {
		t.Fatalf("parent marker = %q, want ▾", got)
	}
	if got := app.ReadCell(2, 0); got.Ch != 'P' || got.FG != red || !got.Bold {
		t.Fatalf("parent span0 (2,0) = %q fg %v bold %v, want 'P' fg %v bold true", got.Ch, got.FG, got.Bold, red)
	}
	if got := app.ReadCell(4, 0); got.Ch != 'P' || got.FG != yellow {
		t.Fatalf("parent span1 (4,0) = %q fg %v, want 'P' fg %v", got.Ch, got.FG, yellow)
	}

	// Child row: indented one level, styled span preserved.
	if got := app.ReadCell(2, 1); got.Ch != 'C' || got.FG != green || !got.Italic {
		t.Fatalf("child span0 (2,1) = %q fg %v italic %v, want 'C' fg %v italic true", got.Ch, got.FG, got.Italic, green)
	}
	if got := app.ReadCell(3, 1); got.Ch != 'A' || got.FG != green || !got.Italic {
		t.Fatalf("child span0 (3,1) = %q fg %v italic %v, want 'A' fg %v italic true", got.Ch, got.FG, got.Italic, green)
	}
}

// Case 3: collapsing the parent hides the styled child, and expanding restores
// it — for both a plain parent and a styled parent.
func TestTextEntryAddStyledFoldHidesAndRestoresChild(t *testing.T) {
	build := func(styledParent bool) (*TextView, *TextEntry, tui.Color) {
		view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 6})
		view.FG = tui.ANSIColor(7)
		view.BG = tui.ANSIColor(0)
		green := tui.ANSIColor(2)
		var parent *TextEntry
		if styledParent {
			parent = view.AddStyled([]StyledSpan{{Text: "P", FG: tui.ANSIColor(1), HasFG: true}})
		} else {
			parent = view.AddColored("P", tui.ANSIColor(1))
		}
		parent.AddStyled([]StyledSpan{{Text: "C", FG: green, HasFG: true}})
		return view, parent, green
	}

	cases := []struct {
		name         string
		styledParent bool
	}{
		{"plain parent", false},
		{"styled parent", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view, parent, green := build(tc.styledParent)

			// Expanded: marker is ▾ and the child is visible.
			app := drawTextView(view, 20, 6)
			if got := app.ReadCell(0, 0).Ch; got != '▾' {
				t.Fatalf("expanded marker = %q, want ▾", got)
			}
			if got := app.ReadCell(2, 1); got.Ch != 'C' || got.FG != green {
				t.Fatalf("expanded child (2,1) = %q fg %v, want 'C' fg %v", got.Ch, got.FG, green)
			}

			// Collapsed: marker flips to ▸ and the child row is blank.
			parent.SetCollapsed(true)
			app = drawTextView(view, 20, 6)
			if got := app.ReadCell(0, 0).Ch; got != '▸' {
				t.Fatalf("collapsed marker = %q, want ▸", got)
			}
			if got := app.ReadCell(2, 1).Ch; got != ' ' {
				t.Fatalf("collapsed child (2,1) = %q, want blank (child must be hidden)", got)
			}

			// Re-expanded: child returns.
			parent.SetCollapsed(false)
			app = drawTextView(view, 20, 6)
			if got := app.ReadCell(2, 1); got.Ch != 'C' || got.FG != green {
				t.Fatalf("re-expanded child (2,1) = %q fg %v, want 'C' fg %v", got.Ch, got.FG, green)
			}
		})
	}
}

// --- computeRows structure (precise indent / marker / entry) ----------------

// computeRows must lay a styled child out indented one level under its parent,
// with the marker only on the parent row and the child carrying its spans.
func TestTextEntryAddStyledComputeRowsStructure(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 6})
	view.Wrap = true
	parent := view.AddColored("parent", tui.ANSIColor(3))
	child := parent.AddStyled([]StyledSpan{
		{Text: "AB", FG: tui.ANSIColor(1), HasFG: true},
		{Text: "CD", FG: tui.ANSIColor(2), HasFG: true},
	})
	rows := view.computeRows(20)

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// Parent row: indent 0, marker ▾, no spans (plain).
	if rows[0].entry != parent || rows[0].indent != 0 || rows[0].marker != '▾' {
		t.Fatalf("parent row = indent %d marker %q, want indent 0 marker ▾", rows[0].indent, rows[0].marker)
	}
	if len(rows[0].spans) != 0 {
		t.Fatalf("parent row has %d spans, want 0 (plain parent)", len(rows[0].spans))
	}
	// Child row: indent 2, no marker, two spans whose text concatenates to ABCD.
	if rows[1].entry != child || rows[1].indent != 2 || rows[1].marker != 0 {
		t.Fatalf("child row = indent %d marker %q, want indent 2 marker 0", rows[1].indent, rows[1].marker)
	}
	if got := spanRowText(rows[1].spans); got != "ABCD" {
		t.Fatalf("child row span text = %q, want %q", got, "ABCD")
	}
	if len(rows[1].spans) != 2 || rows[1].spans[0].FG != tui.ANSIColor(1) || rows[1].spans[1].FG != tui.ANSIColor(2) {
		t.Fatalf("child row spans lost per-span colours: %+v", rows[1].spans)
	}
}

// Nesting styled-under-styled-under-styled must indent one level per depth and
// give a fold marker to every entry that has children.
func TestTextEntryAddStyledNestedGrandchild(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 16, H: 6})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	red := tui.ANSIColor(1)
	green := tui.ANSIColor(2)
	blue := tui.ANSIColor(4)

	parent := view.AddStyled([]StyledSpan{{Text: "P", FG: red, HasFG: true}})
	child := parent.AddStyled([]StyledSpan{{Text: "C", FG: green, HasFG: true}})
	grand := child.AddStyled([]StyledSpan{{Text: "G", FG: blue, HasFG: true}})

	// Structural: three depths, markers on parent and child only.
	rows := view.computeRows(16)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
	want := []struct {
		entry  *TextEntry
		indent int
		marker rune
	}{
		{parent, 0, '▾'},
		{child, 2, '▾'}, // child is foldable (has the grandchild)
		{grand, 4, 0},   // grandchild is a leaf
	}
	for i, w := range want {
		if rows[i].entry != w.entry {
			t.Fatalf("row %d entry mismatch", i)
		}
		if rows[i].indent != w.indent {
			t.Fatalf("row %d indent = %d, want %d", i, rows[i].indent, w.indent)
		}
		if rows[i].marker != w.marker {
			t.Fatalf("row %d marker = %q, want %q", i, rows[i].marker, w.marker)
		}
	}

	// Drawn: parent content at col 2, child marker at col 2 / content at col 4,
	// grandchild content at col 4 — each on its own row, each with its span colour.
	app := drawTextView(view, 16, 6)
	if got := app.ReadCell(0, 0).Ch; got != '▾' {
		t.Fatalf("parent marker (0,0) = %q, want ▾", got)
	}
	if got := app.ReadCell(2, 0); got.Ch != 'P' || got.FG != red {
		t.Fatalf("parent content (2,0) = %q fg %v, want 'P' fg %v", got.Ch, got.FG, red)
	}
	if got := app.ReadCell(2, 1).Ch; got != '▾' {
		t.Fatalf("child marker (2,1) = %q, want ▾", got)
	}
	if got := app.ReadCell(4, 1); got.Ch != 'C' || got.FG != green {
		t.Fatalf("child content (4,1) = %q fg %v, want 'C' fg %v", got.Ch, got.FG, green)
	}
	if got := app.ReadCell(4, 2); got.Ch != 'G' || got.FG != blue {
		t.Fatalf("grandchild content (4,2) = %q fg %v, want 'G' fg %v", got.Ch, got.FG, blue)
	}
}

// --- AllText / copy --------------------------------------------------------

// AllText must include a styled child's concatenated text regardless of fold
// state, matching the plain AddLine/Add contract; copyAll returns the same.
func TestTextEntryAddStyledAllTextAndCopyIncludeChild(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 8})
	parent := view.AddColored("header", tui.ANSIColor(3))
	parent.AddStyled([]StyledSpan{
		{Text: "answer ", Bold: true},
		{Text: "part", Italic: true},
	})
	view.AddLine("after")
	want := "header\nanswer part\nafter"
	if got := view.AllText(); got != want {
		t.Fatalf("AllText (expanded) = %q, want %q", got, want)
	}
	// Collapsing hides the child on screen but AllText still includes it.
	parent.SetCollapsed(true)
	if got := view.AllText(); got != want {
		t.Fatalf("AllText (collapsed) = %q, want %q", got, want)
	}
	if got, ok := view.copyAll(view.Component); !ok || got != want {
		t.Fatalf("copyAll = %q ok %v, want %q ok true", got, ok, want)
	}
	_ = parent // keep parent referenced
}

// --- span-aware wrapping of a styled child ---------------------------------

// A long styled child must wrap across rows under the parent, each wrapped row
// indented one level and keeping its span colour.
func TestTextEntryAddStyledChildWrapsSpanAware(t *testing.T) {
	// Width 7: the child's available width is 7 - indent(2) = 5, so "hello world"
	// wraps at the space (the space is absorbed by the break) -> "hello" / "world".
	// (At avail 6 the trailing space would be kept on row 0 — the same
	// keep-internal-whitespace rule wrapText uses; parity is already covered by
	// TestWrapStyledSpansMatchesPlainWrapText.)
	view := NewTextView("", Rect{X: 0, Y: 0, W: 7, H: 6})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	red := tui.ANSIColor(1)
	green := tui.ANSIColor(2)

	parent := view.AddColored("P", tui.ANSIColor(3))
	parent.AddStyled([]StyledSpan{
		{Text: "hello ", FG: red, HasFG: true},
		{Text: "world", FG: green, HasFG: true},
	})

	// Structural: child wraps at avail = 7 - indent(2) = 5 -> "hello" / "world".
	rows := view.computeRows(7)
	// parent row + two child rows.
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
	if rows[0].indent != 0 || rows[0].marker != '▾' {
		t.Fatalf("parent row = indent %d marker %q, want 0/▾", rows[0].indent, rows[0].marker)
	}
	for i, r := range rows[1:] {
		if r.indent != 2 {
			t.Fatalf("wrapped child row %d indent = %d, want 2", i, r.indent)
		}
		if r.marker != 0 {
			t.Fatalf("wrapped child row %d marker = %q, want none", i, r.marker)
		}
	}
	if got := spanRowText(rows[1].spans); got != "hello" {
		t.Fatalf("wrapped child row 0 = %q, want %q", got, "hello")
	}
	if got := spanRowText(rows[2].spans); got != "world" {
		t.Fatalf("wrapped child row 1 = %q, want %q", got, "world")
	}

	// Drawn: "hello" in red on screen row 1, "world" in green on screen row 2,
	// both starting at column 2 (the one-level indent).
	app := drawTextView(view, 7, 6)
	for x, want := range "hello" {
		if got := app.ReadCell(2+x, 1); got.Ch != want || got.FG != red {
			t.Fatalf("wrapped row0 (%d,1) = %q fg %v, want %q fg %v", 2+x, got.Ch, got.FG, want, red)
		}
	}
	for x, want := range "world" {
		if got := app.ReadCell(2+x, 2); got.Ch != want || got.FG != green {
			t.Fatalf("wrapped row1 (%d,2) = %q fg %v, want %q fg %v", 2+x, got.Ch, got.FG, want, green)
		}
	}
}

// --- cache invalidation ----------------------------------------------------

// Adding a styled child after a draw must invalidate the memoised layout so the
// child appears on the next draw (i.e. touch was really called).
func TestTextEntryAddStyledInvalidatesLayoutCache(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 6})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	green := tui.ANSIColor(2)
	parent := view.AddColored("parent", tui.ANSIColor(3))

	// First draw populates the layout/metric caches with no child present.
	app := drawTextView(view, 20, 6)
	if got := app.ReadCell(2, 1).Ch; got != ' ' {
		t.Fatalf("expected blank where the child will go, got %q", got)
	}

	prev := view.layoutVersion
	parent.AddStyled([]StyledSpan{{Text: "C", FG: green, HasFG: true}})
	if view.layoutVersion == prev {
		t.Fatal("layoutVersion unchanged after AddStyled (touch not called)")
	}

	// Second draw must reflect the newly added child.
	app = drawTextView(view, 20, 6)
	if got := app.ReadCell(2, 1); got.Ch != 'C' || got.FG != green {
		t.Fatalf("child not rendered after cache invalidation: (2,1) = %q fg %v, want 'C' fg %v", got.Ch, got.FG, green)
	}
}

// --- input non-mutation invariant ------------------------------------------

// Rendering and wrapping must not mutate the caller's spans slice — AddStyled
// stores it, but draw/wrapStyledSpans build fresh per-row spans rather than
// editing in place. (A regression here would silently corrupt reused spans.)
func TestTextEntryAddStyledDoesNotMutateInputSpans(t *testing.T) {
	spans := []StyledSpan{
		{Text: "hello ", FG: tui.ANSIColor(1), HasFG: true, Bold: true},
		{Text: "world", FG: tui.ANSIColor(2), HasFG: true, Italic: true},
	}
	snapshot := make([]StyledSpan, len(spans))
	copy(snapshot, spans)

	view := NewTextView("", Rect{X: 0, Y: 0, W: 6, H: 4})
	parent := view.AddColored("P", tui.ANSIColor(3))
	parent.AddStyled(spans)
	// Force both the wrap path and the draw path.
	_ = view.computeRows(6)
	_ = drawTextView(view, 6, 4)

	for i := range spans {
		if spans[i] != snapshot[i] {
			t.Fatalf("input span %d mutated by render/wrap: got %+v, want %+v", i, spans[i], snapshot[i])
		}
	}
}

// --- edge cases (must not panic) -------------------------------------------

// AddStyled with nil or all-empty spans on an entry must not panic, must still
// mark the parent foldable, and the empty child must render as an empty row.
func TestTextEntryAddStyledNilAndEmptySpansNoPanic(t *testing.T) {
	cases := []struct {
		name  string
		spans []StyledSpan
	}{
		{"nil spans", nil},
		{"empty slice", []StyledSpan{}},
		{"single empty text", []StyledSpan{{Text: ""}}},
		{"two empty texts", []StyledSpan{{Text: ""}, {Text: ""}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()
			view := NewTextView("", Rect{X: 0, Y: 0, W: 10, H: 4})
			view.FG = tui.ANSIColor(7)
			view.BG = tui.ANSIColor(0)
			parent := view.AddColored("parent", tui.ANSIColor(3))
			child := parent.AddStyled(tc.spans)
			if !parent.foldable {
				t.Fatal("parent.foldable = false, want true even for empty spans")
			}
			if got := child.GetText(); got != "" {
				t.Fatalf("empty-spans child text = %q, want %q", got, "")
			}
			_ = drawTextView(view, 10, 4) // draw must not panic
		})
	}
}

// A styled child of a styled parent at a tiny width must not panic while wrapping.
func TestTextEntryAddStyledZeroWidthNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	view := NewTextView("", Rect{X: 0, Y: 0, W: 1, H: 4})
	parent := view.AddStyled([]StyledSpan{{Text: "P", FG: tui.ANSIColor(1), HasFG: true}})
	parent.AddStyled([]StyledSpan{{Text: "hello world", FG: tui.ANSIColor(2), HasFG: true}})
	_ = drawTextView(view, 1, 4)
}

// --- mixed tree: styled child can itself have plain/styled descendants ------

// A styled child that also has plain children lays every descendant out at the
// right depth, so a header -> styled answer -> plain detail tree renders fully.
func TestTextEntryAddStyledChildWithPlainGrandchildren(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 24, H: 8})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	green := tui.ANSIColor(2)

	header := view.AddColored("## Assistant", tui.ANSIColor(3))
	answer := header.AddStyled([]StyledSpan{{Text: "the answer", FG: green, HasFG: true, Bold: true}})
	answer.Add("detail one")
	answer.Add("detail two")

	rows := view.computeRows(24)
	// header (marker, indent 0), answer (marker, indent 2), two details (indent 4).
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4: %+v", len(rows), rows)
	}
	if rows[0].entry != header || rows[0].indent != 0 || rows[0].marker != '▾' {
		t.Fatalf("header row wrong: indent %d marker %q", rows[0].indent, rows[0].marker)
	}
	if rows[1].entry != answer || rows[1].indent != 2 || rows[1].marker != '▾' {
		t.Fatalf("answer row wrong: indent %d marker %q", rows[1].indent, rows[1].marker)
	}
	if rows[2].indent != 4 || rows[2].marker != 0 || rows[2].text != "detail one" {
		t.Fatalf("detail one row wrong: indent %d marker %q text %q", rows[2].indent, rows[2].marker, rows[2].text)
	}
	if rows[3].indent != 4 || rows[3].marker != 0 || rows[3].text != "detail two" {
		t.Fatalf("detail two row wrong: indent %d marker %q text %q", rows[3].indent, rows[3].marker, rows[3].text)
	}
	// And AllText reflects the whole tree regardless of fold state.
	want := "## Assistant\nthe answer\ndetail one\ndetail two"
	if got := view.AllText(); got != want {
		t.Fatalf("AllText = %q, want %q", got, want)
	}
}

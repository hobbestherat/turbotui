package tv

import (
	"bytes"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// --- helpers ---------------------------------------------------------------

// spanRowText concatenates the text of every span in a visual row, giving the
// row's plain-text form.
func spanRowText(spans []StyledSpan) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// drawTextView renders view into a fresh app/surface sized w×h and returns the
// app so cells can be inspected via ReadCell. The view is constructed with a
// matching root bounds so AbsoluteBounds() reports (0,0,w,h).
func drawTextView(view *TextView, w, h int) *tui.App {
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	view.draw(view.Component, surface)
	return app
}

// --- AllText / GetText -----------------------------------------------------

// AddStyled must set the entry's plain text to the concatenation of the span
// texts, so AllText/copy behave like AddLine.
func TestAddStyledAllTextIsSpanConcatenation(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 5})
	entry := view.AddStyled([]StyledSpan{
		{Text: "Hello ", Bold: true},
		{Text: "world", Italic: true},
	})
	if got := entry.GetText(); got != "Hello world" {
		t.Fatalf("entry text = %q, want %q", got, "Hello world")
	}
	if got := view.AllText(); got != "Hello world" {
		t.Fatalf("AllText = %q, want %q", got, "Hello world")
	}
}

// AllText of a styled entry still includes its foldable children, matching the
// plain AddLine contract.
func TestAddStyledAllTextIncludesChildren(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 8})
	parent := view.AddStyled([]StyledSpan{{Text: "[Agent] summary", Bold: true}})
	parent.Add("detail one")
	parent.Add("detail two")
	parent.SetCollapsed(true)
	view.AddLine("plain line")
	want := "[Agent] summary\ndetail one\ndetail two\nplain line"
	if got := view.AllText(); got != want {
		t.Fatalf("AllText mismatch:\n got %q\nwant %q", got, want)
	}
}

// AddStyled must return a non-nil entry whose GetText is the concatenation even
// for a single span.
func TestAddStyledReturnsEntryWithConcatText(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 3})
	entry := view.AddStyled([]StyledSpan{{Text: "only", FG: tui.ANSIColor(2), HasFG: true}})
	if entry == nil {
		t.Fatal("AddStyled returned nil")
	}
	if got := entry.GetText(); got != "only" {
		t.Fatalf("entry text = %q, want %q", got, "only")
	}
}

// The CopyFn (copyAll) must return the concatenated span text, so copying a
// styled line yields its plain form exactly as AllText does.
func TestAddStyledCopyAllReturnsConcatenatedText(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 3})
	view.AddStyled([]StyledSpan{
		{Text: "Hello ", Bold: true, FG: tui.ANSIColor(1), HasFG: true},
		{Text: "world", Italic: true, FG: tui.ANSIColor(2), HasFG: true},
	})
	got, ok := view.copyAll(view.Component)
	if !ok {
		t.Fatalf("copyAll ok = false, want true")
	}
	if got != "Hello world" {
		t.Fatalf("copyAll = %q, want %q", got, "Hello world")
	}

	// An empty view copies nothing.
	empty := NewTextView("", Rect{X: 0, Y: 0, W: 10, H: 3})
	if _, ok := empty.copyAll(empty.Component); ok {
		t.Fatalf("empty view copyAll ok = true, want false")
	}
}

// --- per-span painting -----------------------------------------------------

// Each span's foreground colour lands in the correct cells.
func TestAddStyledPaintsEachSpanForeground(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 3})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	red := tui.ANSIColor(1)
	green := tui.ANSIColor(2)
	view.AddStyled([]StyledSpan{
		{Text: "AB", FG: red, HasFG: true},
		{Text: "CD", FG: green, HasFG: true},
	})
	app := drawTextView(view, 20, 3)

	if got := app.ReadCell(0, 0); got.Ch != 'A' || got.FG != red {
		t.Fatalf("cell(0,0) = %q fg %v, want 'A' fg %v", got.Ch, got.FG, red)
	}
	if got := app.ReadCell(1, 0); got.Ch != 'B' || got.FG != red {
		t.Fatalf("cell(1,0) = %q fg %v, want 'B' fg %v", got.Ch, got.FG, red)
	}
	if got := app.ReadCell(2, 0); got.Ch != 'C' || got.FG != green {
		t.Fatalf("cell(2,0) = %q fg %v, want 'C' fg %v", got.Ch, got.FG, green)
	}
	if got := app.ReadCell(3, 0); got.Ch != 'D' || got.FG != green {
		t.Fatalf("cell(3,0) = %q fg %v, want 'D' fg %v", got.Ch, got.FG, green)
	}
}

// Each span's Bold/Underline/Italic attributes land in the correct cells.
func TestAddStyledPaintsBoldUnderlineItalic(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 3})
	view.AddStyled([]StyledSpan{
		{Text: "B", Bold: true, FG: tui.ANSIColor(7), HasFG: true},
		{Text: "U", Underline: true, FG: tui.ANSIColor(7), HasFG: true},
		{Text: "I", Italic: true, FG: tui.ANSIColor(7), HasFG: true},
		{Text: "P", FG: tui.ANSIColor(7), HasFG: true},
	})
	app := drawTextView(view, 20, 3)

	cases := []struct {
		x              int
		ch             rune
		bold, ul, ital bool
	}{
		{0, 'B', true, false, false},
		{1, 'U', false, true, false},
		{2, 'I', false, false, true},
		{3, 'P', false, false, false},
	}
	for _, c := range cases {
		got := app.ReadCell(c.x, 0)
		if got.Ch != c.ch || got.Bold != c.bold || got.Underline != c.ul || got.Italic != c.ital {
			t.Fatalf("cell(%d,0) = %q bold=%v ul=%v ital=%v, want %q bold=%v ul=%v ital=%v",
				c.x, got.Ch, got.Bold, got.Underline, got.Italic, c.ch, c.bold, c.ul, c.ital)
		}
	}
}

// A span with HasFG=false falls back to the TextView's FG.
func TestAddStyledSpanWithoutFGUsesViewFG(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 3})
	view.FG = tui.ANSIColor(9)
	view.AddStyled([]StyledSpan{
		{Text: "A", HasFG: false},
		{Text: "B", FG: tui.ANSIColor(2), HasFG: true},
	})
	app := drawTextView(view, 20, 3)
	if got := app.ReadCell(0, 0).FG; got != tui.ANSIColor(9) {
		t.Fatalf("span without FG: cell(0,0) fg = %v, want view FG %v", got, tui.ANSIColor(9))
	}
	if got := app.ReadCell(1, 0).FG; got != tui.ANSIColor(2) {
		t.Fatalf("span with FG: cell(1,0) fg = %v, want %v", got, tui.ANSIColor(2))
	}
}

// A span with HasBG=true paints its background; without it the view BG is used.
func TestAddStyledBackground(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 3})
	view.BG = tui.ANSIColor(0)
	view.FG = tui.ANSIColor(15)
	spanBG := tui.ANSIColor(5)
	view.AddStyled([]StyledSpan{
		{Text: "A", BG: spanBG, HasBG: true},
		{Text: "B", HasBG: false},
	})
	app := drawTextView(view, 20, 3)
	if got := app.ReadCell(0, 0).BG; got != spanBG {
		t.Fatalf("span with BG: cell(0,0) bg = %v, want %v", got, spanBG)
	}
	if got := app.ReadCell(1, 0).BG; got != tui.ANSIColor(0) {
		t.Fatalf("span without BG: cell(1,0) bg = %v, want view BG %v", got, tui.ANSIColor(0))
	}
}

// A single styled span renders identically to an AddColored line of the same
// text/colour, proving the styled path produces the same cells as the plain one.
func TestAddStyledSingleSpanMatchesAddColored(t *testing.T) {
	mk := func() *TextView {
		v := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 3})
		v.FG = tui.ANSIColor(7)
		v.BG = tui.ANSIColor(0)
		return v
	}
	col := tui.ANSIColor(3)

	plain := mk()
	plain.AddColored("hello", col)
	papp := drawTextView(plain, 20, 3)

	styled := mk()
	styled.AddStyled([]StyledSpan{{Text: "hello", FG: col, HasFG: true}})
	sapp := drawTextView(styled, 20, 3)

	for x := 0; x < 5; x++ {
		pc := papp.ReadCell(x, 0)
		sc := sapp.ReadCell(x, 0)
		if pc != sc {
			t.Fatalf("cell(%d,0) differs: plain %+v styled %+v", x, pc, sc)
		}
	}
}

// --- span-aware wrapping ---------------------------------------------------

// wrapStyledSpans splits a styled line at word boundaries and drops the
// whitespace absorbed at the wrap point, one []StyledSpan per visual row.
func TestWrapStyledSpansSplitsAtWordBoundary(t *testing.T) {
	spans := []StyledSpan{
		{Text: "hello ", FG: tui.ANSIColor(1), HasFG: true, Bold: true},
		{Text: "world", FG: tui.ANSIColor(2), HasFG: true, Italic: true},
	}
	rows := wrapStyledSpans(spans, 5)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %v", len(rows), rowsTextOf(rows))
	}
	if got := spanRowText(rows[0]); got != "hello" {
		t.Fatalf("row0 text = %q, want %q", got, "hello")
	}
	if got := spanRowText(rows[1]); got != "world" {
		t.Fatalf("row1 text = %q, want %q", got, "world")
	}
	// The styling survives the split: row 0 is span 0 (bold), row 1 is span 1 (italic).
	if rows[0][0].Bold != true || rows[0][0].FG != tui.ANSIColor(1) {
		t.Fatalf("row0 span lost its style: %+v", rows[0][0])
	}
	if rows[1][0].Italic != true || rows[1][0].FG != tui.ANSIColor(2) {
		t.Fatalf("row1 span lost its style: %+v", rows[1][0])
	}
}

// rowsTextOf flattens wrapStyledSpans output to plain row strings for messages.
func rowsTextOf(rows [][]StyledSpan) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = spanRowText(r)
	}
	return out
}

// A hard split that lands in the MIDDLE of a span must regroup that row's runes
// back into the correct spans. "abcd" (span0) + "ef" (span1) at width 3 yields
// rows ["abc", "def"] where "def" must split into span0 "d" and span1 "ef".
func TestWrapStyledSpansHardSplitMidSpan(t *testing.T) {
	spans := []StyledSpan{
		{Text: "abcd", FG: tui.ANSIColor(1), HasFG: true, Bold: true},
		{Text: "ef", FG: tui.ANSIColor(2), HasFG: true, Italic: true},
	}
	rows := wrapStyledSpans(spans, 3)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %v", len(rows), rowsTextOf(rows))
	}
	if got := spanRowText(rows[0]); got != "abc" {
		t.Fatalf("row0 = %q, want %q", got, "abc")
	}
	if got := spanRowText(rows[1]); got != "def" {
		t.Fatalf("row1 = %q, want %q", got, "def")
	}
	// Row 1 straddles the span boundary: "d" keeps span0 (bold), "ef" keeps span1 (italic).
	if len(rows[1]) != 2 {
		t.Fatalf("row1 has %d spans, want 2: %+v", len(rows[1]), rows[1])
	}
	if rows[1][0].Text != "d" || !rows[1][0].Bold {
		t.Fatalf("row1[0] = %+v, want Text=%q Bold=true", rows[1][0], "d")
	}
	if rows[1][1].Text != "ef" || !rows[1][1].Italic {
		t.Fatalf("row1[1] = %+v, want Text=%q Italic=true", rows[1][1], "ef")
	}
}

// Property: a uniformly-styled line wraps to exactly the same row texts as the
// plain wrapText on the concatenated text. This guards the span-aware wrapper
// against drift from the canonical word-wrap algorithm.
func TestWrapStyledSpansMatchesPlainWrapText(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
	}{
		{"fits", "hello", 10},
		{"exact", "hello", 5},
		{"wrap at space", "hello world", 7},
		{"two words per row", "a b c", 3},
		{"hard split long word", "abcdefghij", 4},
		{"multiple spaces kept", "a   b   c", 6},
		{"leading indent", "    hello world", 8},
		{"tab expands then wraps", "\thello world", 6},
		{"wide glyphs", "世界test", 5},
		{"long no spaces", "supercalifragilistic", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := wrapText(tc.text, tc.width)
			got := rowsTextOf(wrapStyledSpans([]StyledSpan{{Text: tc.text}}, tc.width))
			if len(got) != len(want) {
				t.Fatalf("wrap %q width %d: got %v (rows=%d), want %v (rows=%d)",
					tc.text, tc.width, got, len(got), want, len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("wrap %q width %d row %d = %q, want %q\nfull got=%v want=%v",
						tc.text, tc.width, i, got[i], want[i], got, want)
				}
			}
		})
	}
}

// Span styling is preserved on every wrapped row when the styling varies per
// span. Each 2-char word is exactly one row wide, so the three spans land on
// three separate rows carrying their own attributes.
func TestWrapStyledSpansPreservesStyleAcrossRows(t *testing.T) {
	spans := []StyledSpan{
		{Text: "MM ", FG: tui.ANSIColor(1), HasFG: true, Bold: true},
		{Text: "NN ", FG: tui.ANSIColor(2), HasFG: true, Italic: true},
		{Text: "OO", FG: tui.ANSIColor(3), HasFG: true, Underline: true},
	}
	rows := wrapStyledSpans(spans, 2)
	want := []struct {
		text           string
		bold, ital, ul bool
	}{
		{"MM", true, false, false},
		{"NN", false, true, false},
		{"OO", false, false, true},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows (%v), want %d", len(rows), rowsTextOf(rows), len(want))
	}
	for i, exp := range want {
		got := spanRowText(rows[i])
		// Each wrapped row keeps the trailing-space-free word and one span.
		if strings.TrimSpace(got) != exp.text {
			t.Fatalf("row %d text = %q, want %q (full %v)", i, got, exp.text, rowsTextOf(rows))
		}
		s := rows[i][0]
		if s.Bold != exp.bold || s.Italic != exp.ital || s.Underline != exp.ul {
			t.Fatalf("row %d (%q) style = bold=%v ital=%v ul=%v, want bold=%v ital=%v ul=%v",
				i, exp.text, s.Bold, s.Italic, s.Underline, exp.bold, exp.ital, exp.ul)
		}
	}
}

// Wrapping a styled line through the full draw path paints each visual row on its
// own screen line, with each segment keeping its span colour.
func TestAddStyledWrapsAcrossRowsInDraw(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 6, H: 4})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	red := tui.ANSIColor(1)
	green := tui.ANSIColor(2)
	// "hello world" wraps at width 6 -> "hello " / "world".
	view.AddStyled([]StyledSpan{
		{Text: "hello ", FG: red, HasFG: true},
		{Text: "world", FG: green, HasFG: true},
	})
	app := drawTextView(view, 6, 4)

	// Row 0: "hello" in red (the trailing space is dropped at the wrap point).
	for x, want := range "hello" {
		if got := app.ReadCell(x, 0); got.Ch != want || got.FG != red {
			t.Fatalf("row0 cell(%d,0) = %q fg %v, want %q fg %v", x, got.Ch, got.FG, want, red)
		}
	}
	// Row 1: "world" in green.
	for x, want := range "world" {
		if got := app.ReadCell(x, 1); got.Ch != want || got.FG != green {
			t.Fatalf("row1 cell(%d,1) = %q fg %v, want %q fg %v", x, got.Ch, got.FG, want, green)
		}
	}
}

// --- foldable marker interaction ------------------------------------------

// A styled entry made foldable still draws its ▾/▸ marker on the first row and
// offsets the styled content by the marker width; continuation rows are indented
// to match.
func TestAddStyledFoldableMarkerAndIndent(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 6})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	red := tui.ANSIColor(1)
	parent := view.AddStyled([]StyledSpan{{Text: "parent", FG: red, HasFG: true}})
	parent.Add("child") // makes parent.foldable, so it gets a ▾ marker
	app := drawTextView(view, 20, 6)

	// Column 0 holds the marker (bold), content starts at column 2.
	marker := app.ReadCell(0, 0)
	if marker.Ch != '▾' {
		t.Fatalf("marker cell = %q, want ▾", marker.Ch)
	}
	if !marker.Bold {
		t.Fatalf("marker should be bold, got %+v", marker)
	}
	content := app.ReadCell(2, 0)
	if content.Ch != 'p' || content.FG != red {
		t.Fatalf("content cell(2,0) = %q fg %v, want 'p' fg %v", content.Ch, content.FG, red)
	}
	// Column 1 is the marker's reserved gap (blank fill), not content.
	if got := app.ReadCell(1, 0).Ch; got != ' ' {
		t.Fatalf("gap cell(1,0) = %q, want blank", got)
	}

	// The child is a leaf (not foldable), so it has no marker of its own: it
	// renders indented by 2 under the parent, "child" starting at column 2.
	found := false
	for y := 1; y < 6; y++ {
		if app.ReadCell(2, y).Ch == 'c' && app.ReadCell(3, y).Ch == 'h' {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("did not find indented child row ('c' at x=2)")
	}
}

// computeRows gives the marker only to the first row of a wrapped styled entry.
func TestStyledComputeRowsMarkerOnlyOnFirstRow(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 6, H: 8})
	view.Wrap = true
	parent := view.AddStyled([]StyledSpan{
		{Text: "hello world foo", FG: tui.ANSIColor(1), HasFG: true},
	})
	parent.Add("kid") // foldable -> marker on first row
	rows := view.computeRows(6)
	// First row of the parent carries the marker; its wrapped continuations do not.
	first := -1
	for i, r := range rows {
		if r.entry == parent {
			if first == -1 {
				first = i
				if r.marker != '▾' {
					t.Fatalf("first parent row marker = %q, want ▾", r.marker)
				}
			} else {
				if r.marker != 0 {
					t.Fatalf("continuation row %d has marker %q, want none", i, r.marker)
				}
			}
		}
	}
	if first == -1 {
		t.Fatalf("parent rows not found in computeRows output")
	}
}

// --- wrap-off clipping -----------------------------------------------------

// With Wrap off, a styled line longer than the width is clipped at the width and
// does not spill past the content area.
func TestAddStyledWrapOffClipsToWidth(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 5, H: 2})
	view.Wrap = false
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	view.AddStyled([]StyledSpan{{Text: "abcdefghij", FG: tui.ANSIColor(1), HasFG: true}})
	app := drawTextView(view, 5, 2)

	for x := 0; x < 5; x++ {
		if got := app.ReadCell(x, 0); got.Ch != rune("abcdefghij"[x]) {
			t.Fatalf("clipped cell(%d,0) = %q, want %q", x, got.Ch, string("abcdefghij"[x]))
		}
	}
	// Nothing renders on row 1 (no wrap), and nothing past the buffer.
	if got := app.ReadCell(0, 1).Ch; got != ' ' {
		t.Fatalf("row1 should be blank, got %q", got)
	}
}

// --- edge / error cases (must not panic) ----------------------------------

// AddStyled with nil or all-empty spans must not panic and renders as an empty
// plain line.
func TestAddStyledNilAndEmptySpansNoPanic(t *testing.T) {
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
			view := NewTextView("", Rect{X: 0, Y: 0, W: 10, H: 3})
			entry := view.AddStyled(tc.spans)
			if got := entry.GetText(); got != "" {
				t.Fatalf("empty-spans entry text = %q, want %q", got, "")
			}
			_ = drawTextView(view, 10, 3) // draw must not panic
		})
	}
}

// A zero/negative available width (tiny view) must not panic when wrapping.
func TestAddStyledWrappingAtZeroWidthNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	_ = wrapStyledSpans([]StyledSpan{{Text: "hello world"}}, 0)
	_ = wrapStyledSpans([]StyledSpan{{Text: "hello world"}}, -3)
	view := NewTextView("", Rect{X: 0, Y: 0, W: 1, H: 3})
	view.AddStyled([]StyledSpan{{Text: "hello world", FG: tui.ANSIColor(1), HasFG: true}})
	_ = drawTextView(view, 1, 3)
}

// Tabs inside styled spans expand to the same column stops as plain text, so a
// tab followed by text stays aligned (and the expansion inherits the tab's span).
func TestAddStyledTabExpansion(t *testing.T) {
	spans := []StyledSpan{
		{Text: "a\tb", FG: tui.ANSIColor(1), HasFG: true, Bold: true},
	}
	rows := wrapStyledSpans(spans, 20)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %v", len(rows), rowsTextOf(rows))
	}
	// "a\tb" -> "a" + 7 spaces + "b" (tab to next multiple of 8).
	if got := spanRowText(rows[0]); got != "a       b" {
		t.Fatalf("tab-expanded row = %q, want %q", got, "a       b")
	}
}

// --- wide-glyph handling in the styled path --------------------------------

// A double-width glyph in a styled span advances two columns (the continuation
// cell is laid down by SetCell), matching the plain WriteString behaviour.
func TestAddStyledWideGlyphAdvancesTwoColumns(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 10, H: 2})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	view.AddStyled([]StyledSpan{{Text: "a世b", FG: tui.ANSIColor(1), HasFG: true}})
	app := drawTextView(view, 10, 2)

	if got := app.ReadCell(0, 0); got.Ch != 'a' {
		t.Fatalf("cell(0,0) = %q, want 'a'", got.Ch)
	}
	if got := app.ReadCell(1, 0); got.Ch != '世' {
		t.Fatalf("cell(1,0) = %q, want '世'", got.Ch)
	}
	// 'b' must follow the wide glyph at column 3, keeping the row aligned.
	if got := app.ReadCell(3, 0); got.Ch != 'b' {
		t.Fatalf("cell(3,0) = %q, want 'b' (wide glyph should advance 2 columns)", got.Ch)
	}
}

// A combining mark in a styled span must fold into its base glyph, matching the
// plain WriteString path. The input is the explicitly-decomposed form "e" + U+0301
// (combining acute) + "x": one grapheme in one cell, with the mark carried in the
// base cell's Combining field, so "x" lands at column 1.
//
// (Regression guard: an earlier per-cell SetCell implementation rendered the mark
// as a stray 1-column glyph and shifted every following char right. The current
// per-span WriteString path folds it correctly. Residual edge, not asserted here:
// a mark placed at the START of a span whose base is in the PREVIOUS span is
// dropped, since each span is a separate WriteString call \u2014 pathological for
// Markdown, where a mark always shares its base's styled run.)
func TestAddStyledCombiningMarkFoldsIntoBase(t *testing.T) {
	decomposed := "e\u0301x" // 'e' + combining acute (U+0301) + 'x'
	view := NewTextView("", Rect{X: 0, Y: 0, W: 10, H: 2})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	view.AddStyled([]StyledSpan{{Text: decomposed, FG: tui.ANSIColor(1), HasFG: true}})
	app := drawTextView(view, 10, 2)

	base := app.ReadCell(0, 0)
	if base.Ch != 'e' {
		t.Fatalf("base cell = %q, want 'e'", base.Ch)
	}
	if !strings.ContainsRune(base.Combining, '\u0301') {
		t.Fatalf("combining acute not folded into base cell, Combining=%q", base.Combining)
	}
	// 'x' must land at column 1 (the combining mark does not consume a column).
	if got := app.ReadCell(1, 0).Ch; got != 'x' {
		t.Fatalf("cell(1,0) = %q, want 'x' (combining mark must not consume a column)", got)
	}
}

// A styled single-span line must render cell-for-cell identically to the plain
// AddLine path, even for content mixing a combining mark and a double-width
// glyph. This locks in the parity the per-span WriteString path is meant to
// preserve (display width, combining-mark folding, wide-glyph advance) and would
// catch any future divergence between the two render paths.
func TestAddStyledSingleSpanMatchesPlainForTrickyContent(t *testing.T) {
	tricky := "a\u0301\u4e16x" // 'a' + combining acute + wide '\u4e16' (U+4E16) + 'x'
	mk := func() *TextView {
		v := NewTextView("", Rect{X: 0, Y: 0, W: 12, H: 2})
		v.FG = tui.ANSIColor(7)
		v.BG = tui.ANSIColor(0)
		return v
	}
	plain := mk()
	plain.AddLine(tricky)
	papp := drawTextView(plain, 12, 2)

	styled := mk()
	styled.AddStyled([]StyledSpan{{Text: tricky, FG: tui.ANSIColor(7), HasFG: true}})
	sapp := drawTextView(styled, 12, 2)

	for x := 0; x < 8; x++ {
		pc := papp.ReadCell(x, 0)
		sc := sapp.ReadCell(x, 0)
		if pc != sc {
			t.Fatalf("cell(%d,0) diverges: plain Ch=%q Comb=%q | styled Ch=%q Comb=%q",
				x, pc.Ch, pc.Combining, sc.Ch, sc.Combining)
		}
	}
	// Sanity: the wide glyph advanced two columns (combining mark folded, 'x' at col 3).
	if got := sapp.ReadCell(3, 0).Ch; got != 'x' {
		t.Fatalf("cell(3,0) = %q, want 'x' (acute folded + wide glyph advanced 2 cols)", got)
	}
}

// --- regression: plain paths unchanged -------------------------------------

// AddLine/AddColored render exactly as before the styled feature was added.
func TestPlainAddLineAndAddColoredUnchanged(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 4})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	col := tui.ANSIColor(3)
	view.AddLine("plain")
	view.AddColored("colored", col)
	app := drawTextView(view, 20, 4)

	// Plain row uses the view FG.
	if got := app.ReadCell(0, 0); got.Ch != 'p' || got.FG != tui.ANSIColor(7) {
		t.Fatalf("plain row cell(0,0) = %+v, want 'p' fg=%v", got, tui.ANSIColor(7))
	}
	// Colored row uses the entry colour.
	if got := app.ReadCell(0, 1); got.Ch != 'c' || got.FG != col {
		t.Fatalf("colored row cell(0,1) = %+v, want 'c' fg=%v", got, col)
	}
	// No italic leaks into plain rows.
	if app.ReadCell(0, 0).Italic || app.ReadCell(0, 1).Italic {
		t.Fatalf("plain rows should not carry italic")
	}
}

// Mixing AddStyled with AddLine in one view lays each out on its own row.
func TestAddStyledMixedWithPlainLines(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 4})
	view.FG = tui.ANSIColor(7)
	view.BG = tui.ANSIColor(0)
	red := tui.ANSIColor(1)
	view.AddLine("first")
	view.AddStyled([]StyledSpan{{Text: "second", FG: red, HasFG: true, Bold: true}})
	view.AddLine("third")
	app := drawTextView(view, 20, 4)

	if got := app.ReadCell(0, 0); got.Ch != 'f' || got.FG != tui.ANSIColor(7) {
		t.Fatalf("row0 = %+v, want 'f' viewFG", got)
	}
	got := app.ReadCell(0, 1)
	if got.Ch != 's' || got.FG != red || !got.Bold {
		t.Fatalf("row1 = %+v, want 's' fg=%v bold=true", got, red)
	}
	if got := app.ReadCell(0, 2); got.Ch != 't' || got.FG != tui.ANSIColor(7) {
		t.Fatalf("row2 = %+v, want 't' viewFG", got)
	}
}
